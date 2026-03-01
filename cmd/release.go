package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Release a port lease",
	RunE:  runRelease,
}

func init() {
	releaseCmd.Flags().String("name", "default", "Service name")
	releaseCmd.Flags().String("worktree", "", "Manual worktree name")
	rootCmd.AddCommand(releaseCmd)
}

func runRelease(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
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

	result, err := app.Manager.Release(gitInfo.Project, gitInfo.Worktree, name)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "released port %d (%s)\n", result.Port, result.Hostname)

	// Remove Caddy route if it was exposed
	if result.WasExpose {
		if err := app.Caddy.RemoveRoute(result.Hostname); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to remove Caddy route: %v\n", err)
		}
	}

	// Update dashboard
	maybeUpdateDashboard(app)

	return nil
}
