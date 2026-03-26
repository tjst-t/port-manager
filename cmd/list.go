package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tjst-t/port-manager/internal/db"
	"github.com/tjst-t/port-manager/internal/port"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all port leases",
	RunE:  runList,
}

func init() {
	listCmd.Flags().Bool("json", false, "Output in JSON format")
	listCmd.Flags().BoolP("current-dir", "c", false, "Show only leases for the current project/worktree")
	rootCmd.AddCommand(listCmd)
}

type listLeaseEntry struct {
	Name      string `json:"name"`
	Project   string `json:"project"`
	Worktree  string `json:"worktree"`
	Port      int    `json:"port"`
	PortEnd   int    `json:"port_end,omitempty"`
	PortCount int    `json:"port_count,omitempty"`
	Hostname  string `json:"hostname"`
	Expose    bool   `json:"expose"`
	Status    string `json:"status"`
	PID       int    `json:"pid,omitempty"`
	URL       string `json:"url"`
}

func runList(cmd *cobra.Command, args []string) error {
	jsonOutput, _ := cmd.Flags().GetBool("json")
	currentDir, _ := cmd.Flags().GetBool("current-dir")

	app, err := initApp()
	if err != nil {
		return err
	}
	defer app.DB.Close()

	var leases []db.Lease
	if currentDir {
		gitInfo, err := resolveGitInfo("")
		if err != nil {
			return err
		}
		leases, err = app.DB.FindLeasesByProjectWorktree(gitInfo.Project, gitInfo.Worktree)
		if err != nil {
			return fmt.Errorf("listing leases: %w", err)
		}
	} else {
		leases, err = app.DB.ListLeases()
		if err != nil {
			return fmt.Errorf("listing leases: %w", err)
		}
	}

	domainSuffix := app.Config.Proxy.DomainSuffix

	if jsonOutput {
		entries := make([]listLeaseEntry, len(leases))
		for i, l := range leases {
			entries[i] = listLeaseEntry{
				Name:      l.Name,
				Project:   l.Project,
				Worktree:  l.Worktree,
				Port:      l.Port,
				PortEnd:   l.PortEnd,
				PortCount: l.PortCount,
				Hostname:  l.Hostname,
				Expose:    l.Expose,
				Status:    plainStatus(l),
				PID:       l.PID,
				URL:       buildURL(l.Hostname, domainSuffix, l.Expose, l.Port),
			}
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(entries)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 3, ' ', 0)

	// Leases
	fmt.Fprintln(w, "PROJECT\tWORKTREE\tNAME\tPORT\tPID\tEXPOSE\tSTATUS")
	for _, l := range leases {
		expose := "no"
		if l.Expose {
			expose = "yes"
		}

		pid := "-"
		if l.PID > 0 {
			pid = strconv.Itoa(l.PID)
		}

		portStr := strconv.Itoa(l.Port)
		if l.IsRange() {
			portStr = fmt.Sprintf("%d-%d(%d)", l.Port, l.PortEnd, l.PortCount)
		}

		status := "○ stale"
		if l.State == "active" {
			if l.IsRange() {
				status = "● range"
			} else if port.IsPortListening(l.Port) {
				status = "● listening"
			} else {
				status = "○ not listening"
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			l.Project, l.Worktree, l.Name, portStr, pid, expose, status)
	}
	w.Flush()

	// Permanent services and reserved ports only in full list mode
	if !currentDir {
		if len(app.Services.Permanent) > 0 {
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(w, "PERMANENT:")
			fmt.Fprintln(w, "NAME\tPORT\tEXPOSE")
			for _, p := range app.Services.Permanent {
				expose := "no"
				if p.Expose {
					expose = "yes"
				}
				fmt.Fprintf(w, "%s\t%d\t%s\n", p.Name, p.Port, expose)
			}
			w.Flush()
		}

		if len(app.Services.Reserved) > 0 {
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(w, "RESERVED:")
			fmt.Fprintln(w, "PORT\tDESCRIPTION")
			for _, r := range app.Services.Reserved {
				fmt.Fprintf(w, "%d\t%s\n", r.Port, r.Description)
			}
			w.Flush()
		}
	}

	return nil
}

// plainStatus returns a plain text status string for JSON output.
func plainStatus(l db.Lease) string {
	if l.State == "active" {
		if l.IsRange() {
			return "range"
		}
		if port.IsPortListening(l.Port) {
			return "listening"
		}
		return "not listening"
	}
	return "stale"
}
