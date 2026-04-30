package cmd

import (
	"fmt"

	"github.com/useamaru/amaru/internal/manifest"

	"github.com/spf13/cobra"
)

var ignoreCmd = &cobra.Command{
	Use:   "ignore <name>",
	Short: "Mark item as accepted drift",
	Long:  "Mark a skill/command as 'accepted drift' — suppresses hash mismatch warnings.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runIgnore(args[0])
	},
}

var unignoreCmd = &cobra.Command{
	Use:   "unignore <name>",
	Short: "Remove item from accepted drift list",
	Long:  "Remove a skill/command from the ignored list, re-enabling drift reporting.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUnignore(args[0])
	},
}

func init() {
	rootCmd.AddCommand(ignoreCmd)
	rootCmd.AddCommand(unignoreCmd)
}

func runIgnore(name string) error {
	m, err := loadManifest()
	if err != nil {
		return err
	}

	// Check if already ignored
	if m.IsIgnored(name) {
		return fmt.Errorf("%s is already in the ignored list", name)
	}

	// Check if item exists in manifest
	if !m.HasDep(name) {
		return fmt.Errorf("%s not found in manifest", name)
	}

	m.Ignored = append(m.Ignored, name)

	if err := manifest.Save(".", m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	fmt.Printf("%s marked as accepted drift. Will not be reported during check.\n", name)
	fmt.Printf("To revert: amaru unignore %s\n", name)
	return nil
}

func runUnignore(name string) error {
	m, err := loadManifest()
	if err != nil {
		return err
	}

	if !m.IsIgnored(name) {
		return fmt.Errorf("%s is not in the ignored list", name)
	}

	var newIgnored []string
	for _, ignored := range m.Ignored {
		if ignored != name {
			newIgnored = append(newIgnored, ignored)
		}
	}
	m.Ignored = newIgnored

	if err := manifest.Save(".", m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	fmt.Printf("%s removed from ignored list. Drift will be reported during check.\n", name)
	return nil
}
