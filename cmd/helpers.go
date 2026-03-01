package cmd

import (
	"fmt"
	"os"

	"github.com/tjst-t/port-manager/internal/config"
	"github.com/tjst-t/port-manager/internal/dashboard"
	"github.com/tjst-t/port-manager/internal/db"
	gitpkg "github.com/tjst-t/port-manager/internal/git"
	"github.com/tjst-t/port-manager/internal/port"
	"github.com/tjst-t/port-manager/internal/proxy"
)

// appContext holds initialized components shared across commands.
type appContext struct {
	Config   config.Config
	Services config.Services
	DB       *db.DB
	Manager  *port.Manager
	Caddy    *proxy.CaddyClient
}

// initApp initializes config, services, DB, and port manager.
func initApp() (*appContext, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	svc, err := config.LoadServices()
	if err != nil {
		return nil, fmt.Errorf("loading services: %w", err)
	}

	database, err := db.Open(cfg.General.DBPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	mgr := &port.Manager{
		DB:       database,
		Config:   cfg,
		Services: svc,
	}

	caddy := proxy.NewCaddyClient(cfg.Proxy)

	return &appContext{
		Config:   cfg,
		Services: svc,
		DB:       database,
		Manager:  mgr,
		Caddy:    caddy,
	}, nil
}

// resolveGitInfo detects git information or uses the manual worktree flag.
func resolveGitInfo(worktreeFlag string) (gitpkg.Info, error) {
	info, err := gitpkg.Detect()
	if err != nil {
		if worktreeFlag == "" {
			return info, fmt.Errorf("not a git repository (or no remote): use --worktree to specify: %w", err)
		}
		// Non-git environment: use worktree flag
		return gitpkg.Info{
			Project:      "local",
			Repo:         "local",
			Worktree:     worktreeFlag,
			WorktreePath: mustGetwd(),
		}, nil
	}

	// Override worktree if flag is set (e.g., detached HEAD)
	if worktreeFlag != "" {
		info.Worktree = worktreeFlag
	}

	return info, nil
}

func mustGetwd() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

// maybeUpdateDashboard regenerates the dashboard if auto_update is enabled.
func maybeUpdateDashboard(app *appContext) {
	if !app.Config.Dashboard.Enabled || !app.Config.Dashboard.AutoUpdate {
		return
	}

	leases, err := app.DB.ListLeases()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to list leases for dashboard: %v\n", err)
		return
	}

	if err := dashboard.Generate(app.Config.Dashboard.OutputDir, leases, app.Services.Permanent, app.Config.Proxy.DomainSuffix); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to update dashboard: %v\n", err)
	}
}
