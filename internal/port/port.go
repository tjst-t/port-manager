package port

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tjst-t/port-manager/internal/config"
	"github.com/tjst-t/port-manager/internal/db"
	"github.com/tjst-t/port-manager/internal/git"
)

const maxAllocateRetries = 3

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
	ExposeAdded   bool // true if expose was newly added (need Caddy registration)
	ExposeRemoved bool // true if expose was removed (need Caddy route deletion)
}

// ReleaseResult contains the result of a port release.
type ReleaseResult struct {
	Port     int
	Hostname string
	WasExpose bool
}

// KillInfo describes how a process was killed during GC.
type KillInfo struct {
	PID    int
	Method string // "pid" or "port-lookup"
}

// GCEntry pairs a lease with optional kill information.
type GCEntry struct {
	Lease    db.Lease
	KillInfo *KillInfo
}

// GCResult contains the result of a GC run.
type GCResult struct {
	WorktreeRemoved []GCEntry
	StaleMarked     []db.Lease
	TTLExpired      []GCEntry
}

// AllocateRangeRequest contains the parameters for allocating a port range.
type AllocateRangeRequest struct {
	Project      string
	Worktree     string
	WorktreePath string
	Repo         string
	Name         string
	Count        int // number of contiguous ports to allocate
}

// AllocateRangeResult contains the result of a port range allocation.
type AllocateRangeResult struct {
	Lease    *db.Lease
	IsNew    bool
	WasStale bool
}

// Allocate assigns a port based on the request parameters.
func (m *Manager) Allocate(req AllocateRequest) (*AllocateResult, error) {
	// Check for existing lease
	existing, err := m.DB.FindLease(req.Project, req.Worktree, req.Name)
	if err != nil {
		return nil, fmt.Errorf("finding existing lease: %w", err)
	}

	if existing != nil {
		if existing.IsRange() {
			return nil, fmt.Errorf("existing lease %s:%s:%s is a range lease, not a single port — release it first",
				req.Project, req.Worktree, req.Name)
		}

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
		if req.Expose != existing.Expose {
			if err := m.DB.UpdateLeaseExpose(existing.ID, req.Expose); err != nil {
				return nil, fmt.Errorf("updating expose: %w", err)
			}
			existing.Expose = req.Expose
			if req.Expose {
				result.ExposeAdded = true
			} else {
				result.ExposeRemoved = true
			}
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

	// Find available port and create lease with retry for UNIQUE constraint violations.
	// Another portman process may grab the same port between findAvailablePort and CreateLease.
	var lease *db.Lease
	for attempt := 0; attempt < maxAllocateRetries; attempt++ {
		port, err := m.findAvailablePort()
		if err != nil {
			return nil, fmt.Errorf("finding available port: %w", err)
		}

		lease = &db.Lease{
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

		err = m.DB.CreateLease(lease)
		if err == nil {
			break
		}
		if !isUniqueConstraintError(err) {
			return nil, fmt.Errorf("creating lease: %w", err)
		}
		// UNIQUE constraint violation on port — retry with a fresh port scan
		if attempt == maxAllocateRetries-1 {
			return nil, fmt.Errorf("creating lease: failed after %d retries: %w", maxAllocateRetries, err)
		}
	}

	return &AllocateResult{
		Lease:       lease,
		IsNew:       true,
		ExposeAdded: req.Expose,
	}, nil
}

// AllocateRange assigns a contiguous range of ports based on the request parameters.
func (m *Manager) AllocateRange(req AllocateRangeRequest) (*AllocateRangeResult, error) {
	if req.Count <= 0 {
		return nil, fmt.Errorf("range count must be positive, got %d", req.Count)
	}

	// Check for existing lease
	existing, err := m.DB.FindLease(req.Project, req.Worktree, req.Name)
	if err != nil {
		return nil, fmt.Errorf("finding existing lease: %w", err)
	}

	if existing != nil {
		result := &AllocateRangeResult{Lease: existing}

		if !existing.IsRange() {
			return nil, fmt.Errorf("existing lease %s:%s:%s is a single-port lease, not a range — release it first",
				req.Project, req.Worktree, req.Name)
		}

		if existing.PortCount != req.Count {
			return nil, fmt.Errorf("existing range lease %s:%s:%s has count %d, requested %d — release it first",
				req.Project, req.Worktree, req.Name, existing.PortCount, req.Count)
		}

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

		return result, nil
	}

	// Generate hostname for identification (ranges are never exposed)
	hostname, err := git.GenerateHostname(
		req.Name, req.Worktree, req.Repo,
		m.Config.Proxy.HostPattern, m.Config.Proxy.DomainSuffix,
	)
	if err != nil {
		return nil, fmt.Errorf("generating hostname: %w", err)
	}

	hostLabel := hostname[:len(hostname)-len(m.Config.Proxy.DomainSuffix)-1]

	existingByHost, err := m.DB.FindLeaseByHostname(hostLabel)
	if err != nil {
		return nil, fmt.Errorf("checking hostname collision: %w", err)
	}
	if existingByHost != nil {
		return nil, fmt.Errorf("hostname collision: %s is already used by %s:%s:%s — use --name to differentiate",
			hostLabel, existingByHost.Project, existingByHost.Worktree, existingByHost.Name)
	}

	// Find contiguous block and create lease with retry.
	// NOTE: The port UNIQUE constraint only protects the start port (port column).
	// Ports within the range (port+1 to port_end) are not protected by DB constraints.
	// Concurrent processes could theoretically overlap ranges. In practice this is
	// mitigated by: (1) AllocatedPorts() expanding ranges before allocation, and
	// (2) SQLite WAL mode serializing writes. A transaction-level check would be
	// needed for full safety under high concurrency.
	var lease *db.Lease
	for attempt := 0; attempt < maxAllocateRetries; attempt++ {
		startPort, err := m.findAvailablePortRange(req.Count)
		if err != nil {
			return nil, fmt.Errorf("finding available port range: %w", err)
		}

		lease = &db.Lease{
			Port:         startPort,
			PortEnd:      startPort + req.Count - 1,
			PortCount:    req.Count,
			Project:      req.Project,
			Worktree:     req.Worktree,
			WorktreePath: req.WorktreePath,
			Repo:         req.Repo,
			Name:         req.Name,
			Hostname:     hostLabel,
			Expose:       false, // ranges are never exposed
			State:        "active",
		}

		err = m.DB.CreateLease(lease)
		if err == nil {
			break
		}
		if !isUniqueConstraintError(err) {
			return nil, fmt.Errorf("creating range lease: %w", err)
		}
		if attempt == maxAllocateRetries-1 {
			return nil, fmt.Errorf("creating range lease: failed after %d retries: %w", maxAllocateRetries, err)
		}
	}

	return &AllocateRangeResult{
		Lease: lease,
		IsNew: true,
	}, nil
}

// findAvailablePortRange finds the first contiguous block of `count` ports in the configured range.
func (m *Manager) findAvailablePortRange(count int) (int, error) {
	allocatedPorts, err := m.DB.AllocatedPorts()
	if err != nil {
		return 0, err
	}

	usedPorts := make(map[int]bool)
	for _, p := range allocatedPorts {
		usedPorts[p] = true
	}
	for _, r := range m.Services.Reserved {
		usedPorts[r.Port] = true
	}
	for _, p := range m.Services.Permanent {
		usedPorts[p.Port] = true
	}

	// Scan for contiguous block
	consecutive := 0
	startPort := m.Config.Ports.RangeStart
	for port := m.Config.Ports.RangeStart; port <= m.Config.Ports.RangeEnd; port++ {
		if usedPorts[port] {
			consecutive = 0
			startPort = port + 1
		} else {
			consecutive++
			if consecutive == count {
				return startPort, nil
			}
		}
	}

	return 0, fmt.Errorf("no contiguous %d-port block available in range %d-%d",
		count, m.Config.Ports.RangeStart, m.Config.Ports.RangeEnd)
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
			ki := killLeaseProcess(lease)
			if err := m.DB.DeleteLease(lease.ID); err != nil {
				return nil, fmt.Errorf("deleting lease (worktree gone): %w", err)
			}
			result.WorktreeRemoved = append(result.WorktreeRemoved, GCEntry{Lease: lease, KillInfo: ki})
		}
	}

	// Stage 2: listen state check (active leases only)
	activeLeases, err := m.DB.ListActiveLeases()
	if err != nil {
		return nil, fmt.Errorf("listing active leases: %w", err)
	}

	for _, lease := range activeLeases {
		shouldMarkStale := false
		if lease.IsRange() {
			// Range leases: only check PID if tracked, skip port listen check
			if lease.PID > 0 && !IsProcessAlive(lease.PID) {
				shouldMarkStale = true
			}
		} else if lease.PID > 0 && !IsProcessAlive(lease.PID) {
			// PID is tracked and process is dead → immediate stale
			shouldMarkStale = true
		} else if lease.PID <= 0 && !IsPortListening(lease.Port) {
			// No PID tracked → fallback to port listen check
			shouldMarkStale = true
		}
		if shouldMarkStale {
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
			ki := killLeaseProcess(lease)
			if err := m.DB.DeleteLease(lease.ID); err != nil {
				return nil, fmt.Errorf("deleting expired lease: %w", err)
			}
			result.TTLExpired = append(result.TTLExpired, GCEntry{Lease: lease, KillInfo: ki})
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

// IsProcessAlive checks if a process with the given PID is still running.
func IsProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// IsPortListening checks if a port is currently being listened on.
func IsPortListening(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// findPIDByPort searches /proc/net/tcp and /proc/net/tcp6 to find the PID
// listening on the given port. Returns 0 if not found (best-effort).
func findPIDByPort(port int) int {
	inode := findInodeByPort(port)
	if inode == 0 {
		return 0
	}
	return findPIDByInode(inode)
}

// findInodeByPort parses /proc/net/tcp and /proc/net/tcp6 looking for a socket
// in LISTEN state on the given port, and returns its inode.
func findInodeByPort(port int) uint64 {
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		if inode := findInodeInFile(path, port); inode != 0 {
			return inode
		}
	}
	return 0
}

// findInodeInFile parses a /proc/net/tcp{,6} file for a listening socket on the given port.
func findInodeInFile(path string, port int) uint64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Scan() // skip header line
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			continue
		}
		// fields[1] = local_address (hex_ip:hex_port)
		// fields[3] = state (0A = LISTEN)
		// fields[9] = inode
		if fields[3] != "0A" {
			continue // not LISTEN state
		}
		parts := strings.Split(fields[1], ":")
		if len(parts) != 2 {
			continue
		}
		hexPort := parts[1]
		p, err := strconv.ParseUint(hexPort, 16, 16)
		if err != nil {
			continue
		}
		if int(p) == port {
			inode, err := strconv.ParseUint(fields[9], 10, 64)
			if err != nil {
				continue
			}
			return inode
		}
	}
	return 0
}

// findPIDByInode walks /proc/*/fd/ to find which process owns the given socket inode.
func findPIDByInode(inode uint64) int {
	target := fmt.Sprintf("socket:[%d]", inode)

	procDirs, err := filepath.Glob("/proc/[0-9]*/fd/*")
	if err != nil {
		return 0
	}

	for _, fdPath := range procDirs {
		link, err := os.Readlink(fdPath)
		if err != nil {
			continue
		}
		if link == target {
			// fdPath is /proc/<pid>/fd/<n>
			parts := strings.Split(fdPath, "/")
			if len(parts) >= 3 {
				pid, err := strconv.Atoi(parts[2])
				if err == nil {
					return pid
				}
			}
		}
	}
	return 0
}

// terminateProcess sends SIGTERM, then polls up to 3 seconds, then SIGKILL if still alive.
// Returns true if the process was successfully terminated.
func terminateProcess(pid int) bool {
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return false
	}

	// Poll every 100ms for up to 3 seconds
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if !IsProcessAlive(pid) {
			return true
		}
	}

	// Still alive — SIGKILL
	_ = syscall.Kill(pid, syscall.SIGKILL)
	time.Sleep(100 * time.Millisecond)
	return !IsProcessAlive(pid)
}

// killLeaseProcess attempts to kill the process associated with a lease.
// It tries the stored PID first, then falls back to port-based lookup.
// Returns nil if no process was found or killed.
func killLeaseProcess(lease db.Lease) *KillInfo {
	// Try stored PID first
	if lease.PID > 0 && IsProcessAlive(lease.PID) {
		if terminateProcess(lease.PID) {
			return &KillInfo{PID: lease.PID, Method: "pid"}
		}
	}

	// Fallback: find PID by port
	pid := findPIDByPort(lease.Port)
	if pid > 0 && IsProcessAlive(pid) {
		if terminateProcess(pid) {
			return &KillInfo{PID: pid, Method: "port-lookup"}
		}
	}

	return nil
}

// isUniqueConstraintError checks if the error is a SQLite UNIQUE constraint violation.
func isUniqueConstraintError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
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
		if IsPortListening(port) {
			results = append(results, AuditResult{Port: port})
		}
	}

	return results, nil
}
