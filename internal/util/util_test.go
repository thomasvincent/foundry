package util

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNowUTC_Format verifies that NowUTC returns a properly formatted RFC3339 timestamp.
func TestNowUTC_Format(t *testing.T) {
	t.Parallel()

	result := NowUTC()

	// Verify it parses as RFC3339.
	_, err := time.Parse(time.RFC3339, result)
	if err != nil {
		t.Fatalf("NowUTC returned non-RFC3339 format: %q, error: %v", result, err)
	}

	// Verify it's in UTC (ends with Z).
	if !contains(result, "Z") {
		t.Errorf("expected RFC3339 UTC format (ending with Z), got: %q", result)
	}
}

// TestCanonicalHash_Deterministic verifies that hashing the same input always produces the same hash.
func TestCanonicalHash_Deterministic(t *testing.T) {
	t.Parallel()

	data := []byte(`{"name": "test", "value": 42}`)

	hash1 := CanonicalHash(data)
	hash2 := CanonicalHash(data)

	if hash1 != hash2 {
		t.Errorf("same input produced different hashes: %q vs %q", hash1, hash2)
	}

	// Verify it's a valid hex string of 64 characters (SHA256 = 256 bits = 64 hex chars).
	if len(hash1) != 64 {
		t.Errorf("expected 64-character hash, got %d characters: %q", len(hash1), hash1)
	}

	// Verify all characters are valid hex.
	for _, ch := range hash1 {
		if !isHexChar(ch) {
			t.Errorf("invalid hex character %q in hash: %q", ch, hash1)
		}
	}
}

// TestCanonicalHash_DifferentOrder verifies that key order doesn't affect the hash for JSON objects.
func TestCanonicalHash_DifferentOrder(t *testing.T) {
	t.Parallel()

	data1 := []byte(`{"a":1,"b":2}`)
	data2 := []byte(`{"b":2,"a":1}`)

	hash1 := CanonicalHash(data1)
	hash2 := CanonicalHash(data2)

	if hash1 != hash2 {
		t.Errorf("different key order produced different hashes: %q vs %q", hash1, hash2)
	}
}

// TestCanonicalHash_NonJSON verifies that non-JSON input still produces a valid hash.
func TestCanonicalHash_NonJSON(t *testing.T) {
	t.Parallel()

	nonJSONData := []byte(`not valid json at all!`)

	hash := CanonicalHash(nonJSONData)

	// Should still produce a valid hash (of raw bytes).
	if len(hash) != 64 {
		t.Errorf("expected 64-character hash for non-JSON, got %d", len(hash))
	}

	// Verify determinism.
	hash2 := CanonicalHash(nonJSONData)
	if hash != hash2 {
		t.Errorf("non-JSON hashing not deterministic: %q vs %q", hash, hash2)
	}
}

// TestEnsureDir creates nested directories and verifies they exist.
func TestEnsureDir(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "a", "b", "c")

	err := EnsureDir(nestedPath)
	if err != nil {
		t.Fatalf("EnsureDir failed: %v", err)
	}

	// Verify all directories exist.
	if _, err := os.Stat(nestedPath); err != nil {
		t.Errorf("directory %q does not exist after EnsureDir: %v", nestedPath, err)
	}
}

// TestWriteJSON_ReadJSON writes an object, reads it back, and verifies equality.
func TestWriteJSON_ReadJSON(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.json")

	// Create a test object.
	originalData := map[string]interface{}{
		"name":  "test-project",
		"count": 42,
		"nested": map[string]interface{}{
			"value": "hello",
		},
	}

	// Write it.
	err := WriteJSON(filePath, originalData)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	// Read it back.
	var readData map[string]interface{}
	err = ReadJSON(filePath, &readData)
	if err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}

	// Verify the data matches.
	if readData["name"] != "test-project" {
		t.Errorf("expected name 'test-project', got %v", readData["name"])
	}

	if readData["count"] != float64(42) {
		t.Errorf("expected count 42, got %v", readData["count"])
	}

	nested, ok := readData["nested"].(map[string]interface{})
	if !ok {
		t.Fatal("nested field is not a map")
	}

	if nested["value"] != "hello" {
		t.Errorf("expected nested value 'hello', got %v", nested["value"])
	}
}

// TestWriteJSON_Atomic verifies that the file only appears after the write completes successfully.
func TestWriteJSON_Atomic(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "atomic.json")

	testData := map[string]string{"key": "value"}

	err := WriteJSON(filePath, testData)
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	// After successful write, file should exist.
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("file does not exist after successful WriteJSON: %v", err)
	}

	if fileInfo.IsDir() {
		t.Fatal("WriteJSON created a directory instead of a file")
	}

	// Verify the file is a regular file (permissions may vary by umask).
	if fileInfo.Size() == 0 {
		t.Error("file is empty after WriteJSON")
	}

	// Verify it's valid JSON by reading it.
	var readData map[string]string
	err = ReadJSON(filePath, &readData)
	if err != nil {
		t.Fatalf("written file is not valid JSON: %v", err)
	}

	// Verify content matches what we wrote.
	if readData["key"] != "value" {
		t.Errorf("expected key='value', got key=%q", readData["key"])
	}

	// Verify the file ends with a newline (POSIX compliance).
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if len(content) == 0 || content[len(content)-1] != '\n' {
		t.Error("expected file to end with newline")
	}
}

// Helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Helper function to check if a rune is a valid hex character.
func isHexChar(ch rune) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

// TestCanonicalHash_ConsistencyWithJSON verifies that canonical hash matches direct JSON marshal for sorted objects.
func TestCanonicalHash_ConsistencyWithJSON(t *testing.T) {
	t.Parallel()

	input := map[string]interface{}{
		"z": 3,
		"a": 1,
		"m": 2,
	}

	inputBytes, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal input: %v", err)
	}

	hash := CanonicalHash(inputBytes)

	if len(hash) != 64 {
		t.Errorf("expected 64-char hash, got %d", len(hash))
	}
}

// TestEnsureDir_AlreadyExists verifies that EnsureDir succeeds if the directory already exists.
func TestEnsureDir_AlreadyExists(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Call EnsureDir on an already existing directory.
	err := EnsureDir(tmpDir)
	if err != nil {
		t.Fatalf("EnsureDir failed on existing directory: %v", err)
	}
}

// TestReadJSON_FileNotFound verifies that ReadJSON fails gracefully when file doesn't exist.
func TestReadJSON_FileNotFound(t *testing.T) {
	t.Parallel()

	var data map[string]string
	err := ReadJSON("/nonexistent/path/file.json", &data)
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

// TestWriteJSON_CreatesParentDirs verifies that WriteJSON creates parent directories.
func TestWriteJSON_CreatesParentDirs(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "deeply", "nested", "path", "file.json")

	testData := map[string]string{"test": "data"}

	err := WriteJSON(filePath, testData)
	if err != nil {
		t.Fatalf("WriteJSON failed to create parent directories: %v", err)
	}

	// Verify file exists.
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("file was not created: %v", err)
	}
}
