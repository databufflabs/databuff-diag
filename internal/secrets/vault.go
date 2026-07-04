package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	keyFileName    = "secrets.key"
	keySize        = 32
	encryptedPrefix = "enc:v1:"
)

// Vault encrypts and decrypts secrets at rest using AES-256-GCM.
type Vault struct {
	dir string
	key []byte
}

// NewVaultAt loads or creates a master key under dir/secrets.key.
func NewVaultAt(dir string) (*Vault, error) {
	if dir == "" {
		return nil, errors.New("secrets dir is required")
	}
	keyPath := filepath.Join(dir, keyFileName)
	key, err := loadOrCreateKey(keyPath)
	if err != nil {
		return nil, err
	}
	return &Vault{dir: dir, key: key}, nil
}

// NewVaultWithKey creates a vault for tests with a fixed key.
func NewVaultWithKey(key []byte) (*Vault, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("secrets key must be %d bytes", keySize)
	}
	copied := make([]byte, keySize)
	copy(copied, key)
	return &Vault{key: copied}, nil
}

func loadOrCreateKey(path string) ([]byte, error) {
	if data, err := os.ReadFile(path); err == nil {
		if len(data) != keySize {
			return nil, fmt.Errorf("invalid secrets key length %d", len(data))
		}
		return data, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read secrets key: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create secrets dir: %w", err)
	}
	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate secrets key: %w", err)
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, fmt.Errorf("write secrets key: %w", err)
	}
	return key, nil
}

// IsEncrypted reports whether s is an encrypted secret blob.
func IsEncrypted(s string) bool {
	return strings.HasPrefix(s, encryptedPrefix)
}

// Encrypt returns an enc:v1:... string. Empty plaintext is returned unchanged.
func (v *Vault) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if IsEncrypted(plaintext) {
		return plaintext, nil
	}
	if v == nil || len(v.key) != keySize {
		return "", errors.New("secrets vault is not configured")
	}

	block, err := aes.NewCipher(v.key)
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	payload := append(nonce, ciphertext...)
	return encryptedPrefix + base64.StdEncoding.EncodeToString(payload), nil
}

// Decrypt returns plaintext for enc:v1 values. Legacy plaintext values pass through.
func (v *Vault) Decrypt(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !IsEncrypted(value) {
		return value, nil
	}
	if v == nil || len(v.key) != keySize {
		return "", errors.New("secrets vault is not configured")
	}

	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, encryptedPrefix))
	if err != nil {
		return "", fmt.Errorf("decode secret: %w", err)
	}

	block, err := aes.NewCipher(v.key)
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("encrypted secret is too short")
	}

	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}
	return string(plaintext), nil
}
