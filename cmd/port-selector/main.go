package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/dapi/port-selector/internal/cache"
	"github.com/dapi/port-selector/internal/config"
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

	// Get config directory for cache
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	// Get last used port from cache
	lastUsed := cache.GetLastUsed(configDir)

	// Find a free port
	freePort, err := port.FindFreePort(cfg.PortStart, cfg.PortEnd, lastUsed)
	if err != nil {
		if errors.Is(err, port.ErrAllPortsBusy) {
			return fmt.Errorf("all ports in range %d-%d are busy", cfg.PortStart, cfg.PortEnd)
		}
		return fmt.Errorf("failed to find free port: %w", err)
	}

	// Save to cache (ignore errors - cache is optional)
	_ = cache.SetLastUsed(configDir, freePort)

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
