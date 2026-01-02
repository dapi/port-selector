package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.PortStart != DefaultPortStart {
		t.Errorf("expected PortStart=%d, got %d", DefaultPortStart, cfg.PortStart)
	}
	if cfg.PortEnd != DefaultPortEnd {
		t.Errorf("expected PortEnd=%d, got %d", DefaultPortEnd, cfg.PortEnd)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "valid config",
			cfg:     Config{PortStart: 3000, PortEnd: 4000},
			wantErr: false,
		},
		{
			name:    "portStart equals portEnd",
			cfg:     Config{PortStart: 3000, PortEnd: 3000},
			wantErr: true,
		},
		{
			name:    "portStart greater than portEnd",
			cfg:     Config{PortStart: 4000, PortEnd: 3000},
			wantErr: true,
		},
		{
			name:    "portStart is zero",
			cfg:     Config{PortStart: 0, PortEnd: 4000},
			wantErr: true,
		},
		{
			name:    "portEnd is zero",
			cfg:     Config{PortStart: 3000, PortEnd: 0},
			wantErr: true,
		},
		{
			name:    "portStart negative",
			cfg:     Config{PortStart: -1, PortEnd: 4000},
			wantErr: true,
		},
		{
			name:    "portEnd out of range",
			cfg:     Config{PortStart: 3000, PortEnd: 70000},
			wantErr: true,
		},
		{
			name:    "minimum valid range",
			cfg:     Config{PortStart: 1, PortEnd: 2},
			wantErr: false,
		},
		{
			name:    "maximum valid range",
			cfg:     Config{PortStart: 65534, PortEnd: 65535},
			wantErr: false,
		},
		{
			name:    "freezePeriodMinutes negative",
			cfg:     Config{PortStart: 3000, PortEnd: 4000, FreezePeriodMinutes: -1},
			wantErr: true,
		},
		{
			name:    "freezePeriodMinutes zero is valid",
			cfg:     Config{PortStart: 3000, PortEnd: 4000, FreezePeriodMinutes: 0},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadAndSave(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Override config directory for testing
	origUserConfigDir := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Setenv("XDG_CONFIG_HOME", origUserConfigDir)

	// Test loading when config doesn't exist (should create default)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.PortStart != DefaultPortStart {
		t.Errorf("expected PortStart=%d, got %d", DefaultPortStart, cfg.PortStart)
	}
	if cfg.PortEnd != DefaultPortEnd {
		t.Errorf("expected PortEnd=%d, got %d", DefaultPortEnd, cfg.PortEnd)
	}

	// Check that config file was created
	configPath := filepath.Join(tmpDir, appName, configFileName)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}

	// Test saving custom config
	customCfg := &Config{PortStart: 5000, PortEnd: 6000}
	if err := Save(customCfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Test loading custom config
	loadedCfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loadedCfg.PortStart != 5000 {
		t.Errorf("expected PortStart=5000, got %d", loadedCfg.PortStart)
	}
	if loadedCfg.PortEnd != 6000 {
		t.Errorf("expected PortEnd=6000, got %d", loadedCfg.PortEnd)
	}
}

func TestLoadInvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()

	origUserConfigDir := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Setenv("XDG_CONFIG_HOME", origUserConfigDir)

	// Create invalid config
	configDir := filepath.Join(tmpDir, appName)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	configPath := filepath.Join(configDir, configFileName)

	// Write invalid YAML (portStart > portEnd)
	invalidYAML := []byte("portStart: 5000\nportEnd: 3000\n")
	if err := os.WriteFile(configPath, invalidYAML, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Error("expected error for invalid config, got nil")
	}
}

func TestLoadMalformedYAML(t *testing.T) {
	tmpDir := t.TempDir()

	origUserConfigDir := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Setenv("XDG_CONFIG_HOME", origUserConfigDir)

	// Create malformed config
	configDir := filepath.Join(tmpDir, appName)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	configPath := filepath.Join(configDir, configFileName)

	malformedYAML := []byte("this is not valid yaml: [")
	if err := os.WriteFile(configPath, malformedYAML, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Error("expected error for malformed YAML, got nil")
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     time.Duration
		wantErr  bool
	}{
		{"empty string", "", 0, false},
		{"zero", "0", 0, false},
		{"days", "30d", 30 * 24 * time.Hour, false},
		{"single day", "1d", 24 * time.Hour, false},
		{"hours", "720h", 720 * time.Hour, false},
		{"minutes", "60m", 60 * time.Minute, false},
		{"seconds", "3600s", 3600 * time.Second, false},
		{"combined go duration", "24h30m", 24*time.Hour + 30*time.Minute, false},
		{"invalid format", "30days", 0, true},
		{"negative days", "-30d", 0, true},
		{"just letters", "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestConfig_GetAllocationTTL(t *testing.T) {
	tests := []struct {
		name     string
		ttl      string
		expected time.Duration
	}{
		{"empty", "", 0},
		{"zero", "0", 0},
		{"30 days", "30d", 30 * 24 * time.Hour},
		{"720 hours", "720h", 720 * time.Hour},
		{"invalid (returns 0)", "invalid", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				PortStart:     3000,
				PortEnd:       4000,
				AllocationTTL: tt.ttl,
			}
			got := cfg.GetAllocationTTL()
			if got != tt.expected {
				t.Errorf("GetAllocationTTL() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConfig_Validate_AllocationTTL(t *testing.T) {
	tests := []struct {
		name    string
		ttl     string
		wantErr bool
	}{
		{"empty is valid", "", false},
		{"zero is valid", "0", false},
		{"valid days", "30d", false},
		{"valid hours", "720h", false},
		{"valid combined", "24h30m", false},
		{"invalid format", "30days", true},
		{"invalid string", "abc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				PortStart:     3000,
				PortEnd:       4000,
				AllocationTTL: tt.ttl,
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
