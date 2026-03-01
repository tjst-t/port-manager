package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Run garbage collection on stale and expired leases",
	RunE:  runGC,
}

func init() {
	rootCmd.AddCommand(gcCmd)
}

func runGC(cmd *cobra.Command, args []string) error {
	app, err := initApp()
	if err != nil {
		return err
	}
	defer app.DB.Close()

	result, err := app.Manager.RunGC()
	if err != nil {
		return fmt.Errorf("running GC: %w", err)
	}

	for _, l := range result.WorktreeRemoved {
		fmt.Fprintf(os.Stderr, "removed: %s:%s:%s (port %d) — worktree path gone\n",
			l.Project, l.Worktree, l.Name, l.Port)
		if l.Expose {
			if err := app.Caddy.RemoveRoute(l.Hostname); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to remove Caddy route for %s: %v\n", l.Hostname, err)
			}
		}
	}

	for _, l := range result.StaleMarked {
		fmt.Fprintf(os.Stderr, "stale: %s:%s:%s (port %d) — not listening\n",
			l.Project, l.Worktree, l.Name, l.Port)
	}

	for _, l := range result.TTLExpired {
		fmt.Fprintf(os.Stderr, "expired: %s:%s:%s (port %d) — TTL exceeded\n",
			l.Project, l.Worktree, l.Name, l.Port)
		if l.Expose {
			if err := app.Caddy.RemoveRoute(l.Hostname); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to remove Caddy route for %s: %v\n", l.Hostname, err)
			}
		}
	}

	total := len(result.WorktreeRemoved) + len(result.StaleMarked) + len(result.TTLExpired)
	if total == 0 {
		fmt.Fprintln(os.Stderr, "gc: nothing to clean up")
	}

	// Update dashboard
	maybeUpdateDashboard(app)

	return nil
}
