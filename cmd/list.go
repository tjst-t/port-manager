package cmd

import (
	"fmt"
	"net"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all port leases",
	RunE:  runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	app, err := initApp()
	if err != nil {
		return err
	}
	defer app.DB.Close()

	leases, err := app.DB.ListLeases()
	if err != nil {
		return fmt.Errorf("listing leases: %w", err)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 3, ' ', 0)

	// Leases
	fmt.Fprintln(w, "PROJECT\tWORKTREE\tNAME\tPORT\tEXPOSE\tSTATUS")
	for _, l := range leases {
		expose := "no"
		if l.Expose {
			expose = "yes"
		}

		status := "○ stale"
		if l.State == "active" {
			if isListening(l.Port) {
				status = "● listening"
			} else {
				status = "○ not listening"
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n",
			l.Project, l.Worktree, l.Name, l.Port, expose, status)
	}
	w.Flush()

	// Permanent services
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

	// Reserved ports
	if len(app.Services.Reserved) > 0 {
		fmt.Fprintln(cmd.OutOrStdout())
		fmt.Fprintln(w, "RESERVED:")
		fmt.Fprintln(w, "PORT\tDESCRIPTION")
		for _, r := range app.Services.Reserved {
			fmt.Fprintf(w, "%d\t%s\n", r.Port, r.Description)
		}
		w.Flush()
	}

	return nil
}

func isListening(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
