package cmd

import (
	"github.com/spf13/cobra"
)

// Sobrescrita no release pelo ldflags do GoReleaser (-X ...cmd.version);
// o valor aqui vale para builds de desenvolvimento e é o que o check-version
// da CI exige bumpar a cada PR.
var version = "0.6.1"

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
