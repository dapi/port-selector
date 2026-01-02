package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/dapi/port-selector/internal/cache"
	"github.com/dapi/port-selector/internal/config"
	"github.com/dapi/port-selector/internal/history"
	"github.com/dapi/port-selector/internal/port"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-h", "--help":
			printHelp()
			return
		case "-v", "--version":
			printVersion()
			return
		default:
			fmt.Fprintf(os.Stderr, "error: unknown option: %s\n", os.Args[1])
			printHelp()
			os.Exit(1)
		}
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Get config directory for cache and history
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	// Get last used port from cache
	lastUsed := cache.GetLastUsed(configDir)

	// Load history and get frozen ports
	hist, err := history.Load(configDir)
	if err != nil {
		// Non-fatal: continue without history, but warn user
		fmt.Fprintf(os.Stderr, "warning: failed to load history, freeze period disabled: %v\n", err)
		hist = &history.History{}
	}

	// Cleanup old records
	hist.Cleanup(cfg.FreezePeriodMinutes)

	// Get frozen ports
	frozenPorts := hist.GetFrozenPorts(cfg.FreezePeriodMinutes)

	// Find a free port (excluding frozen ones)
	freePort, err := port.FindFreePortWithExclusions(cfg.PortStart, cfg.PortEnd, lastUsed, frozenPorts)
	if err != nil {
		if errors.Is(err, port.ErrAllPortsBusy) {
			return fmt.Errorf("all ports in range %d-%d are busy or frozen", cfg.PortStart, cfg.PortEnd)
		}
		return fmt.Errorf("failed to find free port: %w", err)
	}

	// Add to history and save
	hist.AddPort(freePort)
	if err := hist.Save(configDir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to save history: %v\n", err)
	}

	// Save to cache
	if err := cache.SetLastUsed(configDir, freePort); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to save cache: %v\n", err)
	}

	// Output the port
	fmt.Println(freePort)
	return nil
}

func printHelp() {
	fmt.Println(`Usage: port-selector [options]

Finds and returns a free port from configured range.

Options:
  -h, --help     Show this help message
  -v, --version  Show version

Configuration:
  ~/.config/port-selector/default.yaml`)
}

func printVersion() {
	fmt.Printf("port-selector version %s\n", version)
}
