package gui

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
	"runtime"
	"strings"
)

const secretPrefix = "v1:"

// SecretBox encrypts saved database passwords with AES-256-GCM. The key lives
// beside the GUI config and is created with mode 0600. Passwords never leave
// this process in plaintext once saved.
type SecretBox struct {
	keyPath string
	key     []byte
}

// LoadOrCreateSecretBox loads a 32-byte key from keyPath, creating it if needed.
func LoadOrCreateSecretBox(keyPath string) (*SecretBox, error) {
	if keyPath == "" {
		return nil, errors.New("secret key path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return nil, fmt.Errorf("create key directory: %w", err)
	}

	data, err := os.ReadFile(keyPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read key: %w", err)
		}
		key := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, key); err != nil {
			return nil, fmt.Errorf("generate key: %w", err)
		}
		encoded := base64.RawURLEncoding.EncodeToString(key) + "\n"
		if err := os.WriteFile(keyPath, []byte(encoded), 0o600); err != nil {
			return nil, fmt.Errorf("write key: %w", err)
		}
		_ = tightenFileMode(keyPath)
		return &SecretBox{keyPath: keyPath, key: key}, nil
	}

	if err := tightenFileMode(keyPath); err != nil {
		return nil, err
	}

	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("invalid key length: got %d bytes, want 32", len(key))
	}
	return &SecretBox{keyPath: keyPath, key: key}, nil
}

// KeyPath returns the path of the local encryption key.
func (b *SecretBox) KeyPath() string { return b.keyPath }

// Encrypt encrypts plaintext bound to connectionID as associated data.
func (b *SecretBox) Encrypt(connectionID, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	block, err := aes.NewCipher(b.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(plaintext), []byte(connectionID))
	payload := append(nonce, sealed...)
	return secretPrefix + base64.RawStdEncoding.EncodeToString(payload), nil
}

// Decrypt decrypts a value produced by Encrypt.
func (b *SecretBox) Decrypt(connectionID, token string) (string, error) {
	if token == "" {
		return "", nil
	}
	if !strings.HasPrefix(token, secretPrefix) {
		return "", errors.New("unsupported secret format")
	}
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(token, secretPrefix))
	if err != nil {
		return "", fmt.Errorf("decode secret: %w", err)
	}
	block, err := aes.NewCipher(b.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(payload) < gcm.NonceSize() {
		return "", errors.New("secret is truncated")
	}
	nonce, ciphertext := payload[:gcm.NonceSize()], payload[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte(connectionID))
	if err != nil {
		return "", errors.New("could not decrypt saved password (key changed?)")
	}
	return string(plain), nil
}

func tightenFileMode(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("secure permissions on %s: %w", path, err)
		}
	}
	return nil
}
