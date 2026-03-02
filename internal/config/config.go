package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	defaultConfigDir = "/etc/portman"
	configDirEnv     = "PORTMAN_CONFIG_DIR"
	configFileName   = "config.toml"
	servicesFileName = "services.json"
)

// Config represents the application configuration loaded from TOML.
type Config struct {
	General   GeneralConfig   `toml:"general"`
	Ports     PortsConfig     `toml:"ports"`
	Proxy     ProxyConfig     `toml:"proxy"`
	Dashboard DashboardConfig `toml:"dashboard"`
}

// GeneralConfig holds general settings.
type GeneralConfig struct {
	DBPath string `toml:"db_path"`
}

// PortsConfig holds port range and TTL settings.
type PortsConfig struct {
	RangeStart   int `toml:"range_start"`
	RangeEnd     int `toml:"range_end"`
	StaleTTLHours int `toml:"stale_ttl_hours"`
}

// ProxyConfig holds reverse proxy settings.
type ProxyConfig struct {
	Type         string `toml:"type"`
	CaddyAPI     string `toml:"caddy_api"`
	DomainSuffix string `toml:"domain_suffix"`
	HostPattern  string `toml:"host_pattern"`
}

// DashboardConfig holds dashboard settings.
type DashboardConfig struct {
	Enabled    bool   `toml:"enabled"`
	Host       string `toml:"host"`
	OutputDir  string `toml:"output_dir"`
	AutoUpdate bool   `toml:"auto_update"`
	ServeAddr  string `toml:"serve_addr"`
}

// Services represents static service definitions loaded from JSON.
type Services struct {
	Reserved  []ReservedPort    `json:"reserved"`
	Permanent []PermanentService `json:"permanent"`
}

// ReservedPort represents a protected port.
type ReservedPort struct {
	Port        int    `json:"port"`
	Description string `json:"description"`
}

// PermanentService represents a permanent service.
type PermanentService struct {
	Name   string `json:"name"`
	Port   int    `json:"port"`
	Expose bool   `json:"expose"`
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() Config {
	return Config{
		General: GeneralConfig{
			DBPath: "/var/lib/portman/portman.db",
		},
		Ports: PortsConfig{
			RangeStart:    8200,
			RangeEnd:      8999,
			StaleTTLHours: 24,
		},
		Proxy: ProxyConfig{
			Type:         "caddy",
			CaddyAPI:     "http://localhost:2019",
			DomainSuffix: "example.com",
			HostPattern:  "{name}--{worktree}--{repo}",
		},
		Dashboard: DashboardConfig{
			Enabled:    true,
			Host:       "portal.example.com",
			OutputDir:  "/var/lib/portman/portal",
			AutoUpdate: true,
			ServeAddr:  ":8080",
		},
	}
}

// configDir returns the configuration directory path.
func configDir() string {
	if dir := os.Getenv(configDirEnv); dir != "" {
		return dir
	}
	return defaultConfigDir
}

// LoadConfig reads the TOML config file. If the file does not exist,
// default values are returned.
func LoadConfig() (Config, error) {
	cfg := DefaultConfig()
	path := filepath.Join(configDir(), configFileName)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("reading config file: %w", err)
	}

	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing config file: %w", err)
	}

	return cfg, nil
}

// LoadServices reads the JSON services file. If the file does not exist,
// an empty Services is returned.
func LoadServices() (Services, error) {
	var svc Services
	path := filepath.Join(configDir(), servicesFileName)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return svc, nil
		}
		return svc, fmt.Errorf("reading services file: %w", err)
	}

	if err := json.Unmarshal(data, &svc); err != nil {
		return svc, fmt.Errorf("parsing services file: %w", err)
	}

	return svc, nil
}
