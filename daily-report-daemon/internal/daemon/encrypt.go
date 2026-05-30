package daemon

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// EncryptAPIKey encrypts an API key using AES-256-GCM with a machine-derived key.
// The derived key is stored in daemon data directory.
func EncryptAPIKey(plaintext, dataDir string) (string, error) {
	key, err := getOrCreateMachineKey(dataDir)
	if err != nil {
		return "", fmt.Errorf("machine key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptAPIKey decrypts an API key encrypted with EncryptAPIKey.
func DecryptAPIKey(encoded, dataDir string) (string, error) {
	key, err := getOrCreateMachineKey(dataDir)
	if err != nil {
		return "", fmt.Errorf("machine key: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

// getOrCreateMachineKey derives or loads a local encryption key.
// Uses SHA-256 of hostname + random seed stored in dataDir.
func getOrCreateMachineKey(dataDir string) ([]byte, error) {
	keyPath := filepath.Join(dataDir, ".machine-key")
	if data, err := os.ReadFile(keyPath); err == nil {
		decoded, err := base64.StdEncoding.DecodeString(string(data))
		if err == nil && len(decoded) == 32 {
			return decoded, nil
		}
	}

	// Generate new key
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}

	// Mix with hostname for machine binding
	hostname, _ := os.Hostname()
	hash := sha256.Sum256(append([]byte(hostname), key...))

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(hash[:])
	if err := os.WriteFile(keyPath, []byte(encoded), 0600); err != nil {
		return nil, err
	}

	return hash[:], nil
}

// NOTE: If the machine key file (.machine-key) is deleted or the hostname changes,
// all previously encrypted API keys become unrecoverable. Back up the key file or
// re-encrypt keys after hostname changes. See docs for key rotation procedure.
