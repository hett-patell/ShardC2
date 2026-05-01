package main

import (
	"bytes"
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
