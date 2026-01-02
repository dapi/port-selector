package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_NoFile(t *testing.T) {
	tmpDir := t.TempDir()

	h, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(h.Ports) != 0 {
		t.Errorf("expected empty history, got %d records", len(h.Ports))
	}
}

func TestLoad_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, historyFileName)

	content := `ports:
  - port: 3000
    issuedAt: "2024-01-15T10:00:00Z"
  - port: 3001
    issuedAt: "2024-01-15T11:00:00Z"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	h, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(h.Ports) != 2 {
		t.Errorf("expected 2 records, got %d", len(h.Ports))
	}

	if h.Ports[0].Port != 3000 {
		t.Errorf("expected port 3000, got %d", h.Ports[0].Port)
	}
}

func TestLoad_CorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, historyFileName)

	if err := os.WriteFile(path, []byte("invalid yaml: ["), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	h, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Should return empty history on corruption
	if len(h.Ports) != 0 {
		t.Errorf("expected empty history on corruption, got %d records", len(h.Ports))
	}
}

func TestHistory_Save(t *testing.T) {
	tmpDir := t.TempDir()

	h := &History{
		Ports: []PortRecord{
			{Port: 3000, IssuedAt: time.Now()},
			{Port: 3001, IssuedAt: time.Now()},
		},
	}

	if err := h.Save(tmpDir); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	path := filepath.Join(tmpDir, historyFileName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("history file was not created")
	}

	// Verify no temp file left
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should not exist")
	}

	// Verify can reload
	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load() after Save() error = %v", err)
	}

	if len(loaded.Ports) != 2 {
		t.Errorf("expected 2 records after reload, got %d", len(loaded.Ports))
	}
}

func TestHistory_AddPort(t *testing.T) {
	h := &History{}

	h.AddPort(3000)
	h.AddPort(3001)

	if len(h.Ports) != 2 {
		t.Errorf("expected 2 records, got %d", len(h.Ports))
	}

	if h.Ports[0].Port != 3000 {
		t.Errorf("expected port 3000, got %d", h.Ports[0].Port)
	}

	if h.Ports[1].Port != 3001 {
		t.Errorf("expected port 3001, got %d", h.Ports[1].Port)
	}
}

func TestHistory_GetFrozenPorts(t *testing.T) {
	now := time.Now()

	h := &History{
		Ports: []PortRecord{
			{Port: 3000, IssuedAt: now.Add(-30 * time.Minute)}, // 30 min ago
			{Port: 3001, IssuedAt: now.Add(-90 * time.Minute)}, // 90 min ago
			{Port: 3002, IssuedAt: now.Add(-5 * time.Minute)},  // 5 min ago
		},
	}

	// 60 minute freeze period
	frozen := h.GetFrozenPorts(60)

	if !frozen[3000] {
		t.Error("port 3000 (30 min ago) should be frozen")
	}

	if frozen[3001] {
		t.Error("port 3001 (90 min ago) should NOT be frozen")
	}

	if !frozen[3002] {
		t.Error("port 3002 (5 min ago) should be frozen")
	}
}

func TestHistory_GetFrozenPorts_Disabled(t *testing.T) {
	h := &History{
		Ports: []PortRecord{
			{Port: 3000, IssuedAt: time.Now()},
		},
	}

	// Freeze disabled (0)
	frozen := h.GetFrozenPorts(0)

	if len(frozen) != 0 {
		t.Errorf("expected no frozen ports when disabled, got %d", len(frozen))
	}
}

func TestHistory_Cleanup(t *testing.T) {
	now := time.Now()

	h := &History{
		Ports: []PortRecord{
			{Port: 3000, IssuedAt: now.Add(-30 * time.Minute)}, // keep
			{Port: 3001, IssuedAt: now.Add(-90 * time.Minute)}, // remove
			{Port: 3002, IssuedAt: now.Add(-5 * time.Minute)},  // keep
		},
	}

	h.Cleanup(60) // 60 minute cleanup

	if len(h.Ports) != 2 {
		t.Errorf("expected 2 records after cleanup, got %d", len(h.Ports))
	}

	for _, record := range h.Ports {
		if record.Port == 3001 {
			t.Error("port 3001 should have been cleaned up")
		}
	}
}

func TestHistory_Cleanup_DisabledClearsAll(t *testing.T) {
	h := &History{
		Ports: []PortRecord{
			{Port: 3000, IssuedAt: time.Now()},
			{Port: 3001, IssuedAt: time.Now()},
		},
	}

	h.Cleanup(0) // Disabled

	if len(h.Ports) != 0 {
		t.Errorf("expected all records cleared when disabled, got %d", len(h.Ports))
	}
}
