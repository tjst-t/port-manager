package db

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestOpen_CreatesDB(t *testing.T) {
	d := setupTestDB(t)
	// Verify tables exist by performing operations
	leases, err := d.ListLeases()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(leases) != 0 {
		t.Errorf("expected empty leases, got %d", len(leases))
	}
}

func TestCreateAndFindLease(t *testing.T) {
	d := setupTestDB(t)

	lease := &Lease{
		Port:         8200,
		Project:      "tjst-t/palmux",
		Worktree:     "main",
		WorktreePath: "/home/user/palmux",
		Repo:         "palmux",
		Name:         "api",
		Hostname:     "api--main--palmux",
		Expose:       true,
		State:        "active",
	}

	if err := d.CreateLease(lease); err != nil {
		t.Fatalf("failed to create lease: %v", err)
	}

	if lease.ID == 0 {
		t.Error("expected non-zero ID")
	}

	found, err := d.FindLease("tjst-t/palmux", "main", "api")
	if err != nil {
		t.Fatalf("failed to find lease: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find lease")
	}
	if found.Port != 8200 {
		t.Errorf("expected port 8200, got %d", found.Port)
	}
	if found.Hostname != "api--main--palmux" {
		t.Errorf("expected hostname api--main--palmux, got %s", found.Hostname)
	}
	if !found.Expose {
		t.Error("expected expose=true")
	}
}

func TestFindLease_NotFound(t *testing.T) {
	d := setupTestDB(t)

	found, err := d.FindLease("no/project", "no-branch", "no-name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != nil {
		t.Error("expected nil for non-existent lease")
	}
}

func TestFindLeaseByHostname(t *testing.T) {
	d := setupTestDB(t)

	lease := &Lease{
		Port:         8201,
		Project:      "tjst-t/palmux",
		Worktree:     "feature",
		WorktreePath: "/home/user/palmux-feature",
		Repo:         "palmux",
		Name:         "api",
		Hostname:     "api--feature--palmux",
		State:        "active",
	}
	if err := d.CreateLease(lease); err != nil {
		t.Fatal(err)
	}

	found, err := d.FindLeaseByHostname("api--feature--palmux")
	if err != nil {
		t.Fatal(err)
	}
	if found == nil {
		t.Fatal("expected to find lease by hostname")
	}
	if found.Port != 8201 {
		t.Errorf("expected port 8201, got %d", found.Port)
	}
}

func TestUpdateLeaseState(t *testing.T) {
	d := setupTestDB(t)

	lease := &Lease{
		Port: 8202, Project: "org/repo", Worktree: "main",
		WorktreePath: "/tmp/repo", Repo: "repo", Name: "default",
		Hostname: "default--main--repo", State: "active",
	}
	if err := d.CreateLease(lease); err != nil {
		t.Fatal(err)
	}

	now := time.Now().Truncate(time.Second)
	if err := d.UpdateLeaseState(lease.ID, "stale", &now); err != nil {
		t.Fatal(err)
	}

	found, _ := d.FindLease("org/repo", "main", "default")
	if found.State != "stale" {
		t.Errorf("expected stale, got %s", found.State)
	}
	if found.StaleSince == nil {
		t.Error("expected stale_since to be set")
	}

	// Revert to active
	if err := d.UpdateLeaseState(lease.ID, "active", nil); err != nil {
		t.Fatal(err)
	}
	found, _ = d.FindLease("org/repo", "main", "default")
	if found.State != "active" {
		t.Errorf("expected active, got %s", found.State)
	}
}

func TestUpdateLastUsed(t *testing.T) {
	d := setupTestDB(t)

	lease := &Lease{
		Port: 8203, Project: "org/repo", Worktree: "dev",
		WorktreePath: "/tmp/repo", Repo: "repo", Name: "web",
		Hostname: "web--dev--repo", State: "active",
	}
	if err := d.CreateLease(lease); err != nil {
		t.Fatal(err)
	}

	if err := d.UpdateLastUsed(lease.ID); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteLease(t *testing.T) {
	d := setupTestDB(t)

	lease := &Lease{
		Port: 8204, Project: "org/repo", Worktree: "main",
		WorktreePath: "/tmp/repo", Repo: "repo", Name: "api",
		Hostname: "api--main--repo", State: "active",
	}
	if err := d.CreateLease(lease); err != nil {
		t.Fatal(err)
	}

	if err := d.DeleteLease(lease.ID); err != nil {
		t.Fatal(err)
	}

	found, _ := d.FindLease("org/repo", "main", "api")
	if found != nil {
		t.Error("expected lease to be deleted")
	}
}

func TestListLeases(t *testing.T) {
	d := setupTestDB(t)

	for i, name := range []string{"a", "b", "c"} {
		lease := &Lease{
			Port: 8210 + i, Project: "org/repo", Worktree: "main",
			WorktreePath: "/tmp/repo", Repo: "repo", Name: name,
			Hostname: name + "--main--repo", State: "active",
		}
		if err := d.CreateLease(lease); err != nil {
			t.Fatal(err)
		}
	}

	leases, err := d.ListLeases()
	if err != nil {
		t.Fatal(err)
	}
	if len(leases) != 3 {
		t.Errorf("expected 3 leases, got %d", len(leases))
	}
}

func TestListActiveAndStaleLeases(t *testing.T) {
	d := setupTestDB(t)

	active := &Lease{
		Port: 8220, Project: "org/repo", Worktree: "main",
		WorktreePath: "/tmp/repo", Repo: "repo", Name: "active-svc",
		Hostname: "active-svc--main--repo", State: "active",
	}
	stale := &Lease{
		Port: 8221, Project: "org/repo", Worktree: "main",
		WorktreePath: "/tmp/repo", Repo: "repo", Name: "stale-svc",
		Hostname: "stale-svc--main--repo", State: "active",
	}
	if err := d.CreateLease(active); err != nil {
		t.Fatal(err)
	}
	if err := d.CreateLease(stale); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	if err := d.UpdateLeaseState(stale.ID, "stale", &now); err != nil {
		t.Fatal(err)
	}

	activeLeases, err := d.ListActiveLeases()
	if err != nil {
		t.Fatal(err)
	}
	if len(activeLeases) != 1 {
		t.Errorf("expected 1 active lease, got %d", len(activeLeases))
	}

	staleLeases, err := d.ListStaleLeases()
	if err != nil {
		t.Fatal(err)
	}
	if len(staleLeases) != 1 {
		t.Errorf("expected 1 stale lease, got %d", len(staleLeases))
	}
}

func TestListExposeLeases(t *testing.T) {
	d := setupTestDB(t)

	exposed := &Lease{
		Port: 8230, Project: "org/repo", Worktree: "main",
		WorktreePath: "/tmp/repo", Repo: "repo", Name: "exposed",
		Hostname: "exposed--main--repo", Expose: true, State: "active",
	}
	internal := &Lease{
		Port: 8231, Project: "org/repo", Worktree: "main",
		WorktreePath: "/tmp/repo", Repo: "repo", Name: "internal",
		Hostname: "internal--main--repo", Expose: false, State: "active",
	}
	if err := d.CreateLease(exposed); err != nil {
		t.Fatal(err)
	}
	if err := d.CreateLease(internal); err != nil {
		t.Fatal(err)
	}

	exposeLeases, err := d.ListExposeLeases()
	if err != nil {
		t.Fatal(err)
	}
	if len(exposeLeases) != 1 {
		t.Errorf("expected 1 exposed lease, got %d", len(exposeLeases))
	}
}

func TestAllocatedPorts(t *testing.T) {
	d := setupTestDB(t)

	for i := 0; i < 3; i++ {
		lease := &Lease{
			Port: 8240 + i, Project: "org/repo", Worktree: "main",
			WorktreePath: "/tmp/repo", Repo: "repo", Name: "svc" + string(rune('a'+i)),
			Hostname: "svc" + string(rune('a'+i)) + "--main--repo", State: "active",
		}
		if err := d.CreateLease(lease); err != nil {
			t.Fatal(err)
		}
	}

	ports, err := d.AllocatedPorts()
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 3 {
		t.Errorf("expected 3 ports, got %d", len(ports))
	}
	if ports[0] != 8240 || ports[1] != 8241 || ports[2] != 8242 {
		t.Errorf("unexpected ports: %v", ports)
	}
}

func TestGCState(t *testing.T) {
	d := setupTestDB(t)

	// No GC time yet
	gcTime, err := d.GetLastGCTime()
	if err != nil {
		t.Fatal(err)
	}
	if !gcTime.IsZero() {
		t.Error("expected zero time for unset GC state")
	}

	// Set GC time
	now := time.Now().Truncate(time.Second)
	if err := d.SetLastGCTime(now); err != nil {
		t.Fatal(err)
	}

	gcTime, err = d.GetLastGCTime()
	if err != nil {
		t.Fatal(err)
	}
	if !gcTime.Equal(now) {
		t.Errorf("expected %v, got %v", now, gcTime)
	}

	// Update GC time
	later := now.Add(time.Hour)
	if err := d.SetLastGCTime(later); err != nil {
		t.Fatal(err)
	}
	gcTime, _ = d.GetLastGCTime()
	if !gcTime.Equal(later) {
		t.Errorf("expected %v, got %v", later, gcTime)
	}
}

func TestUniqueConstraints(t *testing.T) {
	d := setupTestDB(t)

	lease1 := &Lease{
		Port: 8250, Project: "org/repo", Worktree: "main",
		WorktreePath: "/tmp/repo", Repo: "repo", Name: "api",
		Hostname: "api--main--repo", State: "active",
	}
	if err := d.CreateLease(lease1); err != nil {
		t.Fatal(err)
	}

	// Duplicate port
	lease2 := &Lease{
		Port: 8250, Project: "org/other", Worktree: "main",
		WorktreePath: "/tmp/other", Repo: "other", Name: "api",
		Hostname: "api--main--other", State: "active",
	}
	if err := d.CreateLease(lease2); err == nil {
		t.Error("expected error for duplicate port")
	}

	// Duplicate hostname
	lease3 := &Lease{
		Port: 8251, Project: "org/repo2", Worktree: "main",
		WorktreePath: "/tmp/repo2", Repo: "repo2", Name: "api",
		Hostname: "api--main--repo", State: "active",
	}
	if err := d.CreateLease(lease3); err == nil {
		t.Error("expected error for duplicate hostname")
	}

	// Duplicate project+worktree+name
	lease4 := &Lease{
		Port: 8252, Project: "org/repo", Worktree: "main",
		WorktreePath: "/tmp/repo", Repo: "repo", Name: "api",
		Hostname: "api2--main--repo", State: "active",
	}
	if err := d.CreateLease(lease4); err == nil {
		t.Error("expected error for duplicate project+worktree+name")
	}
}

func TestConcurrentAccess(t *testing.T) {
	d := setupTestDB(t)

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			lease := &Lease{
				Port: 8300 + i, Project: "org/repo", Worktree: "main",
				WorktreePath: "/tmp/repo", Repo: "repo",
				Name:     fmt.Sprintf("svc-%d", i),
				Hostname: fmt.Sprintf("svc-%d--main--repo", i),
				State:    "active",
			}
			if err := d.CreateLease(lease); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent error: %v", err)
	}

	leases, _ := d.ListLeases()
	if len(leases) != 10 {
		t.Errorf("expected 10 leases, got %d", len(leases))
	}
}

func TestUpdateLeasePID(t *testing.T) {
	d := setupTestDB(t)

	lease := &Lease{
		Port: 8270, Project: "org/repo", Worktree: "main",
		WorktreePath: "/tmp/repo", Repo: "repo", Name: "svc",
		Hostname: "svc--main--repo", State: "active",
	}
	if err := d.CreateLease(lease); err != nil {
		t.Fatal(err)
	}

	// PID should be 0 initially
	found, _ := d.FindLease("org/repo", "main", "svc")
	if found.PID != 0 {
		t.Errorf("expected PID=0 initially, got %d", found.PID)
	}

	// Update PID
	if err := d.UpdateLeasePID(lease.ID, 12345); err != nil {
		t.Fatal(err)
	}

	found, _ = d.FindLease("org/repo", "main", "svc")
	if found.PID != 12345 {
		t.Errorf("expected PID=12345, got %d", found.PID)
	}

	// Clear PID
	if err := d.UpdateLeasePID(lease.ID, 0); err != nil {
		t.Fatal(err)
	}

	found, _ = d.FindLease("org/repo", "main", "svc")
	if found.PID != 0 {
		t.Errorf("expected PID=0 after clear, got %d", found.PID)
	}
}

func TestUpdateLeaseExpose(t *testing.T) {
	d := setupTestDB(t)

	lease := &Lease{
		Port: 8260, Project: "org/repo", Worktree: "main",
		WorktreePath: "/tmp/repo", Repo: "repo", Name: "svc",
		Hostname: "svc--main--repo", State: "active", Expose: false,
	}
	if err := d.CreateLease(lease); err != nil {
		t.Fatal(err)
	}

	if err := d.UpdateLeaseExpose(lease.ID, true); err != nil {
		t.Fatal(err)
	}

	found, _ := d.FindLease("org/repo", "main", "svc")
	if !found.Expose {
		t.Error("expected expose=true after update")
	}
}
