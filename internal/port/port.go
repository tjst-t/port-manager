package port

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/tjst-t/port-manager/internal/config"
	"github.com/tjst-t/port-manager/internal/db"
	"github.com/tjst-t/port-manager/internal/git"
)

// Manager handles port allocation, release, and GC operations.
type Manager struct {
	DB       *db.DB
	Config   config.Config
	Services config.Services
}

// AllocateRequest contains the parameters for allocating a port.
type AllocateRequest struct {
	Project      string
	Worktree     string
	WorktreePath string
	Repo         string
	Name         string
	Expose       bool
}

// AllocateResult contains the result of a port allocation.
type AllocateResult struct {
	Lease       *db.Lease
	IsNew       bool
	WasStale    bool
	ExposeAdded bool // true if expose was newly added (need Caddy registration)
}

// ReleaseResult contains the result of a port release.
type ReleaseResult struct {
	Port     int
	Hostname string
	WasExpose bool
}

// GCResult contains the result of a GC run.
type GCResult struct {
	WorktreeRemoved []db.Lease
	StaleMarked     []db.Lease
	TTLExpired      []db.Lease
}

// Allocate assigns a port based on the request parameters.
func (m *Manager) Allocate(req AllocateRequest) (*AllocateResult, error) {
	// Check for existing lease
	existing, err := m.DB.FindLease(req.Project, req.Worktree, req.Name)
	if err != nil {
		return nil, fmt.Errorf("finding existing lease: %w", err)
	}

	if existing != nil {
		result := &AllocateResult{Lease: existing}

		// If stale, reactivate
		if existing.State == "stale" {
			if err := m.DB.UpdateLeaseState(existing.ID, "active", nil); err != nil {
				return nil, fmt.Errorf("reactivating lease: %w", err)
			}
			existing.State = "active"
			existing.StaleSince = nil
			result.WasStale = true
		}

		// Update last_used
		if err := m.DB.UpdateLastUsed(existing.ID); err != nil {
			return nil, fmt.Errorf("updating last_used: %w", err)
		}

		// Update expose if needed
		if req.Expose && !existing.Expose {
			if err := m.DB.UpdateLeaseExpose(existing.ID, true); err != nil {
				return nil, fmt.Errorf("updating expose: %w", err)
			}
			existing.Expose = true
			result.ExposeAdded = true
		}

		return result, nil
	}

	// Generate hostname and check for collision
	hostname, err := git.GenerateHostname(
		req.Name, req.Worktree, req.Repo,
		m.Config.Proxy.HostPattern, m.Config.Proxy.DomainSuffix,
	)
	if err != nil {
		return nil, fmt.Errorf("generating hostname: %w", err)
	}

	// hostname without domain suffix for DB storage
	hostLabel := hostname[:len(hostname)-len(m.Config.Proxy.DomainSuffix)-1]

	existingByHost, err := m.DB.FindLeaseByHostname(hostLabel)
	if err != nil {
		return nil, fmt.Errorf("checking hostname collision: %w", err)
	}
	if existingByHost != nil {
		return nil, fmt.Errorf("hostname collision: %s is already used by %s:%s:%s — use --name to differentiate",
			hostLabel, existingByHost.Project, existingByHost.Worktree, existingByHost.Name)
	}

	// Find available port
	port, err := m.findAvailablePort()
	if err != nil {
		return nil, fmt.Errorf("finding available port: %w", err)
	}

	lease := &db.Lease{
		Port:         port,
		Project:      req.Project,
		Worktree:     req.Worktree,
		WorktreePath: req.WorktreePath,
		Repo:         req.Repo,
		Name:         req.Name,
		Hostname:     hostLabel,
		Expose:       req.Expose,
		State:        "active",
	}

	if err := m.DB.CreateLease(lease); err != nil {
		return nil, fmt.Errorf("creating lease: %w", err)
	}

	return &AllocateResult{
		Lease:       lease,
		IsNew:       true,
		ExposeAdded: req.Expose,
	}, nil
}

// findAvailablePort finds the first available port in the configured range.
func (m *Manager) findAvailablePort() (int, error) {
	allocatedPorts, err := m.DB.AllocatedPorts()
	if err != nil {
		return 0, err
	}

	usedPorts := make(map[int]bool)
	for _, p := range allocatedPorts {
		usedPorts[p] = true
	}

	// Exclude reserved ports
	for _, r := range m.Services.Reserved {
		usedPorts[r.Port] = true
	}

	// Exclude permanent ports
	for _, p := range m.Services.Permanent {
		usedPorts[p.Port] = true
	}

	// Find first available port in range
	for port := m.Config.Ports.RangeStart; port <= m.Config.Ports.RangeEnd; port++ {
		if !usedPorts[port] {
			return port, nil
		}
	}

	return 0, fmt.Errorf("no available ports in range %d-%d", m.Config.Ports.RangeStart, m.Config.Ports.RangeEnd)
}

// Release removes a lease and returns the release result.
func (m *Manager) Release(project, worktree, name string) (*ReleaseResult, error) {
	lease, err := m.DB.FindLease(project, worktree, name)
	if err != nil {
		return nil, fmt.Errorf("finding lease: %w", err)
	}
	if lease == nil {
		return nil, fmt.Errorf("no lease found for %s:%s:%s", project, worktree, name)
	}

	if err := m.DB.DeleteLease(lease.ID); err != nil {
		return nil, fmt.Errorf("deleting lease: %w", err)
	}

	return &ReleaseResult{
		Port:      lease.Port,
		Hostname:  lease.Hostname,
		WasExpose: lease.Expose,
	}, nil
}

// RunGC performs the three-stage garbage collection.
func (m *Manager) RunGC() (*GCResult, error) {
	result := &GCResult{}

	// Stage 1: worktree existence check
	leases, err := m.DB.ListLeases()
	if err != nil {
		return nil, fmt.Errorf("listing leases: %w", err)
	}

	for _, lease := range leases {
		if _, err := os.Stat(lease.WorktreePath); os.IsNotExist(err) {
			if err := m.DB.DeleteLease(lease.ID); err != nil {
				return nil, fmt.Errorf("deleting lease (worktree gone): %w", err)
			}
			result.WorktreeRemoved = append(result.WorktreeRemoved, lease)
		}
	}

	// Stage 2: listen state check (active leases only)
	activeLeases, err := m.DB.ListActiveLeases()
	if err != nil {
		return nil, fmt.Errorf("listing active leases: %w", err)
	}

	for _, lease := range activeLeases {
		if !isPortListening(lease.Port) {
			now := time.Now()
			if err := m.DB.UpdateLeaseState(lease.ID, "stale", &now); err != nil {
				return nil, fmt.Errorf("marking lease stale: %w", err)
			}
			result.StaleMarked = append(result.StaleMarked, lease)
		}
	}

	// Stage 3: TTL expiration check (stale leases only)
	staleLeases, err := m.DB.ListStaleLeases()
	if err != nil {
		return nil, fmt.Errorf("listing stale leases: %w", err)
	}

	ttl := time.Duration(m.Config.Ports.StaleTTLHours) * time.Hour
	now := time.Now()
	for _, lease := range staleLeases {
		if lease.StaleSince != nil && now.Sub(*lease.StaleSince) > ttl {
			if err := m.DB.DeleteLease(lease.ID); err != nil {
				return nil, fmt.Errorf("deleting expired lease: %w", err)
			}
			result.TTLExpired = append(result.TTLExpired, lease)
		}
	}

	// Update last GC time
	if err := m.DB.SetLastGCTime(now); err != nil {
		return nil, fmt.Errorf("updating GC time: %w", err)
	}

	return result, nil
}

// MaybeLightGC runs GC only if the last GC was more than 1 hour ago.
func (m *Manager) MaybeLightGC() (*GCResult, error) {
	lastGC, err := m.DB.GetLastGCTime()
	if err != nil {
		return nil, fmt.Errorf("getting last GC time: %w", err)
	}

	if time.Since(lastGC) < time.Hour {
		return nil, nil
	}

	return m.RunGC()
}

// isPortListening checks if a port is currently being listened on.
func isPortListening(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// AuditResult represents an unauthorized port usage.
type AuditResult struct {
	Port    int
	Process string // process info if available
}

// Audit checks for port usage within the configured range that is not
// managed by portman (no lease, not reserved, not permanent).
func (m *Manager) Audit() ([]AuditResult, error) {
	allocatedPorts, err := m.DB.AllocatedPorts()
	if err != nil {
		return nil, fmt.Errorf("getting allocated ports: %w", err)
	}

	knownPorts := make(map[int]bool)
	for _, p := range allocatedPorts {
		knownPorts[p] = true
	}
	for _, r := range m.Services.Reserved {
		knownPorts[r.Port] = true
	}
	for _, p := range m.Services.Permanent {
		knownPorts[p.Port] = true
	}

	var results []AuditResult
	for port := m.Config.Ports.RangeStart; port <= m.Config.Ports.RangeEnd; port++ {
		if knownPorts[port] {
			continue
		}
		if isPortListening(port) {
			results = append(results, AuditResult{Port: port})
		}
	}

	return results, nil
}
