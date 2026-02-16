package migration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCalculateChecksum(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.sql")

	content := "CREATE TABLE users (id SERIAL PRIMARY KEY);"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Test checksum calculation
	checksum, err := CalculateChecksum(testFile)
	if err != nil {
		t.Fatalf("CalculateChecksum() error = %v", err)
	}

	// SHA-256 produces 64 character hex string
	if len(checksum) != 64 {
		t.Errorf("Checksum length = %d, want 64", len(checksum))
	}

	// Verify deterministic
	checksum2, err := CalculateChecksum(testFile)
	if err != nil {
		t.Fatalf("CalculateChecksum() second call error = %v", err)
	}

	if checksum != checksum2 {
		t.Errorf("Checksums not deterministic: %s != %s", checksum, checksum2)
	}
}

func TestCalculateChecksum_DifferentContent(t *testing.T) {
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "test1.sql")
	file2 := filepath.Join(tmpDir, "test2.sql")

	os.WriteFile(file1, []byte("CREATE TABLE users (id SERIAL);"), 0644)
	os.WriteFile(file2, []byte("CREATE TABLE users (id INTEGER);"), 0644)

	checksum1, _ := CalculateChecksum(file1)
	checksum2, _ := CalculateChecksum(file2)

	if checksum1 == checksum2 {
		t.Error("Different content produced same checksum")
	}
}

func TestCalculateChecksum_NonexistentFile(t *testing.T) {
	_, err := CalculateChecksum("/nonexistent/file.sql")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestChecksumMatch(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.sql")

	content := "CREATE TABLE products (id SERIAL PRIMARY KEY);"
	os.WriteFile(testFile, []byte(content), 0644)

	// Get actual checksum
	actualChecksum, _ := CalculateChecksum(testFile)

	// Test match
	match, err := ChecksumMatch(testFile, actualChecksum)
	if err != nil {
		t.Fatalf("ChecksumMatch() error = %v", err)
	}

	if !match {
		t.Error("ChecksumMatch() = false, want true for matching checksum")
	}

	// Test mismatch
	match, err = ChecksumMatch(testFile, "incorrect_checksum")
	if err != nil {
		t.Fatalf("ChecksumMatch() error = %v", err)
	}

	if match {
		t.Error("ChecksumMatch() = true, want false for mismatched checksum")
	}
}

func TestChecksumMatch_NonexistentFile(t *testing.T) {
	_, err := ChecksumMatch("/nonexistent/file.sql", "somechecksum")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func BenchmarkCalculateChecksum(b *testing.B) {
	tmpDir := b.TempDir()
	testFile := filepath.Join(tmpDir, "bench.sql")

	content := `
	CREATE TABLE products (
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL,
		price DECIMAL(10,2)
	);
	CREATE INDEX idx_products_name ON products(name);
	`
	os.WriteFile(testFile, []byte(content), 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateChecksum(testFile)
	}
}
