package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func XOREncrypt(plaintext, key []byte) []byte {
	out := make([]byte, len(plaintext))
	for i := range plaintext {
		out[i] = plaintext[i] ^ key[i%len(key)]
	}
	return out
}

func XORDecrypt(ciphertext, key []byte) []byte {
	return XOREncrypt(ciphertext, key)
}

func XOREncryptString(s string, key []byte) string {
	return base64.StdEncoding.EncodeToString(XOREncrypt([]byte(s), key))
}

func XORDecryptString(encoded string, key []byte) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	return string(XORDecrypt(data, key)), nil
}

func GenerateXORKey(size int) ([]byte, error) {
	key := make([]byte, size)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

func ObfuscateForBuild(value string) (encoded string, keyHex string) {
	key, _ := GenerateXORKey(16)
	encoded = XOREncryptString(value, key)
	keyHex = fmt.Sprintf("%x", key)
	return
}
