package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRejectsMissingVersion(t *testing.T) {
	m := Manifest{Name: "test-plugin"}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for missing version")
	}
}

func TestValidateRejectsUnknownPermission(t *testing.T) {
	m := Manifest{Name: "test", Version: "1.0", Permissions: []string{"admin.nuke"}}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for unknown permission")
	}
}

func TestValidateRejectsDuplicateCommandNames(t *testing.T) {
	m := Manifest{
		Name:    "test",
		Version: "1.0",
		Commands: []CommandSpec{
			{Name: "scan"},
			{Name: "scan"},
		},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for duplicate command names")
	}
}

func TestValidateAcceptsValidManifest(t *testing.T) {
	m := Manifest{
		Name:        "recon-plugin",
		Version:     "1.0.0",
		Description: "Network recon",
		Permissions: []string{"bots.read", "commands.write"},
		Commands:    []CommandSpec{{Name: "scan", Description: "Run scan"}},
		SHA256:      "abc123",
		Signature:   "sig123",
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadManifestFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	os.WriteFile(path, []byte(`{
		"name": "test-plugin",
		"version": "1.0.0",
		"description": "A test plugin",
		"commands": [{"name": "hello", "description": "Say hello"}],
		"permissions": ["bots.read"],
		"sha256": "abc",
		"signature": "sig"
	}`), 0600)

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if m.Name != "test-plugin" || m.Version != "1.0.0" {
		t.Fatalf("unexpected manifest: %+v", m)
	}
}

func TestLoadAllManifestsFromDirectory(t *testing.T) {
	dir := t.TempDir()

	pluginDir := filepath.Join(dir, "my-plugin")
	os.MkdirAll(pluginDir, 0755)
	os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte(`{
		"name": "my-plugin",
		"version": "2.0",
		"commands": [],
		"permissions": []
	}`), 0600)

	manifests, err := LoadAllManifests(dir)
	if err != nil {
		t.Fatalf("load all: %v", err)
	}
	if len(manifests) != 1 || manifests[0].Name != "my-plugin" {
		t.Fatalf("unexpected manifests: %+v", manifests)
	}
}

func TestLoadAllManifestsReturnsNilForMissingDir(t *testing.T) {
	manifests, err := LoadAllManifests("/nonexistent/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if manifests != nil {
		t.Fatalf("expected nil, got %+v", manifests)
	}
}
