// Package history manages issued ports history with freeze period support.
package history

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const historyFileName = "issued-ports.yaml"

// PortRecord represents a single issued port record.
type PortRecord struct {
	Port     int       `yaml:"port"`
	IssuedAt time.Time `yaml:"issuedAt"`
}

// History represents the issued ports history.
type History struct {
	Ports []PortRecord `yaml:"ports"`
}

// Load reads the history from disk.
// Returns empty history if file doesn't exist.
func Load(configDir string) (*History, error) {
	path := filepath.Join(configDir, historyFileName)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &History{}, nil
		}
		return nil, fmt.Errorf("failed to read history file: %w", err)
	}

	var h History
	if err := yaml.Unmarshal(data, &h); err != nil {
		// If history is corrupted, start fresh
		return &History{}, nil
	}

	return &h, nil
}

// Save writes the history to disk atomically.
func (h *History) Save(configDir string) error {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	path := filepath.Join(configDir, historyFileName)
	tmpPath := path + ".tmp"

	data, err := yaml.Marshal(h)
	if err != nil {
		return fmt.Errorf("failed to marshal history: %w", err)
	}

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// AddPort adds a new port record with current timestamp.
func (h *History) AddPort(port int) {
	h.Ports = append(h.Ports, PortRecord{
		Port:     port,
		IssuedAt: time.Now(),
	})
}

// GetFrozenPorts returns a set of ports that are still within freeze period.
func (h *History) GetFrozenPorts(freezePeriodMinutes int) map[int]bool {
	frozen := make(map[int]bool)

	if freezePeriodMinutes <= 0 {
		return frozen
	}

	cutoff := time.Now().Add(-time.Duration(freezePeriodMinutes) * time.Minute)

	for _, record := range h.Ports {
		if record.IssuedAt.After(cutoff) {
			frozen[record.Port] = true
		}
	}

	return frozen
}

// Cleanup removes records older than freeze period.
func (h *History) Cleanup(freezePeriodMinutes int) {
	if freezePeriodMinutes <= 0 {
		h.Ports = nil
		return
	}

	cutoff := time.Now().Add(-time.Duration(freezePeriodMinutes) * time.Minute)
	var active []PortRecord

	for _, record := range h.Ports {
		if record.IssuedAt.After(cutoff) {
			active = append(active, record)
		}
	}

	h.Ports = active
}
