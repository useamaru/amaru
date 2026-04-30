package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/useamaru/amaru/internal/manifest"
	"github.com/useamaru/amaru/internal/registry"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate initial amaru.json interactively",
	Long:  "Create a new amaru.json with interactively configured registries.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInit()
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit() error {
	// Check if manifest already exists
	if _, err := os.Stat(manifest.ManifestFile); err == nil {
		return fmt.Errorf("amaru.json already exists. Delete it first to re-initialize")
	}

	reader := bufio.NewReader(os.Stdin)
	m := &manifest.Manifest{
		Version:    "1.0.0",
		Registries: make(map[string]manifest.RegistryConfig),
		Skills:     make(map[string]manifest.DependencySpec),
		Commands:   make(map[string]manifest.DependencySpec),
		Agents:     make(map[string]manifest.DependencySpec),
	}

	for {
		// Registry URL
		fmt.Print("Registry URL (ex: github:org/skills-repo): ")
		rawURL, _ := reader.ReadString('\n')
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			return fmt.Errorf("registry URL is required")
		}

		// Normalize URL to canonical github:org/repo format
		url, err := registry.NormalizeURL(rawURL)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		if url != rawURL {
			fmt.Printf("  → normalized to: %s\n", url)
		}

		// Registry alias (suggest from URL)
		suggested := suggestAlias(url)
		fmt.Printf("Registry alias [%s]: ", suggested)
		alias, _ := reader.ReadString('\n')
		alias = strings.TrimSpace(alias)
		if alias == "" {
			alias = suggested
		}

		// Auth method
		fmt.Print("Auth method (github/token/none) [github]: ")
		auth, _ := reader.ReadString('\n')
		auth = strings.TrimSpace(auth)
		if auth == "" {
			auth = "github"
		}
		if auth != "github" && auth != "token" && auth != "none" {
			return fmt.Errorf("invalid auth method: %s", auth)
		}

		m.Registries[alias] = manifest.RegistryConfig{
			URL:  url,
			Auth: auth,
		}

		// Add another?
		fmt.Print("\nAdd another registry? (y/N): ")
		another, _ := reader.ReadString('\n')
		another = strings.TrimSpace(strings.ToLower(another))
		if another != "y" && another != "yes" {
			break
		}
		fmt.Println()
	}

	if err := manifest.Save(".", m); err != nil {
		return fmt.Errorf("saving manifest: %w", err)
	}

	fmt.Printf("\namaru.json created. Run `amaru browse` to see available skills.\n")
	return nil
}

func suggestAlias(url string) string {
	// For "github:org/repo-name", suggest the part after the last / without "-skills" suffix
	url = strings.TrimPrefix(url, "github:")
	url = strings.TrimPrefix(url, "https://github.com/")
	parts := strings.Split(url, "/")
	if len(parts) >= 2 {
		name := parts[len(parts)-1]
		name = strings.TrimSuffix(name, "-skills")
		name = strings.TrimSuffix(name, ".git")
		return name
	}
	return "default"
}
