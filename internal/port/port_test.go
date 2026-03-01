package port

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tjst-t/port-manager/internal/config"
	"github.com/tjst-t/port-manager/internal/db"
)

func setupTestManager(t *testing.T) *Manager {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	cfg := config.DefaultConfig()
	cfg.Ports.RangeStart = 9100
	cfg.Ports.RangeEnd = 9199

	return &Manager{
		DB:     d,
		Config: cfg,
		Services: config.Services{
			Reserved: []config.ReservedPort{
				{Port: 80, Description: "http"},
			},
			Permanent: []config.PermanentService{
				{Name: "grafana", Port: 3000, Expose: true},
			},
		},
	}
}

func TestAllocate_NewLease(t *testing.T) {
	m := setupTestManager(t)

	result, err := m.Allocate(AllocateRequest{
		Project:      "tjst-t/palmux",
		Worktree:     "main",
		WorktreePath: "/tmp/palmux",
		Repo:         "palmux",
		Name:         "api",
		Expose:       true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsNew {
		t.Error("expected IsNew=true")
	}
	if result.Lease.Port < 9100 || result.Lease.Port > 9199 {
		t.Errorf("port out of range: %d", result.Lease.Port)
	}
	if result.Lease.State != "active" {
		t.Errorf("expected active state, got %s", result.Lease.State)
	}
	if !result.ExposeAdded {
		t.Error("expected ExposeAdded=true")
	}
}

func TestAllocate_ExistingActive(t *testing.T) {
	m := setupTestManager(t)

	// First allocation
	result1, err := m.Allocate(AllocateRequest{
		Project: "tjst-t/palmux", Worktree: "main",
		WorktreePath: "/tmp/palmux", Repo: "palmux",
		Name: "api", Expose: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Second allocation with same key
	result2, err := m.Allocate(AllocateRequest{
		Project: "tjst-t/palmux", Worktree: "main",
		WorktreePath: "/tmp/palmux", Repo: "palmux",
		Name: "api", Expose: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result2.IsNew {
		t.Error("expected IsNew=false for existing lease")
	}
	if result2.Lease.Port != result1.Lease.Port {
		t.Errorf("expected same port %d, got %d", result1.Lease.Port, result2.Lease.Port)
	}
}

func TestAllocate_ReactivateStale(t *testing.T) {
	m := setupTestManager(t)

	// Create and make stale
	result1, _ := m.Allocate(AllocateRequest{
		Project: "tjst-t/palmux", Worktree: "main",
		WorktreePath: "/tmp/palmux", Repo: "palmux",
		Name: "api",
	})
	now := time.Now()
	m.DB.UpdateLeaseState(result1.Lease.ID, "stale", &now)

	// Re-allocate same key
	result2, err := m.Allocate(AllocateRequest{
		Project: "tjst-t/palmux", Worktree: "main",
		WorktreePath: "/tmp/palmux", Repo: "palmux",
		Name: "api",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !result2.WasStale {
		t.Error("expected WasStale=true")
	}
	if result2.Lease.Port != result1.Lease.Port {
		t.Error("expected same port for reactivated lease")
	}
	if result2.Lease.State != "active" {
		t.Errorf("expected active state, got %s", result2.Lease.State)
	}
}

func TestAllocate_ExposeUpgrade(t *testing.T) {
	m := setupTestManager(t)

	// Allocate without expose
	result1, _ := m.Allocate(AllocateRequest{
		Project: "tjst-t/palmux", Worktree: "main",
		WorktreePath: "/tmp/palmux", Repo: "palmux",
		Name: "api", Expose: false,
	})
	if result1.Lease.Expose {
		t.Error("expected expose=false initially")
	}

	// Re-allocate with expose
	result2, err := m.Allocate(AllocateRequest{
		Project: "tjst-t/palmux", Worktree: "main",
		WorktreePath: "/tmp/palmux", Repo: "palmux",
		Name: "api", Expose: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if !result2.ExposeAdded {
		t.Error("expected ExposeAdded=true")
	}
	if !result2.Lease.Expose {
		t.Error("expected expose=true after upgrade")
	}
}

func TestAllocate_StalePortExcludedFromNewAllocation(t *testing.T) {
	m := setupTestManager(t)

	// Create a lease and make it stale
	result1, _ := m.Allocate(AllocateRequest{
		Project: "org/repo1", Worktree: "main",
		WorktreePath: "/tmp/repo1", Repo: "repo1",
		Name: "api",
	})
	now := time.Now()
	m.DB.UpdateLeaseState(result1.Lease.ID, "stale", &now)

	// New allocation for different key should skip the stale port
	result2, err := m.Allocate(AllocateRequest{
		Project: "org/repo2", Worktree: "main",
		WorktreePath: "/tmp/repo2", Repo: "repo2",
		Name: "api",
	})
	if err != nil {
		t.Fatal(err)
	}

	if result2.Lease.Port == result1.Lease.Port {
		t.Error("new allocation should not use stale port")
	}
}

func TestAllocate_HostnameCollision(t *testing.T) {
	m := setupTestManager(t)

	// Create first lease
	_, err := m.Allocate(AllocateRequest{
		Project: "org1/palmux", Worktree: "main",
		WorktreePath: "/tmp/palmux1", Repo: "palmux",
		Name: "api",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Try to create lease that generates the same hostname (different org, same repo)
	_, err = m.Allocate(AllocateRequest{
		Project: "org2/palmux", Worktree: "main",
		WorktreePath: "/tmp/palmux2", Repo: "palmux",
		Name: "api",
	})
	if err == nil {
		t.Error("expected hostname collision error")
	}
}

func TestAllocate_ReservedPortExcluded(t *testing.T) {
	m := setupTestManager(t)
	m.Config.Ports.RangeStart = 80
	m.Config.Ports.RangeEnd = 82

	result, err := m.Allocate(AllocateRequest{
		Project: "org/repo", Worktree: "main",
		WorktreePath: "/tmp/repo", Repo: "repo",
		Name: "api",
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Lease.Port == 80 {
		t.Error("should not allocate reserved port 80")
	}
}

func TestRelease(t *testing.T) {
	m := setupTestManager(t)

	// Allocate
	result, _ := m.Allocate(AllocateRequest{
		Project: "tjst-t/palmux", Worktree: "main",
		WorktreePath: "/tmp/palmux", Repo: "palmux",
		Name: "api", Expose: true,
	})

	// Release
	releaseResult, err := m.Release("tjst-t/palmux", "main", "api")
	if err != nil {
		t.Fatal(err)
	}

	if releaseResult.Port != result.Lease.Port {
		t.Errorf("expected port %d, got %d", result.Lease.Port, releaseResult.Port)
	}
	if !releaseResult.WasExpose {
		t.Error("expected WasExpose=true")
	}

	// Verify deleted
	lease, _ := m.DB.FindLease("tjst-t/palmux", "main", "api")
	if lease != nil {
		t.Error("expected lease to be deleted")
	}
}

func TestRelease_NotFound(t *testing.T) {
	m := setupTestManager(t)

	_, err := m.Release("no/project", "no-branch", "no-name")
	if err == nil {
		t.Error("expected error for non-existent lease")
	}
}

func TestRunGC_WorktreeRemoval(t *testing.T) {
	m := setupTestManager(t)

	// Create a lease with a non-existent worktree path
	_, err := m.Allocate(AllocateRequest{
		Project: "org/repo", Worktree: "main",
		WorktreePath: "/nonexistent/path/12345", Repo: "repo",
		Name: "api",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := m.RunGC()
	if err != nil {
		t.Fatal(err)
	}

	if len(result.WorktreeRemoved) != 1 {
		t.Errorf("expected 1 worktree removed, got %d", len(result.WorktreeRemoved))
	}
}

func TestRunGC_TTLExpiration(t *testing.T) {
	m := setupTestManager(t)
	m.Config.Ports.StaleTTLHours = 1

	// Create a lease with existing worktree path
	tmpDir := t.TempDir()
	result, _ := m.Allocate(AllocateRequest{
		Project: "org/repo", Worktree: "main",
		WorktreePath: tmpDir, Repo: "repo",
		Name: "api",
	})

	// Make it stale with old stale_since
	past := time.Now().Add(-2 * time.Hour)
	m.DB.UpdateLeaseState(result.Lease.ID, "stale", &past)

	gcResult, err := m.RunGC()
	if err != nil {
		t.Fatal(err)
	}

	if len(gcResult.TTLExpired) != 1 {
		t.Errorf("expected 1 TTL expired, got %d", len(gcResult.TTLExpired))
	}
}

func TestRunGC_SetsLastGCTime(t *testing.T) {
	m := setupTestManager(t)

	before := time.Now().Add(-time.Second)
	_, err := m.RunGC()
	if err != nil {
		t.Fatal(err)
	}

	gcTime, _ := m.DB.GetLastGCTime()
	if gcTime.Before(before) {
		t.Error("expected GC time to be updated")
	}
}

func TestMaybeLightGC_SkipsRecent(t *testing.T) {
	m := setupTestManager(t)

	// Set recent GC time
	m.DB.SetLastGCTime(time.Now())

	result, err := m.MaybeLightGC()
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil result for recent GC")
	}
}

func TestMaybeLightGC_RunsIfOld(t *testing.T) {
	m := setupTestManager(t)

	// Set old GC time
	m.DB.SetLastGCTime(time.Now().Add(-2 * time.Hour))

	// Create lease with non-existent path to verify GC runs
	m.Allocate(AllocateRequest{
		Project: "org/repo", Worktree: "main",
		WorktreePath: "/nonexistent/path/xyz", Repo: "repo",
		Name: "api",
	})

	result, err := m.MaybeLightGC()
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Error("expected GC to run for old GC time")
	}
}

func TestAllocate_NoPortsAvailable(t *testing.T) {
	m := setupTestManager(t)
	m.Config.Ports.RangeStart = 9100
	m.Config.Ports.RangeEnd = 9100 // Only 1 port available

	// Allocate the only port
	_, err := m.Allocate(AllocateRequest{
		Project: "org/repo", Worktree: "main",
		WorktreePath: "/tmp/repo", Repo: "repo",
		Name: "api",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Try to allocate another
	_, err = m.Allocate(AllocateRequest{
		Project: "org/repo", Worktree: "main",
		WorktreePath: "/tmp/repo", Repo: "repo",
		Name: "web",
	})
	if err == nil {
		t.Error("expected no-ports-available error")
	}
}

func TestRunGC_WorktreeExists(t *testing.T) {
	m := setupTestManager(t)

	// Create lease with a path that exists
	tmpDir := t.TempDir()
	worktreeDir := filepath.Join(tmpDir, "myworktree")
	os.MkdirAll(worktreeDir, 0755)

	_, err := m.Allocate(AllocateRequest{
		Project: "org/repo", Worktree: "main",
		WorktreePath: worktreeDir, Repo: "repo",
		Name: "api",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := m.RunGC()
	if err != nil {
		t.Fatal(err)
	}

	if len(result.WorktreeRemoved) != 0 {
		t.Error("should not remove lease for existing worktree")
	}
}
