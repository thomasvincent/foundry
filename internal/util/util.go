// Package util provides shared utilities for the Foundry CI/CD engine.
package util

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// NowUTC returns the current time in UTC as an RFC3339-formatted string.
func NowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// CanonicalHash computes the SHA-256 hash of canonicalized JSON data.
// Canonicalization ensures deterministic hashing by unmarshaling and
// re-marshaling with sorted object keys (Go's json.Marshal sorts map keys).
// Returns the hash as a lowercase hex string.
func CanonicalHash(data []byte) string {
	var obj interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		// If data is not valid JSON, hash the raw bytes.
		slog.Debug("canonical hash: input is not JSON, hashing raw bytes")
		return hashBytes(data)
	}

	canonical := canonicalizeJSON(obj)
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return hashBytes(data)
	}

	return hashBytes(encoded)
}

// canonicalizeJSON recursively processes JSON values to ensure deterministic
// marshaling. Go's json.Marshal already sorts map keys, so this primarily
// ensures nested structures are consistently handled.
func canonicalizeJSON(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		result := make(map[string]interface{}, len(keys))
		for _, k := range keys {
			result[k] = canonicalizeJSON(val[k])
		}
		return result

	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = canonicalizeJSON(item)
		}
		return result

	default:
		return val
	}
}

// hashBytes computes SHA-256 hash of the input bytes.
func hashBytes(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// EnsureDir creates a directory and all necessary parent directories if they
// do not already exist.
func EnsureDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("ensure directory %q: %w", path, err)
	}
	return nil
}

// WriteJSON marshals v to indented JSON and writes it atomically to path.
// Atomic writing is achieved by writing to a temporary file in the same
// directory, then renaming. The file is created with mode 0o644.
func WriteJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	// Append trailing newline for POSIX compliance.
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := EnsureDir(dir); err != nil {
		return fmt.Errorf("write JSON ensure dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".tmp.*")
	if err != nil {
		return fmt.Errorf("write JSON create temp: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write JSON write temp: %w", err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("write JSON close temp: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("write JSON rename: %w", err)
	}

	return nil
}

// ReadJSON reads JSON from path and unmarshals it into v.
func ReadJSON(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read JSON: %w", err)
	}

	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("read JSON unmarshal %q: %w", path, err)
	}

	return nil
}
