package migrate

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
)

// helloSum is the SHA-256 of "hello", the form vm stores in the checksum column.
const helloSum = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

func TestChecksum(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "100_hello.sql", "hello")

	got, err := Checksum(path)
	if err != nil {
		t.Fatalf("Checksum: %v", err)
	}
	if got != helloSum {
		t.Errorf("Checksum() = %q, want %q", got, helloSum)
	}
	if bytes := ChecksumBytes([]byte("hello")); bytes != got {
		t.Errorf("ChecksumBytes() = %q, want %q", bytes, got)
	}
}

func TestChecksumEmptyFile(t *testing.T) {
	dir := t.TempDir()
	got, err := Checksum(writeFile(t, dir, "100_empty.sql", ""))
	if err != nil {
		t.Fatalf("Checksum: %v", err)
	}
	const emptySum = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got != emptySum {
		t.Errorf("Checksum() = %q, want %q", got, emptySum)
	}
}

func TestChecksumMissingFile(t *testing.T) {
	_, err := Checksum(filepath.Join(t.TempDir(), "gone.sql"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Checksum on missing file: err = %v, want fs.ErrNotExist", err)
	}
}
