package builds

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shardc2/shardc2/pkg/models"
)

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "cmd", "agent")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("repo root not found")
		}
		dir = parent
	}
}

func TestValidateRequestRejectsInvalidGOOS(t *testing.T) {
	req := Request{GOOS: "freebsd", GOARCH: "amd64", Profile: "default"}
	if err := req.Validate(); err == nil {
		t.Fatal("expected error for invalid GOOS")
	}
}

func TestValidateRequestRejectsInvalidGOARCH(t *testing.T) {
	req := Request{GOOS: "linux", GOARCH: "mips", Profile: "default"}
	if err := req.Validate(); err == nil {
		t.Fatal("expected error for invalid GOARCH")
	}
}

func TestValidateRequestAcceptsValidCombination(t *testing.T) {
	req := Request{GOOS: "linux", GOARCH: "amd64", Profile: "default"}
	if err := req.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRequestRejectsEmptyProfile(t *testing.T) {
	req := Request{GOOS: "linux", GOARCH: "amd64", Profile: ""}
	if err := req.Validate(); err == nil {
		t.Fatal("expected error for empty profile")
	}
}

func TestLocalBuilderBuildProducesArtifact(t *testing.T) {
	repoRoot := findRepoRoot(t)
	builder := NewLocalBuilder(repoRoot)
	req := Request{GOOS: "linux", GOARCH: "amd64", Profile: "default"}
	artifact, err := builder.Build(context.Background(), req)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if artifact.Path == "" {
		t.Fatal("artifact path is empty")
	}
	if artifact.Status != models.BuildStatusCompleted {
		t.Fatalf("artifact status: got %q, want %q", artifact.Status, models.BuildStatusCompleted)
	}
}
