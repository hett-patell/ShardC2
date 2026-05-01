package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var AllowedPermissions = map[string]bool{
	"commands.read":    true,
	"commands.write":   true,
	"bots.read":        true,
	"campaigns.read":   true,
	"campaigns.write":  true,
	"credentials.read": true,
	"exfil.read":       true,
	"audit.read":       true,
}

type CommandSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Manifest struct {
	Name        string        `json:"name"`
	Version     string        `json:"version"`
	Description string        `json:"description"`
	Commands    []CommandSpec `json:"commands"`
	Permissions []string      `json:"permissions"`
	SHA256      string        `json:"sha256"`
	Signature   string        `json:"signature"`
}

func (m Manifest) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("plugin name is required")
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("plugin version is required")
	}
	for _, perm := range m.Permissions {
		if !AllowedPermissions[perm] {
			return fmt.Errorf("unknown permission %q", perm)
		}
	}
	seen := map[string]bool{}
	for _, cmd := range m.Commands {
		if strings.TrimSpace(cmd.Name) == "" {
			return fmt.Errorf("command name is required")
		}
		if seen[cmd.Name] {
			return fmt.Errorf("duplicate command name %q", cmd.Name)
		}
		seen[cmd.Name] = true
	}
	return nil
}

func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("invalid manifest: %w", err)
	}
	return m, nil
}

func LoadAllManifests(pluginsDir string) ([]Manifest, error) {
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read plugins dir: %w", err)
	}

	var manifests []Manifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(pluginsDir, entry.Name(), "manifest.json")
		m, err := LoadManifest(manifestPath)
		if err != nil {
			continue
		}
		manifests = append(manifests, m)
	}
	return manifests, nil
}
