package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetLastUsed_NoFile(t *testing.T) {
	tmpDir := t.TempDir()

	port := GetLastUsed(tmpDir)
	if port != 0 {
		t.Errorf("expected 0 for non-existent file, got %d", port)
	}
}

func TestGetLastUsed_ValidPort(t *testing.T) {
	tmpDir := t.TempDir()

	// Write valid port
	path := filepath.Join(tmpDir, lastUsedFileName)
	if err := os.WriteFile(path, []byte("3005"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	port := GetLastUsed(tmpDir)
	if port != 3005 {
		t.Errorf("expected 3005, got %d", port)
	}
}

func TestGetLastUsed_WithWhitespace(t *testing.T) {
	tmpDir := t.TempDir()

	path := filepath.Join(tmpDir, lastUsedFileName)
	if err := os.WriteFile(path, []byte("  3010\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	port := GetLastUsed(tmpDir)
	if port != 3010 {
		t.Errorf("expected 3010, got %d", port)
	}
}

func TestGetLastUsed_InvalidContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"not a number", "abc"},
		{"negative", "-1"},
		{"zero", "0"},
		{"too large", "70000"},
		{"empty", ""},
		{"float", "3000.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, lastUsedFileName)
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			port := GetLastUsed(tmpDir)
			if port != 0 {
				t.Errorf("expected 0 for invalid content %q, got %d", tt.content, port)
			}
		})
	}
}

func TestSetLastUsed(t *testing.T) {
	tmpDir := t.TempDir()

	if err := SetLastUsed(tmpDir, 3015); err != nil {
		t.Fatalf("SetLastUsed() error = %v", err)
	}

	// Verify file was created
	path := filepath.Join(tmpDir, lastUsedFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if string(data) != "3015" {
		t.Errorf("expected file content '3015', got %q", string(data))
	}

	// Verify no temp file left
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should not exist after successful write")
	}
}

func TestSetLastUsed_CreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "nested", "dir")

	if err := SetLastUsed(nestedDir, 3020); err != nil {
		t.Fatalf("SetLastUsed() error = %v", err)
	}

	port := GetLastUsed(nestedDir)
	if port != 3020 {
		t.Errorf("expected 3020, got %d", port)
	}
}

func TestSetAndGetLastUsed_Roundtrip(t *testing.T) {
	tmpDir := t.TempDir()

	testPorts := []int{3000, 3500, 4000, 65535, 1}

	for _, expected := range testPorts {
		if err := SetLastUsed(tmpDir, expected); err != nil {
			t.Fatalf("SetLastUsed(%d) error = %v", expected, err)
		}

		got := GetLastUsed(tmpDir)
		if got != expected {
			t.Errorf("expected %d, got %d", expected, got)
		}
	}
}
