package secrets

import (
	"strings"
	"testing"
)

func TestVaultEncryptDecrypt(t *testing.T) {
	v, err := NewVaultWithKey(make([]byte, keySize))
	if err != nil {
		t.Fatalf("NewVaultWithKey: %v", err)
	}

	enc, err := v.Encrypt("sk-test-secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !IsEncrypted(enc) {
		t.Fatalf("expected encrypted prefix, got %q", enc)
	}
	if strings.Contains(enc, "sk-test-secret") {
		t.Fatalf("ciphertext leaked plaintext: %q", enc)
	}

	got, err := v.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != "sk-test-secret" {
		t.Fatalf("Decrypt = %q, want sk-test-secret", got)
	}
}

func TestVaultLegacyPlaintextPassthrough(t *testing.T) {
	v, err := NewVaultWithKey(make([]byte, keySize))
	if err != nil {
		t.Fatalf("NewVaultWithKey: %v", err)
	}
	got, err := v.Decrypt("legacy-plain-key")
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != "legacy-plain-key" {
		t.Fatalf("Decrypt = %q, want legacy-plain-key", got)
	}
}

func TestVaultEmptyValues(t *testing.T) {
	v, err := NewVaultWithKey(make([]byte, keySize))
	if err != nil {
		t.Fatalf("NewVaultWithKey: %v", err)
	}
	enc, err := v.Encrypt("")
	if err != nil || enc != "" {
		t.Fatalf("Encrypt empty = %q, err = %v", enc, err)
	}
	got, err := v.Decrypt("")
	if err != nil || got != "" {
		t.Fatalf("Decrypt empty = %q, err = %v", got, err)
	}
}

func TestNewVaultAtCreatesKeyFile(t *testing.T) {
	dir := t.TempDir()
	v, err := NewVaultAt(dir)
	if err != nil {
		t.Fatalf("NewVaultAt: %v", err)
	}
	enc, err := v.Encrypt("host-password")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	v2, err := NewVaultAt(dir)
	if err != nil {
		t.Fatalf("NewVaultAt reload: %v", err)
	}
	got, err := v2.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != "host-password" {
		t.Fatalf("Decrypt = %q, want host-password", got)
	}
}
