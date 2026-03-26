package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tjst-t/port-manager/internal/port"
)

var leaseCmd = &cobra.Command{
	Use:   "lease",
	Short: "Allocate a port for the current project/worktree",
	Long:  `Allocate a port and print the port number to stdout. Logs and warnings go to stderr.`,
	RunE:  runLease,
}

func init() {
	leaseCmd.Flags().String("name", "default", "Service name")
	leaseCmd.Flags().Bool("expose", false, "Register with Caddy reverse proxy")
	leaseCmd.Flags().String("worktree", "", "Manual worktree name (required in non-git dirs)")
	rootCmd.AddCommand(leaseCmd)
}

func runLease(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	expose, _ := cmd.Flags().GetBool("expose")
	worktree, _ := cmd.Flags().GetString("worktree")

	app, err := initApp()
	if err != nil {
		return err
	}
	defer app.DB.Close()

	gitInfo, err := resolveGitInfo(worktree)
	if err != nil {
		return err
	}

	// Run light GC if needed
	if gcResult, err := app.Manager.MaybeLightGC(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: light GC failed: %v\n", err)
	} else if gcResult != nil {
		for _, entry := range gcResult.WorktreeRemoved {
			fmt.Fprintf(os.Stderr, "gc: removed lease %s (worktree gone)\n", entry.Lease.Hostname)
			if entry.Lease.Expose {
				_ = app.Caddy.RemoveRoute(entry.Lease.Hostname)
			}
		}
		for _, entry := range gcResult.TTLExpired {
			fmt.Fprintf(os.Stderr, "gc: removed lease %s (TTL expired)\n", entry.Lease.Hostname)
			if entry.Lease.Expose {
				_ = app.Caddy.RemoveRoute(entry.Lease.Hostname)
			}
		}
	}

	result, err := app.Manager.Allocate(port.AllocateRequest{
		Project:      gitInfo.Project,
		Worktree:     gitInfo.Worktree,
		WorktreePath: gitInfo.WorktreePath,
		Repo:         gitInfo.Repo,
		Name:         name,
		Expose:       expose,
	})
	if err != nil {
		return err
	}

	// Register with Caddy if expose was added
	if result.ExposeAdded {
		if err := app.Caddy.AddRoute(result.Lease.Hostname, result.Lease.Port); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to register with Caddy (will recover on sync): %v\n", err)
		}
	}
	// Remove from Caddy if expose was removed
	if result.ExposeRemoved {
		if err := app.Caddy.RemoveRoute(result.Lease.Hostname); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to remove from Caddy: %v\n", err)
		}
	}

	// Update dashboard
	maybeUpdateDashboard(app)

	// Output port number to stdout
	fmt.Fprintln(cmd.OutOrStdout(), result.Lease.Port)
	return nil
}
