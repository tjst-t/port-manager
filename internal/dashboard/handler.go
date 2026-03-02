package dashboard

import (
	"fmt"
	"net/http"
	"os"

	"github.com/tjst-t/port-manager/internal/config"
	"github.com/tjst-t/port-manager/internal/db"
)

// Handler serves the live dashboard over HTTP.
type Handler struct {
	DB           *db.DB
	Services     config.Services
	ProxyConfig  config.ProxyConfig
	StatusCheck  StatusChecker
	Version      string
}

// NewHandler creates a new dashboard HTTP handler.
func NewHandler(database *db.DB, services config.Services, proxyCfg config.ProxyConfig, checker StatusChecker, version string) *Handler {
	return &Handler{
		DB:          database,
		Services:    services,
		ProxyConfig: proxyCfg,
		StatusCheck: checker,
		Version:     version,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	leases, err := h.DB.ListLeases()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to list leases: %v", err), http.StatusInternalServerError)
		fmt.Fprintf(os.Stderr, "dashboard handler: failed to list leases: %v\n", err)
		return
	}

	data := BuildDashboardData(leases, h.Services.Permanent, h.ProxyConfig.DomainSuffix, h.StatusCheck, true, h.Version)
	html, err := RenderHTML(data)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to render dashboard: %v", err), http.StatusInternalServerError)
		fmt.Fprintf(os.Stderr, "dashboard handler: failed to render: %v\n", err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
