package dashboard

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tjst-t/port-manager/internal/config"
	"github.com/tjst-t/port-manager/internal/db"
)

func setupTestDB(t *testing.T) *db.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestHandler_ServesHTML(t *testing.T) {
	database := setupTestDB(t)

	// Create a lease
	lease := &db.Lease{
		Port: 8200, Project: "tjst-t/palmux", Worktree: "main",
		WorktreePath: "/tmp/palmux", Repo: "palmux", Name: "api",
		Hostname: "api--main--palmux", Expose: true, State: "active",
	}
	if err := database.CreateLease(lease); err != nil {
		t.Fatal(err)
	}

	services := config.Services{
		Permanent: []config.PermanentService{
			{Name: "grafana", Port: 3000, Expose: true},
		},
	}

	proxyCfg := config.ProxyConfig{
		DomainSuffix: "example.com",
	}

	// Always report alive
	checker := func(l db.Lease) bool { return true }

	handler := NewHandler(database, services, proxyCfg, checker)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	if !strings.Contains(body, "text/html") {
		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/html") {
			t.Errorf("expected Content-Type text/html, got %s", ct)
		}
	}

	// Check auto-refresh meta tag (live mode)
	if !strings.Contains(body, `http-equiv="refresh"`) {
		t.Error("expected auto-refresh meta tag in live mode")
	}

	// Check lease data
	if !strings.Contains(body, "api") {
		t.Error("expected lease name 'api'")
	}
	if !strings.Contains(body, "8200") {
		t.Error("expected port 8200")
	}
	if !strings.Contains(body, "api--main--palmux.example.com") {
		t.Error("expected FQDN link")
	}

	// Check active status
	if !strings.Contains(body, "active") {
		t.Error("expected active status")
	}

	// Check permanent service
	if !strings.Contains(body, "grafana") {
		t.Error("expected permanent service 'grafana'")
	}
}

func TestHandler_StatusChecker(t *testing.T) {
	database := setupTestDB(t)

	lease := &db.Lease{
		Port: 8200, Project: "org/repo", Worktree: "main",
		WorktreePath: "/tmp/repo", Repo: "repo", Name: "api",
		Hostname: "api--main--repo", Expose: false, State: "active",
	}
	if err := database.CreateLease(lease); err != nil {
		t.Fatal(err)
	}

	proxyCfg := config.ProxyConfig{DomainSuffix: "example.com"}

	// Report not alive
	checker := func(l db.Lease) bool { return false }

	handler := NewHandler(database, config.Services{}, proxyCfg, checker)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "not running") {
		t.Error("expected 'not running' status when checker returns false")
	}
}

func TestHandler_EmptyDB(t *testing.T) {
	database := setupTestDB(t)
	proxyCfg := config.ProxyConfig{DomainSuffix: "example.com"}
	checker := func(l db.Lease) bool { return true }

	handler := NewHandler(database, config.Services{}, proxyCfg, checker)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "No active leases") {
		t.Error("expected 'No active leases' for empty DB")
	}
}
