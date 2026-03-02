package cmd

import (
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "portman",
	Short: "Development port manager - DHCP-like port allocation with Caddy integration",
	Long:  `portman is a DHCP-like port management CLI tool for development environments. It allocates ports per worktree and integrates with Caddy for automatic reverse proxy setup.`,
}

// Execute runs the root command.
func Execute() error {
	rootCmd.Version = Version
	return rootCmd.Execute()
}
