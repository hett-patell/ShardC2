package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"
)

const (
	NonceSize      = 12
	ReplayWindowS  = 300 // 5 minutes
)

func Encrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func Decrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ct, nil)
}

func Sign(method, path string, body, key []byte, ts int64) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(method))
	mac.Write([]byte(path))
	mac.Write(body)
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

func Verify(method, path string, body, key []byte, ts int64, signature string) bool {
	elapsed := time.Now().Unix() - ts
	if elapsed < 0 {
		elapsed = -elapsed
	}
	if elapsed > ReplayWindowS {
		return false
	}
	expected := Sign(method, path, body, key, ts)
	return hmac.Equal([]byte(expected), []byte(signature))
}

func DeriveKey(raw string) []byte {
	h := sha256.Sum256([]byte(raw))
	return h[:]
}

func ParseHexKey(s string) ([]byte, error) {
	key, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid hex key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(key))
	}
	return key, nil
}

func GenerateHexKey() (string, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}

func TimestampNow() int64 {
	return time.Now().Unix()
}

func IsTimestampValid(ts int64) bool {
	elapsed := time.Now().Unix() - ts
	if elapsed < 0 {
		elapsed = -elapsed
	}
	return elapsed <= ReplayWindowS
}

// EncryptedSize returns the output size for a given plaintext length.
func EncryptedSize(plaintextLen int) int {
	if plaintextLen > math.MaxInt-NonceSize-16 {
		return 0
	}
	return NonceSize + plaintextLen + 16 // nonce + ciphertext + GCM tag
}
