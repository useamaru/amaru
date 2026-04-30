package installer

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/useamaru/amaru/internal/registry"
)

const (
	SkillsDir   = ".claude/skills"
	CommandsDir = ".claude/commands"
	AgentsDir   = ".claude/agents"
)

// DirForType returns the .claude subdirectory for a given item type.
func DirForType(itemType string) string {
	switch itemType {
	case "skill":
		return SkillsDir
	case "command":
		return CommandsDir
	case "agent":
		return AgentsDir
	default:
		return ".claude/" + itemType + "s"
	}
}

// Install writes the downloaded files to the appropriate directory in the project.
// Returns the content hash of the installed files.
func Install(projectDir, itemType, name string, files []registry.File) (string, error) {
	targetDir := filepath.Join(projectDir, DirForType(itemType), name)

	// Clean target directory before installing
	if err := os.RemoveAll(targetDir); err != nil {
		return "", fmt.Errorf("cleaning target directory: %w", err)
	}

	for _, f := range files {
		fullPath := filepath.Join(targetDir, f.Path)

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return "", fmt.Errorf("creating directory for %s: %w", f.Path, err)
		}

		if err := os.WriteFile(fullPath, f.Content, 0644); err != nil {
			return "", fmt.Errorf("writing %s: %w", f.Path, err)
		}
	}

	hash, err := ComputeHash(targetDir)
	if err != nil {
		return "", fmt.Errorf("computing hash: %w", err)
	}

	return hash, nil
}

// ComputeHash computes a SHA256 hash over all files in a directory.
// Files are sorted by path for deterministic output.
// Returns the hash truncated to 12 hex characters.
func ComputeHash(dir string) (string, error) {
	var paths []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			paths = append(paths, rel)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walking directory: %w", err)
	}

	sort.Strings(paths)

	h := sha256.New()
	for _, p := range paths {
		content, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", p, err)
		}
		// Use forward slashes for consistency across platforms
		normalizedPath := strings.ReplaceAll(p, string(os.PathSeparator), "/")
		h.Write([]byte(normalizedPath))
		h.Write([]byte("\n"))
		h.Write(content)
	}

	return fmt.Sprintf("%x", h.Sum(nil))[:12], nil
}

// Uninstall removes the installed files for a skill or command.
func Uninstall(projectDir, itemType, name string) error {
	targetDir := filepath.Join(projectDir, DirForType(itemType), name)
	return os.RemoveAll(targetDir)
}

// IsInstalled checks if an item is installed locally.
func IsInstalled(projectDir, itemType, name string) bool {
	targetDir := filepath.Join(projectDir, DirForType(itemType), name)
	info, err := os.Stat(targetDir)
	return err == nil && info.IsDir()
}
