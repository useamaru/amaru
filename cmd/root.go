package cmd

import (
	"github.com/spf13/cobra"
)

const version = "0.2.6"

var rootCmd = &cobra.Command{
	Use:   "amaru",
	Short: "Skills and commands manager for Claude Code",
	Long: `amaru manages skills and commands for Claude Code via a manifest file (amaru.json).
Supports multiple registries (public and private), checks for updates,
and warns when newer versions are available.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.Version = version
}
