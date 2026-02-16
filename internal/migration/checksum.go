package migration

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// CalculateChecksum computes SHA-256 hash of a migration file
func CalculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// ChecksumMatch compares file checksum with stored checksum
func ChecksumMatch(filePath, storedChecksum string) (bool, error) {
	currentChecksum, err := CalculateChecksum(filePath)
	if err != nil {
		return false, err
	}
	return currentChecksum == storedChecksum, nil
}
