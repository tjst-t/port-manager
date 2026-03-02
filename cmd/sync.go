package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tjst-t/port-manager/internal/dashboard"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync all expose leases to Caddy and regenerate dashboard",
	RunE:  runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	app, err := initApp()
	if err != nil {
		return err
	}
	defer app.DB.Close()

	leases, err := app.DB.ListLeases()
	if err != nil {
		return fmt.Errorf("listing leases: %w", err)
	}

	if err := app.Caddy.SyncAll(leases, app.Services.Permanent, app.Config.Dashboard); err != nil {
		return fmt.Errorf("syncing to Caddy: %w", err)
	}

	fmt.Fprintln(os.Stderr, "sync: all routes synced to Caddy")

	// Regenerate dashboard
	if app.Config.Dashboard.Enabled {
		if err := dashboard.Generate(app.Config.Dashboard.OutputDir, leases, app.Services.Permanent, app.Config.Proxy.DomainSuffix, Version); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to regenerate dashboard: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "sync: dashboard regenerated")
		}
	}

	return nil
}
