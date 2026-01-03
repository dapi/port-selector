package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShortenHomePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "exact home directory",
			input:    home,
			expected: "~",
		},
		{
			name:     "path under home",
			input:    filepath.Join(home, "projects", "myapp"),
			expected: "~/projects/myapp",
		},
		{
			name:     "path not under home",
			input:    "/var/log/app.log",
			expected: "/var/log/app.log",
		},
		{
			name:     "root path",
			input:    "/",
			expected: "/",
		},
		{
			name:     "empty path",
			input:    "",
			expected: "",
		},
		{
			name:     "similar prefix but not home",
			input:    home + "extra/path",
			expected: home + "extra/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShortenHomePath(tt.input)
			if result != tt.expected {
				t.Errorf("ShortenHomePath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
