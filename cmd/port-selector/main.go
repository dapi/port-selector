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
		case "--scan":
			if err := runScan(); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "-c", "--lock":
			portArg, err := parseOptionalPort()
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			if err := runSetLocked(portArg, true); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "-u", "--unlock":
			portArg, err := parseOptionalPort()
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			if err := runSetLocked(portArg, false); err != nil {
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
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	allocs := allocations.Load(configDir)
	removed, found := allocs.RemoveByDirectory(cwd)
	if !found {
		fmt.Printf("No allocation found for %s\n", cwd)
		return nil
	}

	if err := allocations.Save(configDir, allocs); err != nil {
		return fmt.Errorf("failed to save allocations: %w", err)
	}

	fmt.Printf("Cleared allocation for %s (was port %d)\n", cwd, removed.Port)
	return nil
}

func runForgetAll() error {
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	allocs := allocations.Load(configDir)
	count := allocs.RemoveAll()
	if count == 0 {
		fmt.Println("No allocations found")
		return nil
	}

	if err := allocations.Save(configDir, allocs); err != nil {
		return fmt.Errorf("failed to save allocations: %w", err)
	}

	fmt.Printf("Cleared %d allocation(s)\n", count)
	return nil
}

// parseOptionalPort parses an optional port number from os.Args[2].
// Returns 0 if no port is specified, or the parsed port if valid.
func parseOptionalPort() (int, error) {
	if len(os.Args) <= 2 {
		return 0, nil
	}
	portArg, err := strconv.Atoi(os.Args[2])
	if err != nil || portArg < 1 || portArg > 65535 {
		return 0, fmt.Errorf("invalid port number: %s (must be 1-65535)", os.Args[2])
	}
	return portArg, nil
}

func runSetLocked(portArg int, locked bool) error {
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	allocs := allocations.Load(configDir)

	var targetPort int
	if portArg > 0 {
		targetPort, err = lockSpecificPort(allocs, portArg, cwd, locked)
	} else {
		targetPort, err = lockCurrentDirectory(allocs, cwd, locked)
	}
	if err != nil {
		return err
	}

	if err := allocations.Save(configDir, allocs); err != nil {
		return fmt.Errorf("failed to save allocations: %w", err)
	}

	action := "Locked"
	if !locked {
		action = "Unlocked"
	}
	fmt.Printf("%s port %d\n", action, targetPort)
	return nil
}

// lockSpecificPort handles locking/unlocking a specific port number.
func lockSpecificPort(allocs *allocations.AllocationList, portArg int, cwd string, locked bool) (int, error) {
	alloc := allocs.FindByPort(portArg)
	if alloc != nil {
		// Port already allocated - update its lock status
		if !allocs.SetLockedByPort(portArg, locked) {
			return 0, fmt.Errorf("internal error: allocation for port %d disappeared unexpectedly", portArg)
		}
		return portArg, nil
	}

	// Port not allocated yet
	if !locked {
		return 0, fmt.Errorf("no allocation found for port %d", portArg)
	}

	// Try to allocate and lock the port
	cfg, err := config.Load()
	if err != nil {
		return 0, fmt.Errorf("failed to load config: %w", err)
	}

	if portArg < cfg.PortStart || portArg > cfg.PortEnd {
		return 0, fmt.Errorf("port %d is outside configured range %d-%d", portArg, cfg.PortStart, cfg.PortEnd)
	}

	if !port.IsPortFree(portArg) {
		if procInfo := port.GetPortProcess(portArg); procInfo != nil {
			return 0, fmt.Errorf("port %d is in use by another process (%s)", portArg, procInfo)
		}
		return 0, fmt.Errorf("port %d is in use by another process", portArg)
	}

	existingAlloc := allocs.FindByDirectory(cwd)
	if existingAlloc != nil {
		return 0, fmt.Errorf("directory already has port %d allocated (use --forget first)", existingAlloc.Port)
	}

	allocs.SetAllocation(cwd, portArg)
	if !allocs.SetLocked(cwd, true) {
		return 0, fmt.Errorf("internal error: failed to lock port %d after allocation", portArg)
	}

	return portArg, nil
}

// lockCurrentDirectory handles locking/unlocking the port for the current directory.
func lockCurrentDirectory(allocs *allocations.AllocationList, cwd string, locked bool) (int, error) {
	alloc := allocs.FindByDirectory(cwd)
	if alloc == nil {
		return 0, fmt.Errorf("no allocation found for %s (run port-selector first)", cwd)
	}

	if !allocs.SetLocked(cwd, locked) {
		return 0, fmt.Errorf("internal error: allocation for %s disappeared unexpectedly", cwd)
	}

	return alloc.Port, nil
}

func runList() error {
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	allocs := allocations.Load(configDir)
	if len(allocs.Allocations) == 0 {
		fmt.Println("No port allocations found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PORT\tSTATUS\tPID\tPROCESS\tLOCKED\tDIRECTORY\tASSIGNED")

	for _, alloc := range allocs.SortedByPort() {
		status := "free"
		pid := "-"
		process := "-"

		if !port.IsPortFree(alloc.Port) {
			status = "busy"
			if procInfo := port.GetPortProcess(alloc.Port); procInfo != nil {
				pid = strconv.Itoa(procInfo.PID)
				if procInfo.Name != "" {
					process = procInfo.Name
					if len(process) > 15 {
						process = process[:12] + "..."
					}
				}
			}
		}

		locked := ""
		if alloc.Locked {
			locked = "yes"
		}

		timestamp := alloc.AssignedAt.Local().Format("2006-01-02 15:04")
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n", alloc.Port, status, pid, process, locked, alloc.Directory, timestamp)
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
  --scan            Scan port range and record busy ports with their directories

Port Locking:
  Locked ports are reserved for their directory and won't be allocated
  to other directories. Use this for long-running services.

  Using --lock with a port number will allocate AND lock that port
  to the current directory in one step (if the port is free).

Configuration:
  ~/.config/port-selector/default.yaml

  Available options:
    portStart: 3000            # Start of port range
    portEnd: 4000              # End of port range
    freezePeriodMinutes: 1440  # How long to avoid reusing a port
    allocationTTL: 30d         # Auto-expire allocations (e.g., 30d, 720h, 0 to disable)

Source code:
  https://github.com/dapi/port-selector`)
}

func printVersion() {
	fmt.Printf("port-selector version %s\n", version)
}

func runScan() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	fmt.Printf("Scanning ports %d-%d...\n", cfg.PortStart, cfg.PortEnd)

	allocs := allocations.Load(configDir)
	discovered := 0

	for p := cfg.PortStart; p <= cfg.PortEnd; p++ {
		if port.IsPortFree(p) {
			continue
		}

		// Skip if already allocated
		if existing := allocs.FindByPort(p); existing != nil {
			fmt.Printf("Port %d: already allocated to %s\n", p, existing.Directory)
			continue
		}

		// Port is busy - try to get process info
		procInfo := port.GetPortProcess(p)

		// Add allocation for this port
		if procInfo != nil && procInfo.Cwd != "" {
			allocs.SetAllocation(procInfo.Cwd, p)
		} else {
			allocs.SetUnknownPortAllocation(p)
		}
		discovered++

		// Print status message
		if procInfo != nil {
			if procInfo.Cwd != "" {
				fmt.Printf("Port %d: used by %s (pid=%d, cwd=%s)\n", p, procInfo.Name, procInfo.PID, procInfo.Cwd)
			} else {
				fmt.Printf("Port %d: used by %s (pid=%d, recorded with unknown directory)\n", p, procInfo.Name, procInfo.PID)
			}
		} else {
			fmt.Printf("Port %d: busy (process unknown, recorded)\n", p)
		}
	}

	if discovered > 0 {
		if err := allocations.Save(configDir, allocs); err != nil {
			return fmt.Errorf("failed to save allocations (%d discovered): %w", discovered, err)
		}
		fmt.Printf("\nRecorded %d port(s) to allocations.\n", discovered)
	} else {
		fmt.Println("\nNo new ports to record.")
	}

	return nil
}
