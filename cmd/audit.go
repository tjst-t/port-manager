package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Detect unmanaged port usage in the configured range",
	RunE:  runAudit,
}

func init() {
	rootCmd.AddCommand(auditCmd)
}

func runAudit(cmd *cobra.Command, args []string) error {
	app, err := initApp()
	if err != nil {
		return err
	}
	defer app.DB.Close()

	results, err := app.Manager.Audit()
	if err != nil {
		return fmt.Errorf("running audit: %w", err)
	}

	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "audit: no unmanaged ports detected")
		return nil
	}

	// Try to get process info via ss
	processInfo := getProcessInfo()

	for _, r := range results {
		info := ""
		if pi, ok := processInfo[r.Port]; ok {
			info = fmt.Sprintf(" (PID %s: %s)", pi.pid, pi.name)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "WARNING: port %d is in use but not managed%s\n", r.Port, info)
	}

	return nil
}

type procInfo struct {
	pid  string
	name string
}

// ssPortPattern matches port numbers from ss output
var ssPortPattern = regexp.MustCompile(`:(\d+)\s`)

// ssProcPattern matches process info from ss -p output
var ssProcPattern = regexp.MustCompile(`users:\(\("([^"]*)",pid=(\d+)`)

func getProcessInfo() map[int]procInfo {
	result := make(map[int]procInfo)

	out, err := exec.Command("ss", "-tlnp").Output()
	if err != nil {
		return result
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if !strings.Contains(line, "LISTEN") {
			continue
		}

		// Extract port from local address
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		localAddr := fields[3]
		idx := strings.LastIndex(localAddr, ":")
		if idx < 0 {
			continue
		}
		portStr := localAddr[idx+1:]
		var port int
		if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
			continue
		}

		// Extract process info
		if matches := ssProcPattern.FindStringSubmatch(line); matches != nil {
			result[port] = procInfo{
				name: matches[1],
				pid:  matches[2],
			}
		}
	}

	return result
}
