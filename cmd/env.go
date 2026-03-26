package cmd

import (
	"fmt"
	"os"
	"strconv"
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
	envCmd.Flags().StringSlice("range", nil, "Allocate port range: name=count (e.g., libvirt-hosts=20)")
	rootCmd.AddCommand(envCmd)
}

// parseName parses a name entry which may have a ":expose" suffix.
// e.g., "dashboard:expose" -> ("dashboard", true), "api" -> ("api", false)
func parseName(entry string) (name string, expose bool) {
	if strings.HasSuffix(entry, ":expose") {
		return strings.TrimSuffix(entry, ":expose"), true
	}
	return entry, false
}

// parseRange parses a range entry in the form "name=count".
func parseRange(entry string) (name string, count int, err error) {
	parts := strings.SplitN(entry, "=", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid range format %q, expected name=count", entry)
	}
	name = parts[0]
	if name == "" {
		return "", 0, fmt.Errorf("range name cannot be empty")
	}
	count, err = strconv.Atoi(parts[1])
	if err != nil || count <= 0 {
		return "", 0, fmt.Errorf("range count must be a positive integer, got %q", parts[1])
	}
	return name, count, nil
}

// nameToEnvVar converts a service name to an environment variable name.
// e.g., "api" -> "API_PORT", "my-service" -> "MY_SERVICE_PORT"
func nameToEnvVar(name string) string {
	upper := strings.ToUpper(name)
	upper = strings.ReplaceAll(upper, "-", "_")
	return upper + "_PORT"
}

// nameToEnvVarRange converts a range name to start/end environment variable names.
// e.g., "libvirt-hosts" -> ("LIBVIRT_HOSTS_PORT_START", "LIBVIRT_HOSTS_PORT_END")
func nameToEnvVarRange(name string) (string, string) {
	upper := strings.ToUpper(name)
	upper = strings.ReplaceAll(upper, "-", "_")
	return upper + "_PORT_START", upper + "_PORT_END"
}

func runEnv(cmd *cobra.Command, args []string) error {
	names, _ := cmd.Flags().GetStringSlice("name")
	expose, _ := cmd.Flags().GetBool("expose")
	worktree, _ := cmd.Flags().GetString("worktree")
	output, _ := cmd.Flags().GetString("output")
	ranges, _ := cmd.Flags().GetStringSlice("range")

	app, err := initApp()
	if err != nil {
		return err
	}
	defer app.DB.Close()

	gitInfo, err := resolveGitInfo(worktree)
	if err != nil {
		return err
	}

	// Validate no name collision between --name and --range
	usedNames := make(map[string]bool)
	for _, entry := range names {
		name, _ := parseName(entry)
		if usedNames[name] {
			return fmt.Errorf("duplicate name %q in --name flags", name)
		}
		usedNames[name] = true
	}
	for _, entry := range ranges {
		name, _, err := parseRange(entry)
		if err != nil {
			return err
		}
		if usedNames[name] {
			return fmt.Errorf("name %q is used in both --name and --range", name)
		}
		if !usedNames[name] {
			usedNames[name] = true
		}
	}

	var lines []string

	// Process --name entries
	for _, entry := range names {
		name, perNameExpose := parseName(entry)
		result, err := app.Manager.Allocate(port.AllocateRequest{
			Project:      gitInfo.Project,
			Worktree:     gitInfo.Worktree,
			WorktreePath: gitInfo.WorktreePath,
			Repo:         gitInfo.Repo,
			Name:         name,
			Expose:       expose || perNameExpose,
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
		// Remove from Caddy if expose was removed
		if result.ExposeRemoved {
			if err := app.Caddy.RemoveRoute(result.Lease.Hostname); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to remove %s from Caddy: %v\n", name, err)
			}
		}

		envVar := nameToEnvVar(name)
		lines = append(lines, fmt.Sprintf("%s=%d", envVar, result.Lease.Port))
	}

	// Process --range entries
	for _, entry := range ranges {
		name, count, err := parseRange(entry)
		if err != nil {
			return err
		}

		result, err := app.Manager.AllocateRange(port.AllocateRangeRequest{
			Project:      gitInfo.Project,
			Worktree:     gitInfo.Worktree,
			WorktreePath: gitInfo.WorktreePath,
			Repo:         gitInfo.Repo,
			Name:         name,
			Count:        count,
		})
		if err != nil {
			return fmt.Errorf("allocating port range for %s: %w", name, err)
		}

		startVar, endVar := nameToEnvVarRange(name)
		lines = append(lines, fmt.Sprintf("%s=%d", startVar, result.Lease.Port))
		lines = append(lines, fmt.Sprintf("%s=%d", endVar, result.Lease.PortEnd))
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
