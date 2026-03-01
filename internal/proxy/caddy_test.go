package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tjst-t/port-manager/internal/config"
	"github.com/tjst-t/port-manager/internal/db"
)

func setupTestServer(t *testing.T, handler http.HandlerFunc) (*CaddyClient, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := NewCaddyClient(config.ProxyConfig{
		CaddyAPI:     server.URL,
		DomainSuffix: "example.com",
	})
	return client, server
}

func TestAddRoute(t *testing.T) {
	var receivedBody map[string]any

	client, _ := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/config/apps/http/servers/srv0/routes") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
	})

	err := client.AddRoute("api--main--palmux", 8234)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedBody["@id"] != "portman-api--main--palmux" {
		t.Errorf("unexpected @id: %v", receivedBody["@id"])
	}

	matchList, ok := receivedBody["match"].([]any)
	if !ok || len(matchList) == 0 {
		t.Fatal("expected match array")
	}
	matchObj := matchList[0].(map[string]any)
	hosts := matchObj["host"].([]any)
	if hosts[0] != "api--main--palmux.example.com" {
		t.Errorf("unexpected host: %v", hosts[0])
	}
}

func TestRemoveRoute(t *testing.T) {
	var deletedPath string

	client, _ := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletedPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	err := client.RemoveRoute("api--main--palmux")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if deletedPath != "/id/portman-api--main--palmux" {
		t.Errorf("unexpected delete path: %s", deletedPath)
	}
}

func TestAddRoute_APIError(t *testing.T) {
	client, _ := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	})

	err := client.AddRoute("api--main--palmux", 8234)
	if err == nil {
		t.Error("expected error for API failure")
	}
}

func TestRemoveRoute_APIError(t *testing.T) {
	client, _ := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	})

	err := client.RemoveRoute("api--main--palmux")
	if err == nil {
		t.Error("expected error for API failure")
	}
}

func TestIsAvailable_Up(t *testing.T) {
	client, _ := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	if !client.IsAvailable() {
		t.Error("expected IsAvailable=true")
	}
}

func TestIsAvailable_Down(t *testing.T) {
	client := NewCaddyClient(config.ProxyConfig{
		CaddyAPI: "http://localhost:19999", // unlikely to be running
	})
	client.client.Timeout = 100 * time.Millisecond

	if client.IsAvailable() {
		t.Error("expected IsAvailable=false")
	}
}

func TestEnsureRoute_DeleteNotFound(t *testing.T) {
	var requests []string

	client, _ := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodDelete {
			// Route doesn't exist yet — 404 is expected and ignored
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("not found"))
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	err := client.EnsureRoute("api--main--palmux", 8234)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(requests) != 2 {
		t.Fatalf("expected 2 requests (DELETE + POST), got %d: %v", len(requests), requests)
	}
	if requests[0] != "DELETE /id/portman-api--main--palmux" {
		t.Errorf("expected DELETE request first, got %s", requests[0])
	}
	if !strings.HasSuffix(requests[1], "/config/apps/http/servers/srv0/routes") || !strings.HasPrefix(requests[1], "POST") {
		t.Errorf("expected POST request second, got %s", requests[1])
	}
}

func TestEnsureRoute_DeleteSuccess(t *testing.T) {
	var requests []string

	client, _ := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})

	err := client.EnsureRoute("api--main--palmux", 8234)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(requests) != 2 {
		t.Fatalf("expected 2 requests (DELETE + POST), got %d: %v", len(requests), requests)
	}
	if requests[0] != "DELETE /id/portman-api--main--palmux" {
		t.Errorf("expected DELETE request first, got %s", requests[0])
	}
	if !strings.HasPrefix(requests[1], "POST") {
		t.Errorf("expected POST request second, got %s", requests[1])
	}
}

func TestSyncAll(t *testing.T) {
	var routes []string

	client, _ := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			routes = append(routes, body["@id"].(string))
		}
		w.WriteHeader(http.StatusOK)
	})

	leases := []db.Lease{
		{Hostname: "api--main--palmux", Port: 8200, Expose: true},
		{Hostname: "internal--main--palmux", Port: 8201, Expose: false},
	}

	permanents := []config.PermanentService{
		{Name: "grafana", Port: 3000, Expose: true},
		{Name: "prometheus", Port: 9090, Expose: false},
	}

	dashCfg := config.DashboardConfig{
		Enabled:   true,
		Host:      "portal.example.com",
		OutputDir: "/var/lib/portman/portal",
	}

	err := client.SyncAll(leases, permanents, dashCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have: 1 expose lease + 1 expose permanent + 1 dashboard = 3 routes
	if len(routes) != 3 {
		t.Errorf("expected 3 routes added, got %d: %v", len(routes), routes)
	}
}
