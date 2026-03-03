package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var urlCmd = &cobra.Command{
	Use:   "url",
	Short: "Show URL for leased services in the current worktree",
	Long:  `Show the URL for leased services. If --name is specified, shows a single URL. Otherwise, lists all services in the current worktree.`,
	RunE:  runURL,
}

func init() {
	urlCmd.Flags().String("name", "", "Service name (shows all services if not specified)")
	urlCmd.Flags().String("worktree", "", "Manual worktree name (required in non-git dirs)")
	urlCmd.Flags().Bool("json", false, "Output in JSON format")
	rootCmd.AddCommand(urlCmd)
}

type urlEntry struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Port   int    `json:"port"`
	Expose bool   `json:"expose"`
}

func buildURL(hostname, domainSuffix string, expose bool, port int) string {
	if expose {
		return "https://" + hostname + "." + domainSuffix
	}
	return fmt.Sprintf("http://localhost:%d", port)
}

func runURL(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	worktree, _ := cmd.Flags().GetString("worktree")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	app, err := initApp()
	if err != nil {
		return err
	}
	defer app.DB.Close()

	gitInfo, err := resolveGitInfo(worktree)
	if err != nil {
		return err
	}

	domainSuffix := app.Config.Proxy.DomainSuffix

	if name != "" {
		// Single service lookup
		lease, err := app.DB.FindLease(gitInfo.Project, gitInfo.Worktree, name)
		if err != nil {
			return fmt.Errorf("finding lease: %w", err)
		}
		if lease == nil {
			return fmt.Errorf("no lease found for service %q in current worktree", name)
		}

		url := buildURL(lease.Hostname, domainSuffix, lease.Expose, lease.Port)

		if jsonOutput {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(urlEntry{
				Name:   lease.Name,
				URL:    url,
				Port:   lease.Port,
				Expose: lease.Expose,
			})
		}

		fmt.Fprintln(cmd.OutOrStdout(), url)
		return nil
	}

	// All services in current worktree
	leases, err := app.DB.FindLeasesByProjectWorktree(gitInfo.Project, gitInfo.Worktree)
	if err != nil {
		return fmt.Errorf("finding leases: %w", err)
	}
	if len(leases) == 0 {
		return fmt.Errorf("no leases found for current worktree")
	}

	if jsonOutput {
		entries := make([]urlEntry, len(leases))
		for i, l := range leases {
			entries[i] = urlEntry{
				Name:   l.Name,
				URL:    buildURL(l.Hostname, domainSuffix, l.Expose, l.Port),
				Port:   l.Port,
				Expose: l.Expose,
			}
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(entries)
	}

	for _, l := range leases {
		url := buildURL(l.Hostname, domainSuffix, l.Expose, l.Port)
		fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", l.Name, url)
	}

	return nil
}
