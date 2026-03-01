package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.General.DBPath != "/var/lib/portman/portman.db" {
		t.Errorf("unexpected DBPath: %s", cfg.General.DBPath)
	}
	if cfg.Ports.RangeStart != 8200 {
		t.Errorf("unexpected RangeStart: %d", cfg.Ports.RangeStart)
	}
	if cfg.Ports.RangeEnd != 8999 {
		t.Errorf("unexpected RangeEnd: %d", cfg.Ports.RangeEnd)
	}
	if cfg.Ports.StaleTTLHours != 24 {
		t.Errorf("unexpected StaleTTLHours: %d", cfg.Ports.StaleTTLHours)
	}
	if cfg.Proxy.Type != "caddy" {
		t.Errorf("unexpected Proxy.Type: %s", cfg.Proxy.Type)
	}
	if cfg.Proxy.CaddyAPI != "http://localhost:2019" {
		t.Errorf("unexpected CaddyAPI: %s", cfg.Proxy.CaddyAPI)
	}
	if cfg.Proxy.DomainSuffix != "example.com" {
		t.Errorf("unexpected DomainSuffix: %s", cfg.Proxy.DomainSuffix)
	}
	if cfg.Proxy.HostPattern != "{name}--{worktree}--{repo}" {
		t.Errorf("unexpected HostPattern: %s", cfg.Proxy.HostPattern)
	}
	if !cfg.Dashboard.Enabled {
		t.Error("expected Dashboard.Enabled to be true")
	}
	if !cfg.Dashboard.AutoUpdate {
		t.Error("expected Dashboard.AutoUpdate to be true")
	}
}

func TestLoadConfig_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(configDirEnv, dir)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return defaults
	expected := DefaultConfig()
	if cfg.General.DBPath != expected.General.DBPath {
		t.Errorf("expected default DBPath, got %s", cfg.General.DBPath)
	}
	if cfg.Ports.RangeStart != expected.Ports.RangeStart {
		t.Errorf("expected default RangeStart, got %d", cfg.Ports.RangeStart)
	}
}

func TestLoadConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(configDirEnv, dir)

	content := `
[general]
db_path = "/tmp/test.db"

[ports]
range_start = 9000
range_end = 9999
stale_ttl_hours = 48

[proxy]
type = "caddy"
caddy_api = "http://localhost:3019"
domain_suffix = "test.example.com"
host_pattern = "{name}--{worktree}--{repo}"

[dashboard]
enabled = false
host = "dash.test.example.com"
output_dir = "/tmp/portal"
auto_update = false
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.General.DBPath != "/tmp/test.db" {
		t.Errorf("unexpected DBPath: %s", cfg.General.DBPath)
	}
	if cfg.Ports.RangeStart != 9000 {
		t.Errorf("unexpected RangeStart: %d", cfg.Ports.RangeStart)
	}
	if cfg.Ports.RangeEnd != 9999 {
		t.Errorf("unexpected RangeEnd: %d", cfg.Ports.RangeEnd)
	}
	if cfg.Ports.StaleTTLHours != 48 {
		t.Errorf("unexpected StaleTTLHours: %d", cfg.Ports.StaleTTLHours)
	}
	if cfg.Proxy.CaddyAPI != "http://localhost:3019" {
		t.Errorf("unexpected CaddyAPI: %s", cfg.Proxy.CaddyAPI)
	}
	if cfg.Dashboard.Enabled {
		t.Error("expected Dashboard.Enabled to be false")
	}
	if cfg.Dashboard.AutoUpdate {
		t.Error("expected Dashboard.AutoUpdate to be false")
	}
}

func TestLoadConfig_PartialOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(configDirEnv, dir)

	content := `
[ports]
range_start = 7000
`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Ports.RangeStart != 7000 {
		t.Errorf("expected RangeStart=7000, got %d", cfg.Ports.RangeStart)
	}
	// Other defaults should remain
	if cfg.General.DBPath != "/var/lib/portman/portman.db" {
		t.Errorf("expected default DBPath, got %s", cfg.General.DBPath)
	}
}

func TestLoadServices_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(configDirEnv, dir)

	svc, err := LoadServices()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(svc.Reserved) != 0 {
		t.Errorf("expected empty reserved, got %d", len(svc.Reserved))
	}
	if len(svc.Permanent) != 0 {
		t.Errorf("expected empty permanent, got %d", len(svc.Permanent))
	}
}

func TestLoadServices_ValidFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(configDirEnv, dir)

	content := `{
  "reserved": [
    { "port": 80, "description": "caddy http" },
    { "port": 443, "description": "caddy https" }
  ],
  "permanent": [
    { "name": "grafana", "port": 3000, "expose": true }
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, servicesFileName), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	svc, err := LoadServices()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(svc.Reserved) != 2 {
		t.Fatalf("expected 2 reserved, got %d", len(svc.Reserved))
	}
	if svc.Reserved[0].Port != 80 {
		t.Errorf("unexpected port: %d", svc.Reserved[0].Port)
	}
	if svc.Reserved[0].Description != "caddy http" {
		t.Errorf("unexpected description: %s", svc.Reserved[0].Description)
	}

	if len(svc.Permanent) != 1 {
		t.Fatalf("expected 1 permanent, got %d", len(svc.Permanent))
	}
	if svc.Permanent[0].Name != "grafana" {
		t.Errorf("unexpected name: %s", svc.Permanent[0].Name)
	}
	if svc.Permanent[0].Port != 3000 {
		t.Errorf("unexpected port: %d", svc.Permanent[0].Port)
	}
	if !svc.Permanent[0].Expose {
		t.Error("expected expose to be true")
	}
}

func TestLoadConfig_InvalidTOML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(configDirEnv, dir)

	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte("invalid[toml"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

func TestLoadServices_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(configDirEnv, dir)

	if err := os.WriteFile(filepath.Join(dir, servicesFileName), []byte("{invalid}"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadServices()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
