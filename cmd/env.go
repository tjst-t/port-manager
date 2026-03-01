package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tjst-t/port-manager/internal/port"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Lease ports for multiple services and output as NAME_PORT=XXXX",
	Long:  `Lease ports for multiple services and output environment variable assignments. Useful for Docker Compose integration.`,
	RunE:  runEnv,
}

func init() {
	envCmd.Flags().StringSlice("name", []string{"default"}, "Service name(s), can be specified multiple times")
	envCmd.Flags().Bool("expose", false, "Register with Caddy reverse proxy")
	envCmd.Flags().String("worktree", "", "Manual worktree name")
	envCmd.Flags().String("output", "", "Output file path (stdout if not specified)")
	rootCmd.AddCommand(envCmd)
}

// nameToEnvVar converts a service name to an environment variable name.
// e.g., "api" -> "API_PORT", "my-service" -> "MY_SERVICE_PORT"
func nameToEnvVar(name string) string {
	upper := strings.ToUpper(name)
	upper = strings.ReplaceAll(upper, "-", "_")
	return upper + "_PORT"
}

func runEnv(cmd *cobra.Command, args []string) error {
	names, _ := cmd.Flags().GetStringSlice("name")
	expose, _ := cmd.Flags().GetBool("expose")
	worktree, _ := cmd.Flags().GetString("worktree")
	output, _ := cmd.Flags().GetString("output")

	app, err := initApp()
	if err != nil {
		return err
	}
	defer app.DB.Close()

	gitInfo, err := resolveGitInfo(worktree)
	if err != nil {
		return err
	}

	var lines []string
	for _, name := range names {
		result, err := app.Manager.Allocate(port.AllocateRequest{
			Project:      gitInfo.Project,
			Worktree:     gitInfo.Worktree,
			WorktreePath: gitInfo.WorktreePath,
			Repo:         gitInfo.Repo,
			Name:         name,
			Expose:       expose,
		})
		if err != nil {
			return fmt.Errorf("allocating port for %s: %w", name, err)
		}

		// Register with Caddy if expose was added
		if result.ExposeAdded {
			if err := app.Caddy.AddRoute(result.Lease.Hostname, result.Lease.Port); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to register %s with Caddy: %v\n", name, err)
			}
		}

		envVar := nameToEnvVar(name)
		lines = append(lines, fmt.Sprintf("%s=%d", envVar, result.Lease.Port))
	}

	// Update dashboard
	maybeUpdateDashboard(app)

	content := strings.Join(lines, "\n") + "\n"

	if output != "" {
		if err := os.WriteFile(output, []byte(content), 0644); err != nil {
			return fmt.Errorf("writing output file: %w", err)
		}
		fmt.Fprintf(os.Stderr, "env: written to %s\n", output)
	} else {
		fmt.Fprint(cmd.OutOrStdout(), content)
	}

	return nil
}
