package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "portman",
	Short: "Development port manager - DHCP-like port allocation with Caddy integration",
	Long:  `portman is a DHCP-like port management CLI tool for development environments. It allocates ports per worktree and integrates with Caddy for automatic reverse proxy setup.`,
}

func Execute() error {
	return rootCmd.Execute()
}
