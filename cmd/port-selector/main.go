package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/dapi/port-selector/internal/allocations"
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
		case "-l", "--list":
			if err := runList(); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "--forget":
			if err := runForget(); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "--forget-all":
			if err := runForgetAll(); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "-c", "--lock":
			portArg := 0
			if len(os.Args) > 2 {
				var err error
				portArg, err = strconv.Atoi(os.Args[2])
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: invalid port number: %s\n", os.Args[2])
					os.Exit(1)
				}
			}
			if err := runLock(portArg); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "-u", "--unlock":
			portArg := 0
			if len(os.Args) > 2 {
				var err error
				portArg, err = strconv.Atoi(os.Args[2])
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: invalid port number: %s\n", os.Args[2])
					os.Exit(1)
				}
			}
			if err := runUnlock(portArg); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
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

	// Get config directory for cache, history, and allocations
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Load allocations
	allocs := allocations.Load(configDir)

	// Auto-cleanup expired allocations (silent)
	ttl := cfg.GetAllocationTTL()
	if ttl > 0 {
		if removed := allocs.RemoveExpired(ttl); removed > 0 {
			// Save cleaned allocations
			if err := allocations.Save(configDir, allocs); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to save allocations after cleanup: %v\n", err)
			}
		}
	}

	// Check if current directory already has an allocated port
	if existing := allocs.FindByDirectory(cwd); existing != nil {
		// Check if the previously allocated port is free
		if port.IsPortFree(existing.Port) {
			// Update last_used_at timestamp
			if !allocs.UpdateLastUsed(cwd) {
				// Should not happen since we just found the allocation, but log for debugging
				fmt.Fprintf(os.Stderr, "warning: allocation for %s disappeared during timestamp update\n", cwd)
			}
			if err := allocations.Save(configDir, allocs); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to update allocation timestamp: %v\n", err)
			}
			fmt.Println(existing.Port)
			return nil
		}
		// Port is busy, need to allocate a new one
	}

	// Get last used port from cache for round-robin behavior
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

	// Add locked ports from other directories to the exclusion set
	lockedPorts := allocs.GetLockedPortsForExclusion(cwd)
	for port := range lockedPorts {
		if frozenPorts == nil {
			frozenPorts = make(map[int]bool)
		}
		frozenPorts[port] = true
	}

	// Find a free port (excluding frozen and locked ones)
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

	// Save allocation for this directory
	allocs.SetAllocation(cwd, freePort)
	if err := allocations.Save(configDir, allocs); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to save allocation: %v\n", err)
	}

	// Save to cache
	if err := cache.SetLastUsed(configDir, freePort); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to save cache: %v\n", err)
	}

	// Output the port
	fmt.Println(freePort)
	return nil
}

func runForget() error {
	// Get config directory
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Load allocations
	allocs := allocations.Load(configDir)

	// Remove allocation for current directory
	removed, found := allocs.RemoveByDirectory(cwd)
	if !found {
		fmt.Printf("No allocation found for %s\n", cwd)
		return nil
	}

	// Save updated allocations
	if err := allocations.Save(configDir, allocs); err != nil {
		return fmt.Errorf("failed to save allocations: %w", err)
	}

	fmt.Printf("Cleared allocation for %s (was port %d)\n", cwd, removed.Port)
	return nil
}

func runForgetAll() error {
	// Get config directory
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	// Load allocations
	allocs := allocations.Load(configDir)

	count := allocs.RemoveAll()
	if count == 0 {
		fmt.Println("No allocations found")
		return nil
	}

	// Save updated allocations
	if err := allocations.Save(configDir, allocs); err != nil {
		return fmt.Errorf("failed to save allocations: %w", err)
	}

	fmt.Printf("Cleared %d allocation(s)\n", count)
	return nil
}

func runLock(portArg int) error {
	// Get config directory
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Load allocations
	allocs := allocations.Load(configDir)

	var found bool
	var lockedPort int

	if portArg > 0 {
		// Lock specific port
		alloc := allocs.FindByPort(portArg)
		if alloc == nil {
			return fmt.Errorf("no allocation found for port %d", portArg)
		}
		found = allocs.SetLockedByPort(portArg, true)
		lockedPort = portArg
	} else {
		// Lock port for current directory
		alloc := allocs.FindByDirectory(cwd)
		if alloc == nil {
			return fmt.Errorf("no allocation found for %s (run port-selector first)", cwd)
		}
		found = allocs.SetLocked(cwd, true)
		lockedPort = alloc.Port
	}

	if !found {
		return fmt.Errorf("failed to lock port")
	}

	// Save updated allocations
	if err := allocations.Save(configDir, allocs); err != nil {
		return fmt.Errorf("failed to save allocations: %w", err)
	}

	fmt.Printf("Locked port %d\n", lockedPort)
	return nil
}

func runUnlock(portArg int) error {
	// Get config directory
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Load allocations
	allocs := allocations.Load(configDir)

	var found bool
	var unlockedPort int

	if portArg > 0 {
		// Unlock specific port
		alloc := allocs.FindByPort(portArg)
		if alloc == nil {
			return fmt.Errorf("no allocation found for port %d", portArg)
		}
		found = allocs.SetLockedByPort(portArg, false)
		unlockedPort = portArg
	} else {
		// Unlock port for current directory
		alloc := allocs.FindByDirectory(cwd)
		if alloc == nil {
			return fmt.Errorf("no allocation found for %s", cwd)
		}
		found = allocs.SetLocked(cwd, false)
		unlockedPort = alloc.Port
	}

	if !found {
		return fmt.Errorf("failed to unlock port")
	}

	// Save updated allocations
	if err := allocations.Save(configDir, allocs); err != nil {
		return fmt.Errorf("failed to save allocations: %w", err)
	}

	fmt.Printf("Unlocked port %d\n", unlockedPort)
	return nil
}

func runList() error {
	// Get config directory
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	// Load allocations
	allocs := allocations.Load(configDir)

	if len(allocs.Allocations) == 0 {
		fmt.Println("No port allocations found.")
		return nil
	}

	// Print header and allocations using tabwriter for aligned columns
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PORT\tSTATUS\tLOCKED\tDIRECTORY\tASSIGNED")

	for _, alloc := range allocs.SortedByPort() {
		status := "free"
		if !port.IsPortFree(alloc.Port) {
			status = "busy"
		}

		locked := ""
		if alloc.Locked {
			locked = "yes"
		}

		timestamp := alloc.AssignedAt.Local().Format("2006-01-02 15:04")
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", alloc.Port, status, locked, alloc.Directory, timestamp)
	}

	w.Flush()
	return nil
}

func printHelp() {
	fmt.Println(`Usage: port-selector [options]

Finds and returns a free port from configured range.
Remembers which port was assigned to which directory.

Options:
  -h, --help        Show this help message
  -v, --version     Show version
  -l, --list        List all port allocations
  -c, --lock [PORT] Lock port for current directory (or specified port)
  -u, --unlock [PORT] Unlock port for current directory (or specified port)
  --forget          Clear port allocation for current directory
  --forget-all      Clear all port allocations

Port Locking:
  Locked ports are reserved for their directory and won't be allocated
  to other directories. Use this for long-running services.

Configuration:
  ~/.config/port-selector/default.yaml

  Available options:
    portStart: 3000            # Start of port range
    portEnd: 4000              # End of port range
    freezePeriodMinutes: 1440  # How long to avoid reusing a port
    allocationTTL: 30d         # Auto-expire allocations (e.g., 30d, 720h, 0 to disable)

Author:
  Danil Pismenny <https://github.com/dapi>`)
}

func printVersion() {
	fmt.Printf("port-selector version %s\n", version)
}
