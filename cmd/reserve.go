package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/tjst-t/port-manager/internal/config"
)

var reserveCmd = &cobra.Command{
	Use:   "reserve <port>",
	Short: "Reserve a port (add to services.json protected ports)",
	Args:  cobra.ExactArgs(1),
	RunE:  runReserve,
}

func init() {
	reserveCmd.Flags().String("description", "", "Description for the reserved port")
	rootCmd.AddCommand(reserveCmd)
}

func runReserve(cmd *cobra.Command, args []string) error {
	portNum, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid port number: %s", args[0])
	}

	description, _ := cmd.Flags().GetString("description")

	app, err := initApp()
	if err != nil {
		return err
	}
	defer app.DB.Close()

	// Check if already reserved
	for _, r := range app.Services.Reserved {
		if r.Port == portNum {
			fmt.Fprintf(os.Stderr, "warning: port %d is already reserved (%s)\n", portNum, r.Description)
			return nil
		}
	}

	// Check if port is currently leased
	allocatedPorts, err := app.DB.AllocatedPorts()
	if err != nil {
		return fmt.Errorf("checking allocated ports: %w", err)
	}
	for _, p := range allocatedPorts {
		if p == portNum {
			return fmt.Errorf("port %d is currently leased — release it first", portNum)
		}
	}

	// Add to services.json
	app.Services.Reserved = append(app.Services.Reserved, config.ReservedPort{
		Port:        portNum,
		Description: description,
	})

	if err := writeServicesJSON(app.Services); err != nil {
		return fmt.Errorf("writing services.json: %w", err)
	}

	fmt.Fprintf(os.Stderr, "reserved port %d", portNum)
	if description != "" {
		fmt.Fprintf(os.Stderr, " (%s)", description)
	}
	fmt.Fprintln(os.Stderr)

	return nil
}

func writeServicesJSON(svc config.Services) error {
	configDir := os.Getenv("PORTMAN_CONFIG_DIR")
	if configDir == "" {
		configDir = "/etc/portman"
	}

	data, err := json.MarshalIndent(svc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling services: %w", err)
	}

	path := filepath.Join(configDir, "services.json")
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}
