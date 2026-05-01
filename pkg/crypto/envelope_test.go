package crypto

import (
	"bytes"
	"testing"
	"time"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key := DeriveKey("test-payload-key")
	plaintext := []byte(`{"command":"id","type":"shell"}`)

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Equal(ct, plaintext) {
		t.Fatal("ciphertext equals plaintext")
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(pt, plaintext) {
		t.Fatalf("roundtrip mismatch: got %q, want %q", pt, plaintext)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := DeriveKey("key-one")
	key2 := DeriveKey("key-two")

	ct, _ := Encrypt([]byte("secret"), key1)
	_, err := Decrypt(ct, key2)
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	key := DeriveKey("tamper-test")
	ct, _ := Encrypt([]byte("hello"), key)

	ct[len(ct)-1] ^= 0xff
	_, err := Decrypt(ct, key)
	if err == nil {
		t.Fatal("expected error on tampered ciphertext")
	}
}

func TestDecryptTooShort(t *testing.T) {
	key := DeriveKey("short-test")
	_, err := Decrypt([]byte("tiny"), key)
	if err == nil {
		t.Fatal("expected error on short ciphertext")
	}
}

func TestUniqueNonces(t *testing.T) {
	key := DeriveKey("nonce-test")
	msg := []byte("same message")

	ct1, _ := Encrypt(msg, key)
	ct2, _ := Encrypt(msg, key)
	if bytes.Equal(ct1, ct2) {
		t.Fatal("two encryptions of same plaintext produced identical ciphertext")
	}
}

func TestSignVerify(t *testing.T) {
	key := DeriveKey("hmac-key")
	ts := time.Now().Unix()
	body := []byte(`{"output":"uid=0(root)"}`)

	sig := Sign("POST", "/api/v1/agent/result", body, key, ts)
	if !Verify("POST", "/api/v1/agent/result", body, key, ts, sig) {
		t.Fatal("valid signature rejected")
	}
}

func TestVerifyWrongMethod(t *testing.T) {
	key := DeriveKey("hmac-key")
	ts := time.Now().Unix()
	body := []byte(`{"data":"test"}`)

	sig := Sign("POST", "/api/v1/agent/result", body, key, ts)
	if Verify("GET", "/api/v1/agent/result", body, key, ts, sig) {
		t.Fatal("signature should fail with wrong method")
	}
}

func TestVerifyTamperedBody(t *testing.T) {
	key := DeriveKey("hmac-key")
	ts := time.Now().Unix()

	sig := Sign("POST", "/path", []byte("original"), key, ts)
	if Verify("POST", "/path", []byte("tampered"), key, ts, sig) {
		t.Fatal("signature should fail with tampered body")
	}
}

func TestVerifyExpiredTimestamp(t *testing.T) {
	key := DeriveKey("hmac-key")
	ts := time.Now().Unix() - 600 // 10 minutes ago
	body := []byte("data")

	sig := Sign("POST", "/path", body, key, ts)
	if Verify("POST", "/path", body, key, ts, sig) {
		t.Fatal("expired timestamp should be rejected")
	}
}

func TestDeriveKeyDeterministic(t *testing.T) {
	k1 := DeriveKey("same-input")
	k2 := DeriveKey("same-input")
	if !bytes.Equal(k1, k2) {
		t.Fatal("DeriveKey not deterministic")
	}
	if len(k1) != 32 {
		t.Fatalf("key length: got %d, want 32", len(k1))
	}
}

func TestParseHexKey(t *testing.T) {
	hex, _ := GenerateHexKey()
	key, err := ParseHexKey(hex)
	if err != nil {
		t.Fatalf("ParseHexKey: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("key length: got %d, want 32", len(key))
	}
}

func TestParseHexKeyInvalid(t *testing.T) {
	if _, err := ParseHexKey("not-hex"); err == nil {
		t.Fatal("expected error for invalid hex")
	}
	if _, err := ParseHexKey("aabb"); err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestEncryptedSize(t *testing.T) {
	key := DeriveKey("size-test")
	msg := []byte("hello world")
	ct, _ := Encrypt(msg, key)
	expected := EncryptedSize(len(msg))
	if len(ct) != expected {
		t.Fatalf("size mismatch: got %d, predicted %d", len(ct), expected)
	}
}
