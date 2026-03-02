package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tjst-t/port-manager/internal/config"
	"github.com/tjst-t/port-manager/internal/db"
)

// CaddyClient manages interactions with the Caddy Admin API.
type CaddyClient struct {
	apiURL       string
	domainSuffix string
	client       *http.Client
}

// NewCaddyClient creates a CaddyClient from the proxy config.
func NewCaddyClient(cfg config.ProxyConfig) *CaddyClient {
	return &CaddyClient{
		apiURL:       cfg.CaddyAPI,
		domainSuffix: cfg.DomainSuffix,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// caddyRoute represents a Caddy route configuration.
type caddyRoute struct {
	ID     string        `json:"@id"`
	Match  []caddyMatch  `json:"match"`
	Handle []caddyHandle `json:"handle"`
}

type caddyMatch struct {
	Host []string `json:"host"`
}

type caddyHandle struct {
	Handler   string          `json:"handler"`
	Upstreams []caddyUpstream `json:"upstreams"`
}

type caddyUpstream struct {
	Dial string `json:"dial"`
}

// routeID returns the @id for a portman-managed route.
func routeID(hostname string) string {
	return "portman-" + hostname
}

// AddRoute registers a reverse proxy route in Caddy for the given hostname and port.
func (c *CaddyClient) AddRoute(hostname string, port int) error {
	fqdn := hostname + "." + c.domainSuffix

	route := caddyRoute{
		ID: routeID(hostname),
		Match: []caddyMatch{
			{Host: []string{fqdn}},
		},
		Handle: []caddyHandle{
			{
				Handler:   "reverse_proxy",
				Upstreams: []caddyUpstream{{Dial: fmt.Sprintf("localhost:%d", port)}},
			},
		},
	}

	body, err := json.Marshal(route)
	if err != nil {
		return fmt.Errorf("marshaling route: %w", err)
	}

	url := c.apiURL + "/config/apps/http/servers/srv0/routes"
	resp, err := c.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("adding Caddy route: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Caddy API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// RemoveRoute removes a portman-managed route from Caddy by its @id.
func (c *CaddyClient) RemoveRoute(hostname string) error {
	id := routeID(hostname)
	url := c.apiURL + "/id/" + id

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("creating delete request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("removing Caddy route: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Caddy API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// EnsureRoute idempotently registers a reverse proxy route in Caddy.
// It removes any existing route with the same @id before adding the new one,
// preventing duplicate routes.
func (c *CaddyClient) EnsureRoute(hostname string, port int) error {
	_ = c.RemoveRoute(hostname)
	return c.AddRoute(hostname, port)
}

// SyncAll removes all portman-managed routes and re-registers all expose leases
// and expose permanent services. If dashboard is enabled, its route is also added.
func (c *CaddyClient) SyncAll(leases []db.Lease, permanents []config.PermanentService, dashCfg config.DashboardConfig) error {
	// Remove existing portman routes
	for _, lease := range leases {
		// Ignore errors during cleanup — route may not exist
		_ = c.RemoveRoute(lease.Hostname)
	}
	for _, perm := range permanents {
		_ = c.RemoveRoute(perm.Name)
	}

	// Re-register expose leases
	for _, lease := range leases {
		if !lease.Expose {
			continue
		}
		if err := c.AddRoute(lease.Hostname, lease.Port); err != nil {
			return fmt.Errorf("syncing lease route %s: %w", lease.Hostname, err)
		}
	}

	// Re-register expose permanents
	for _, perm := range permanents {
		if !perm.Expose {
			continue
		}
		if err := c.AddRoute(perm.Name, perm.Port); err != nil {
			return fmt.Errorf("syncing permanent route %s: %w", perm.Name, err)
		}
	}

	// Add dashboard route if enabled
	if dashCfg.Enabled && dashCfg.Host != "" {
		if err := c.addDashboardRoute(dashCfg); err != nil {
			return fmt.Errorf("syncing dashboard route: %w", err)
		}
	}

	return nil
}

// addDashboardRoute registers the dashboard route.
// If ServeAddr is configured, uses reverse_proxy to the serve process.
// Otherwise, uses file_server with the static output directory.
func (c *CaddyClient) addDashboardRoute(dashCfg config.DashboardConfig) error {
	var handle []map[string]any
	if dashCfg.ServeAddr != "" {
		dial := normalizeAddr(dashCfg.ServeAddr)
		handle = []map[string]any{
			{
				"handler":   "reverse_proxy",
				"upstreams": []map[string]string{{"dial": dial}},
			},
		}
	} else {
		handle = []map[string]any{
			{
				"handler": "file_server",
				"root":    dashCfg.OutputDir,
			},
		}
	}

	route := map[string]any{
		"@id":    "portman-dashboard",
		"match":  []map[string]any{{"host": []string{dashCfg.Host}}},
		"handle": handle,
	}

	body, err := json.Marshal(route)
	if err != nil {
		return fmt.Errorf("marshaling dashboard route: %w", err)
	}

	// Remove existing dashboard route first
	_ = c.RemoveRoute("dashboard")

	url := c.apiURL + "/config/apps/http/servers/srv0/routes"
	resp, err := c.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("adding dashboard route: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Caddy API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// normalizeAddr converts ":8080" to "localhost:8080".
func normalizeAddr(addr string) string {
	if len(addr) > 0 && addr[0] == ':' {
		return "localhost" + addr
	}
	return addr
}

// IsAvailable checks if the Caddy Admin API is reachable.
func (c *CaddyClient) IsAvailable() bool {
	resp, err := c.client.Get(c.apiURL + "/config/")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}
