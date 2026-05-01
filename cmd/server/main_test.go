package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDeriveServerSecretsGeneratesSeparateJWTSecret(t *testing.T) {
	operatorToken, jwtSecret, generated, err := deriveServerSecrets("", "", nil)
	if err != nil {
		t.Fatalf("derive secrets: %v", err)
	}
	if !generated {
		t.Fatal("expected generated operator token")
	}
	if operatorToken == "" {
		t.Fatal("operator token is empty")
	}
	if len(jwtSecret) == 0 {
		t.Fatal("jwt secret is empty")
	}
	if bytes.Equal([]byte(operatorToken), jwtSecret) {
		t.Fatal("jwt secret must not equal operator token")
	}
}

func TestDeriveServerSecretsRejectsSharedOperatorAndJWTSecret(t *testing.T) {
	_, _, _, err := deriveServerSecrets("shared-secret", "shared-secret", nil)
	if err == nil {
		t.Fatal("expected error when operator token and jwt secret match")
	}
}

func TestLoadServerPolicyDefaultsToSafePolicy(t *testing.T) {
	p, err := loadServerPolicy("")
	if err != nil {
		t.Fatalf("load default policy: %v", err)
	}
	if !p.SafeMode || p.AllowExternalBrute || p.AllowAutoDeploy {
		t.Fatalf("unexpected default policy: %+v", p)
	}
	if err := p.ValidateTarget("127.0.0.1"); err != nil {
		t.Fatalf("default policy rejected loopback: %v", err)
	}
	if err := p.ValidateTarget("8.8.8.8"); err == nil {
		t.Fatal("default policy accepted external target")
	}
}

func TestLoadServerPolicyReadsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(path, []byte(`{"safe_mode":true,"allowed_cidrs":["10.0.0.0/8"],"blocked_cidrs":["10.1.0.0/16"]}`), 0600); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	p, err := loadServerPolicy(path)
	if err != nil {
		t.Fatalf("load policy file: %v", err)
	}
	if err := p.ValidateTarget("10.2.3.4"); err != nil {
		t.Fatalf("loaded policy rejected allowed target: %v", err)
	}
	if err := p.ValidateTarget("10.1.2.3"); err == nil {
		t.Fatal("loaded policy accepted blocked target")
	}
}
