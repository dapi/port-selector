// Package config handles reading and writing configuration files.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// dayPattern matches duration strings with day suffix (e.g., "30d", "7d")
var dayPattern = regexp.MustCompile(`^(\d+)d$`)

const (
	appName        = "port-selector"
	configFileName = "default.yaml"

	DefaultPortStart           = 3000
	DefaultPortEnd             = 4000
	DefaultFreezePeriodMinutes = 1440 // 24 hours
	DefaultAllocationTTL       = ""   // empty means disabled
)

// Config represents the application configuration.
type Config struct {
	PortStart           int    `yaml:"portStart"`
	PortEnd             int    `yaml:"portEnd"`
	FreezePeriodMinutes int    `yaml:"freezePeriodMinutes"`
	AllocationTTL       string `yaml:"allocationTTL,omitempty"`
}

// DefaultConfig returns a new Config with default values.
func DefaultConfig() *Config {
	return &Config{
		PortStart:           DefaultPortStart,
		PortEnd:             DefaultPortEnd,
		FreezePeriodMinutes: DefaultFreezePeriodMinutes,
		AllocationTTL:       DefaultAllocationTTL,
	}
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.PortStart <= 0 {
		return errors.New("portStart must be positive")
	}
	if c.PortEnd <= 0 {
		return errors.New("portEnd must be positive")
	}
	if c.PortStart >= c.PortEnd {
		return fmt.Errorf("portStart (%d) must be less than portEnd (%d)", c.PortStart, c.PortEnd)
	}
	if c.PortStart < 1 || c.PortStart > 65535 {
		return fmt.Errorf("portStart (%d) must be between 1 and 65535", c.PortStart)
	}
	if c.PortEnd < 1 || c.PortEnd > 65535 {
		return fmt.Errorf("portEnd (%d) must be between 1 and 65535", c.PortEnd)
	}
	if c.FreezePeriodMinutes < 0 {
		return errors.New("freezePeriodMinutes must be non-negative")
	}
	if c.AllocationTTL != "" && c.AllocationTTL != "0" {
		if _, err := ParseDuration(c.AllocationTTL); err != nil {
			return fmt.Errorf("invalid allocationTTL: %w", err)
		}
	}
	return nil
}

// ParseDuration parses a duration string like "30d", "720h", "24h30m".
// Supports: d (days), h (hours), m (minutes), s (seconds).
func ParseDuration(s string) (time.Duration, error) {
	if s == "" || s == "0" {
		return 0, nil
	}

	// Try standard Go duration first (handles h, m, s, ms, etc.)
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// Handle day suffix (e.g., "30d", "7d")
	if matches := dayPattern.FindStringSubmatch(s); matches != nil {
		days, _ := strconv.Atoi(matches[1])
		return time.Duration(days) * 24 * time.Hour, nil
	}

	return 0, fmt.Errorf("cannot parse duration: %s (use format like 30d, 720h, 24h30m)", s)
}

// GetAllocationTTL returns the parsed allocation TTL duration.
// Returns 0 if TTL is disabled, empty, or has an invalid format.
// Logs a warning to stderr if the format is invalid.
func (c *Config) GetAllocationTTL() time.Duration {
	if c.AllocationTTL == "" || c.AllocationTTL == "0" {
		return 0
	}
	d, err := ParseDuration(c.AllocationTTL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: invalid allocationTTL %q, TTL disabled: %v\n", c.AllocationTTL, err)
		return 0
	}
	return d
}

// ConfigDir returns the path to the configuration directory.
func ConfigDir() (string, error) {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user config dir: %w", err)
	}
	return filepath.Join(userConfigDir, appName), nil
}

// ConfigPath returns the full path to the configuration file.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

// Load reads the configuration from disk.
// If the config file doesn't exist, it creates one with default values.
func Load() (*Config, error) {
	configPath, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config
		cfg := DefaultConfig()
		if err := Save(cfg); err != nil {
			// If we can't save, just return defaults without error
			return cfg, nil
		}
		return cfg, nil
	}

	// Read existing config
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// Save writes the configuration to disk.
func Save(cfg *Config) error {
	configPath, err := ConfigPath()
	if err != nil {
		return err
	}

	// Ensure config directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
