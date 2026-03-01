package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	execpkg "github.com/tjst-t/port-manager/internal/exec"
	"github.com/tjst-t/port-manager/internal/port"
)

var execCmd = &cobra.Command{
	Use:                "exec [flags] -- <command> [args...]",
	Short:              "Lease a port and run a command with {} replaced by the port number",
	DisableFlagParsing: false,
	RunE:               runExec,
}

func init() {
	execCmd.Flags().String("name", "default", "Service name")
	execCmd.Flags().Bool("expose", false, "Register with Caddy reverse proxy")
	execCmd.Flags().String("worktree", "", "Manual worktree name")
	rootCmd.AddCommand(execCmd)
}

func runExec(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("command is required after --")
	}

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

	// Lease a port (same as lease command)
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

	// Start the command (but don't register with Caddy yet)
	runner, err := execpkg.Start(args[0], args[1:], result.Lease.Port)
	if err != nil {
		return err
	}

	// Wait for startup grace period (2s) to confirm process stays alive
	alive, waitErr := runner.WaitStartup(2 * time.Second)
	if !alive {
		// Process exited during startup — skip Caddy registration, keep lease for retry
		return waitErr
	}

	// Process is alive — register with Caddy if expose is enabled
	if result.Lease.Expose {
		if err := app.Caddy.EnsureRoute(result.Lease.Hostname, result.Lease.Port); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to register with Caddy: %v\n", err)
		}
	}

	// Update dashboard
	maybeUpdateDashboard(app)

	// Wait for process to complete
	// No auto-release after command exits (reuse same port on restart)
	return runner.Wait()
}
