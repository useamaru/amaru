package cmd

import (
	"context"
	"fmt"

	"github.com/barelias/amaru/internal/manifest"
	"github.com/barelias/amaru/internal/registry"
	"github.com/barelias/amaru/internal/ui"
)

// loadManifest loads the manifest from the current directory.
func loadManifest() (*manifest.Manifest, error) {
	m, err := manifest.Load(".")
	if err != nil {
		return nil, fmt.Errorf("failed to load amaru.json: %w\nRun 'amaru init' to create one", err)
	}
	return m, nil
}

// loadLock loads the lock file from the current directory.
func loadLock() (*manifest.Lock, error) {
	return manifest.LoadLock(".")
}

// buildClients creates registry clients for all registries in the manifest.
// Authenticates each one and prints status.
// Each client is configured with a mirror factory so that mirrors listed in
// amaru_registry.json are automatically fetched and merged on FetchIndex.
func buildClients(ctx context.Context, m *manifest.Manifest, silent bool) (map[string]registry.Client, error) {
	if !silent {
		fmt.Println("Authenticating registries...")
	}

	// Mirror factory: creates unauthenticated clients for mirror URLs.
	mirrorFactory := func(url string) (registry.Client, error) {
		noAuth, err := registry.NewAuthenticator("none", "")
		if err != nil {
			return nil, err
		}
		return registry.NewGitHubClient(url, noAuth)
	}

	clients := make(map[string]registry.Client)
	for alias, regConf := range m.Registries {
		auth, err := registry.NewAuthenticator(regConf.Auth, alias)
		if err != nil {
			return nil, fmt.Errorf("registry %s: %w", alias, err)
		}

		client, err := registry.NewGitHubClient(regConf.URL, auth)
		if err != nil {
			return nil, fmt.Errorf("registry %s: %w", alias, err)
		}

		client.WithMirrorFactory(mirrorFactory)

		// Validate authentication
		if _, err := auth.Token(ctx); err != nil && regConf.Auth != "none" {
			return nil, fmt.Errorf("registry %s authentication failed: %w", alias, err)
		}

		clients[alias] = client

		if !silent {
			ui.Check("%s (%s) — via %s", alias, regConf.URL, auth.Method())
		}
	}

	return clients, nil
}
