package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tjst-t/port-manager/internal/config"
	"github.com/tjst-t/port-manager/internal/db"
)

func TestGenerate_CreatesFile(t *testing.T) {
	dir := t.TempDir()

	leases := []db.Lease{
		{
			Name: "api", Project: "tjst-t/palmux", Worktree: "main",
			Port: 8200, Hostname: "api--main--palmux", Expose: true, State: "active",
		},
	}

	permanents := []config.PermanentService{
		{Name: "grafana", Port: 3000, Expose: true},
	}

	err := Generate(dir, leases, permanents, "example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("failed to read generated file: %v", err)
	}

	html := string(content)

	// Check structure
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("expected HTML doctype")
	}
	if !strings.Contains(html, "portman dashboard") {
		t.Error("expected title")
	}

	// Check lease data
	if !strings.Contains(html, "api") {
		t.Error("expected lease name 'api'")
	}
	if !strings.Contains(html, "tjst-t/palmux") {
		t.Error("expected project name")
	}
	if !strings.Contains(html, "8200") {
		t.Error("expected port 8200")
	}

	// Check expose link on Name
	if !strings.Contains(html, "api--main--palmux.example.com") {
		t.Error("expected FQDN link for exposed lease")
	}

	// Check active indicator (dot)
	if !strings.Contains(html, "dot-active") {
		t.Error("expected active status dot")
	}

	// Check permanent services
	if !strings.Contains(html, "grafana") {
		t.Error("expected permanent service 'grafana'")
	}
}

func TestGenerate_HidesStaleLeases(t *testing.T) {
	dir := t.TempDir()

	leases := []db.Lease{
		{
			Name: "api", Project: "tjst-t/palmux", Worktree: "main",
			Port: 8200, Hostname: "api--main--palmux", Expose: true, State: "active",
		},
		{
			Name: "worker", Project: "tjst-t/palmux", Worktree: "main",
			Port: 8201, Hostname: "worker--main--palmux", Expose: false, State: "stale",
		},
	}

	err := Generate(dir, leases, nil, "example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}

	html := string(content)

	// Active lease should be present
	if !strings.Contains(html, "8200") {
		t.Error("expected active lease port 8200")
	}

	// Stale lease should be hidden
	if strings.Contains(html, "8201") {
		t.Error("stale lease port 8201 should be hidden")
	}
	if strings.Contains(html, "worker") {
		t.Error("stale lease name 'worker' should be hidden")
	}
}

func TestGenerate_AllStaleShowsEmpty(t *testing.T) {
	dir := t.TempDir()

	leases := []db.Lease{
		{
			Name: "worker", Project: "tjst-t/palmux", Worktree: "main",
			Port: 8201, Hostname: "worker--main--palmux", Expose: false, State: "stale",
		},
	}

	err := Generate(dir, leases, nil, "example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), "No active leases") {
		t.Error("expected 'No active leases' when all leases are stale")
	}
}

func TestGenerate_EmptyLeases(t *testing.T) {
	dir := t.TempDir()

	err := Generate(dir, nil, nil, "example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), "No active leases") {
		t.Error("expected 'No active leases' message")
	}
}

func TestGenerate_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")

	err := Generate(dir, nil, nil, "example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "index.html")); os.IsNotExist(err) {
		t.Error("expected index.html to be created")
	}
}

func TestGenerate_XSSPrevention(t *testing.T) {
	dir := t.TempDir()

	leases := []db.Lease{
		{
			Name: "<script>alert('xss')</script>", Project: "org/repo",
			Worktree: "main", Port: 8200, Hostname: "test--main--repo",
			State: "active",
		},
	}

	err := Generate(dir, leases, nil, "example.com")
	if err != nil {
		t.Fatal(err)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	if strings.Contains(string(content), "<script>alert") {
		t.Error("HTML should be escaped to prevent XSS")
	}
}

func TestGenerate_GroupedByProject(t *testing.T) {
	dir := t.TempDir()

	leases := []db.Lease{
		{
			Name: "api", Project: "tjst-t/palmux", Worktree: "main",
			Port: 8200, Hostname: "api--main--palmux", Expose: true, State: "active",
		},
		{
			Name: "worker", Project: "tjst-t/palmux", Worktree: "main",
			Port: 8201, Hostname: "worker--main--palmux", Expose: false, State: "active",
		},
		{
			Name: "api", Project: "tjst-t/other", Worktree: "feature",
			Port: 8202, Hostname: "api--feature--other", Expose: true, State: "active",
		},
	}

	err := Generate(dir, leases, nil, "example.com")
	if err != nil {
		t.Fatal(err)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	html := string(content)

	// Both projects should appear
	if !strings.Contains(html, "tjst-t/palmux") {
		t.Error("expected project 'tjst-t/palmux'")
	}
	if !strings.Contains(html, "tjst-t/other") {
		t.Error("expected project 'tjst-t/other'")
	}

	// Branch names should appear
	if !strings.Contains(html, "main") {
		t.Error("expected branch 'main'")
	}
	if !strings.Contains(html, "feature") {
		t.Error("expected branch 'feature'")
	}
}

func TestGenerate_BranchLinkSingleLease(t *testing.T) {
	dir := t.TempDir()

	leases := []db.Lease{
		{
			Name: "api", Project: "tjst-t/palmux", Worktree: "main",
			Port: 8200, Hostname: "api--main--palmux", Expose: true, State: "active",
		},
	}

	err := Generate(dir, leases, nil, "example.com")
	if err != nil {
		t.Fatal(err)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	html := string(content)

	// Branch name should be a link when there's a single exposed lease
	if !strings.Contains(html, `<a href="https://api--main--palmux.example.com"`) {
		t.Error("expected branch name to be a link for single exposed lease")
	}
}

func TestGenerate_BranchNoLinkMultipleLeases(t *testing.T) {
	dir := t.TempDir()

	leases := []db.Lease{
		{
			Name: "api", Project: "tjst-t/palmux", Worktree: "main",
			Port: 8200, Hostname: "api--main--palmux", Expose: true, State: "active",
		},
		{
			Name: "worker", Project: "tjst-t/palmux", Worktree: "main",
			Port: 8201, Hostname: "worker--main--palmux", Expose: true, State: "active",
		},
	}

	err := Generate(dir, leases, nil, "example.com")
	if err != nil {
		t.Fatal(err)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	html := string(content)

	// Branch "main" in the branch-name div should NOT be a link (multiple leases)
	if strings.Contains(html, `branch-name"><a href`) {
		t.Error("branch name should not be a link when multiple leases exist")
	}
}

func TestGenerate_Responsive(t *testing.T) {
	dir := t.TempDir()

	err := Generate(dir, nil, nil, "example.com")
	if err != nil {
		t.Fatal(err)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	html := string(content)

	if !strings.Contains(html, "@media") {
		t.Error("expected responsive media query")
	}
	if !strings.Contains(html, "max-width: 640px") {
		t.Error("expected mobile breakpoint")
	}
}
