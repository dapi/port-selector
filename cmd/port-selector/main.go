package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/dapi/port-selector/internal/allocations"
	"github.com/dapi/port-selector/internal/config"
	"github.com/dapi/port-selector/internal/debug"
	"github.com/dapi/port-selector/internal/pathutil"
	"github.com/dapi/port-selector/internal/port"
)

var version = "dev"

// parseArgs extracts --verbose flag and returns remaining arguments.
func parseArgs() []string {
	var args []string
	for _, arg := range os.Args[1:] {
		if arg == "--verbose" {
			debug.SetEnabled(true)
		} else {
			args = append(args, arg)
		}
	}
	return args
}

// parseOptionalPortFromArgs parses an optional port number from args[1].
func parseOptionalPortFromArgs(args []string) (int, error) {
	if len(args) <= 1 {
		return 0, nil
	}
	portArg, err := strconv.Atoi(args[1])
	if err != nil || portArg < 1 || portArg > 65535 {
		return 0, fmt.Errorf("invalid port number: %s (must be 1-65535)", args[1])
	}
	return portArg, nil
}

// truncateProcessName shortens process name if it exceeds 15 characters.
func truncateProcessName(name string) string {
	if len(name) > 15 {
		return name[:12] + "..."
	}
	return name
}

func main() {
	// Parse arguments, extracting --verbose flag
	args := parseArgs()

	if len(args) > 0 {
		switch args[0] {
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
			portArg, err := parseOptionalPortFromArgs(args)
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
			portArg, err := parseOptionalPortFromArgs(args)
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
			fmt.Fprintf(os.Stderr, "error: unknown option: %s\n", args[0])
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
	debug.Printf("main", "starting port selection")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	debug.Printf("main", "config loaded: portStart=%d, portEnd=%d, freezePeriod=%d min",
		cfg.PortStart, cfg.PortEnd, cfg.FreezePeriodMinutes)

	// Get config directory for allocations
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}
	debug.Printf("main", "config dir: %s", configDir)

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	debug.Printf("main", "current directory: %s", cwd)

	// Use WithStore for atomic operations
	var resultPort int
	err = allocations.WithStore(configDir, func(store *allocations.Store) error {
		// Auto-cleanup expired allocations
		ttl := cfg.GetAllocationTTL()
		if ttl > 0 {
			if removed := store.RemoveExpired(ttl); removed > 0 {
				debug.Printf("main", "removed %d expired allocations", removed)
			}
		}

		// Check if current directory already has an allocated port
		if existing := store.FindByDirectory(cwd); existing != nil {
			debug.Printf("main", "found existing allocation: port %d", existing.Port)
			// Check if the previously allocated port is free
			if port.IsPortFree(existing.Port) {
				debug.Printf("main", "existing port %d is free, reusing", existing.Port)
				// Update last_used timestamp
				store.UpdateLastUsed(cwd)
				resultPort = existing.Port
				return nil
			}
			debug.Printf("main", "existing port %d is busy, need new allocation", existing.Port)
		}

		// Get last used port for round-robin behavior
		lastUsed := store.GetLastIssuedPort()
		debug.Printf("main", "last issued port: %d", lastUsed)

		// Get frozen ports (recently used)
		frozenPorts := store.GetFrozenPorts(cfg.FreezePeriodMinutes)
		debug.Printf("main", "frozen ports: %d", len(frozenPorts))

		// Add locked ports from other directories to the exclusion set
		lockedPorts := store.GetLockedPortsForExclusion(cwd)
		debug.Printf("main", "locked ports from other directories: %d", len(lockedPorts))
		for p := range lockedPorts {
			frozenPorts[p] = true
		}

		// Find a free port (excluding frozen and locked ones)
		debug.Printf("main", "searching for free port in range %d-%d, starting after %d",
			cfg.PortStart, cfg.PortEnd, lastUsed)
		freePort, err := port.FindFreePortWithExclusions(cfg.PortStart, cfg.PortEnd, lastUsed, frozenPorts)
		if err != nil {
			if errors.Is(err, port.ErrAllPortsBusy) {
				return fmt.Errorf("all ports in range %d-%d are busy or frozen", cfg.PortStart, cfg.PortEnd)
			}
			return fmt.Errorf("failed to find free port: %w", err)
		}
		debug.Printf("main", "found free port: %d", freePort)

		// Save allocation for this directory
		store.SetAllocation(cwd, freePort)

		// Update last issued port
		store.SetLastIssuedPort(freePort)

		resultPort = freePort
		return nil
	})

	if err != nil {
		return err
	}

	// Output the port
	fmt.Println(resultPort)
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

	var removedPort int
	err = allocations.WithStore(configDir, func(store *allocations.Store) error {
		removed, found := store.RemoveByDirectory(cwd)
		if !found {
			fmt.Printf("No allocation found for %s\n", pathutil.ShortenHomePath(cwd))
			return nil
		}
		removedPort = removed.Port
		return nil
	})

	if err != nil {
		return err
	}

	if removedPort > 0 {
		fmt.Printf("Cleared allocation for %s (was port %d)\n", pathutil.ShortenHomePath(cwd), removedPort)
	}
	return nil
}

func runForgetAll() error {
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	var count int
	err = allocations.WithStore(configDir, func(store *allocations.Store) error {
		count = store.RemoveAll()
		return nil
	})

	if err != nil {
		return err
	}

	if count == 0 {
		fmt.Println("No allocations found")
	} else {
		fmt.Printf("Cleared %d allocation(s)\n", count)
	}
	return nil
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

	var targetPort int
	err = allocations.WithStore(configDir, func(store *allocations.Store) error {
		var lockErr error
		if portArg > 0 {
			targetPort, lockErr = lockSpecificPort(store, portArg, cwd, locked)
		} else {
			targetPort, lockErr = lockCurrentDirectory(store, cwd, locked)
		}
		return lockErr
	})

	if err != nil {
		return err
	}

	action := "Locked"
	if !locked {
		action = "Unlocked"
	}
	fmt.Printf("%s port %d\n", action, targetPort)
	return nil
}

// lockSpecificPort handles locking/unlocking a specific port number.
func lockSpecificPort(store *allocations.Store, portArg int, cwd string, locked bool) (int, error) {
	alloc := store.FindByPort(portArg)
	if alloc != nil {
		// Port already allocated - update its lock status
		if !store.SetLockedByPort(portArg, locked) {
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

	existingAlloc := store.FindByDirectory(cwd)
	if existingAlloc != nil {
		return 0, fmt.Errorf("directory already has port %d allocated (use --forget first)", existingAlloc.Port)
	}

	store.SetAllocation(cwd, portArg)
	if !store.SetLocked(cwd, true) {
		return 0, fmt.Errorf("internal error: failed to lock port %d after allocation", portArg)
	}

	return portArg, nil
}

// lockCurrentDirectory handles locking/unlocking the port for the current directory.
func lockCurrentDirectory(store *allocations.Store, cwd string, locked bool) (int, error) {
	alloc := store.FindByDirectory(cwd)
	if alloc == nil {
		return 0, fmt.Errorf("no allocation found for %s (run port-selector first)", cwd)
	}

	if !store.SetLocked(cwd, locked) {
		return 0, fmt.Errorf("internal error: allocation for %s disappeared unexpectedly", cwd)
	}

	return alloc.Port, nil
}

func runList() error {
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	store, err := allocations.Load(configDir)
	if err != nil {
		return err
	}
	if store.Count() == 0 {
		fmt.Println("No port allocations found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PORT\tSTATUS\tLOCKED\tUSER\tPID\tPROCESS\tDIRECTORY\tASSIGNED")

	hasIncompleteInfo := false

	for _, alloc := range store.SortedByPort() {
		status := "free"
		username := "-"
		pid := "-"
		process := "-"
		directory := alloc.Directory

		// Use saved process name from allocation if available
		if alloc.ProcessName != "" {
			process = truncateProcessName(alloc.ProcessName)
		}

		if !port.IsPortFree(alloc.Port) {
			status = "busy"
			if procInfo := port.GetPortProcess(alloc.Port); procInfo != nil {
				if procInfo.User != "" {
					username = procInfo.User
				}
				if procInfo.PID > 0 {
					pid = strconv.Itoa(procInfo.PID)
					// Override with current process name if available
					if procInfo.Name != "" {
						process = truncateProcessName(procInfo.Name)
					}
				} else if procInfo.ContainerID != "" {
					// Docker container detected via fallback
					process = "docker-proxy"
					if procInfo.Cwd != "" && procInfo.Cwd != "/" {
						directory = procInfo.Cwd
					}
				} else {
					// Have user but no PID and no Docker - mark incomplete only if no saved name
					if alloc.ProcessName == "" {
						hasIncompleteInfo = true
					}
				}

				// Use live Docker directory if available and better than saved
				if procInfo.ContainerID != "" && procInfo.Cwd != "" && procInfo.Cwd != "/" {
					directory = procInfo.Cwd
				}
			}
		}

		locked := ""
		if alloc.Locked {
			locked = "yes"
		}

		timestamp := alloc.AssignedAt.Local().Format("2006-01-02 15:04")
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", alloc.Port, status, locked, username, pid, process, pathutil.ShortenHomePath(directory), timestamp)
	}

	w.Flush()

	if hasIncompleteInfo {
		fmt.Fprintln(os.Stderr, "\nTip: Run with sudo for full process info: sudo port-selector --list")
	}

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
  --verbose         Enable debug output (can be combined with other flags)

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

	var discovered int
	var hasIncompleteInfo bool

	err = allocations.WithStore(configDir, func(store *allocations.Store) error {
		for p := cfg.PortStart; p <= cfg.PortEnd; p++ {
			if port.IsPortFree(p) {
				continue
			}

			// Skip if already allocated
			if existing := store.FindByPort(p); existing != nil {
				fmt.Printf("Port %d: already allocated to %s\n", p, pathutil.ShortenHomePath(existing.Directory))
				continue
			}

			// Port is busy - try to get process info
			procInfo := port.GetPortProcess(p)

			// Determine process name for allocation
			processName := ""
			if procInfo != nil {
				if procInfo.ContainerID != "" {
					processName = "docker-proxy"
				} else if procInfo.Name != "" {
					processName = procInfo.Name
				}
			}

			// Add allocation for this port
			if procInfo != nil && procInfo.Cwd != "" {
				store.SetAllocationWithProcess(procInfo.Cwd, p, processName)
			} else {
				store.SetUnknownPortAllocation(p, processName)
			}
			discovered++

			// Print status message
			if procInfo != nil {
				if procInfo.Cwd != "" {
					cwdShort := pathutil.ShortenHomePath(procInfo.Cwd)
					if procInfo.PID > 0 {
						fmt.Printf("Port %d: used by %s (pid=%d, cwd=%s)\n", p, procInfo.Name, procInfo.PID, cwdShort)
					} else if procInfo.ContainerID != "" {
						fmt.Printf("Port %d: used by docker-proxy (cwd=%s)\n", p, cwdShort)
					} else if procInfo.User != "" {
						fmt.Printf("Port %d: used by user=%s (cwd=%s)\n", p, procInfo.User, cwdShort)
					} else {
						fmt.Printf("Port %d: used by unknown process (cwd=%s)\n", p, cwdShort)
					}
				} else if procInfo.PID > 0 {
					fmt.Printf("Port %d: used by %s (pid=%d, cwd unknown, recorded as unknown)\n", p, procInfo.Name, procInfo.PID)
					hasIncompleteInfo = true
				} else if procInfo.User != "" {
					fmt.Printf("Port %d: used by user=%s, cwd unknown, recorded as (unknown:%d)\n", p, procInfo.User, p)
					hasIncompleteInfo = true
				} else {
					fmt.Printf("Port %d: busy (process unknown, recorded)\n", p)
					hasIncompleteInfo = true
				}
			} else {
				fmt.Printf("Port %d: busy (process unknown, recorded)\n", p)
				hasIncompleteInfo = true
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	if discovered > 0 {
		fmt.Printf("\nRecorded %d port(s) to allocations.\n", discovered)
	} else {
		fmt.Println("\nNo new ports to record.")
	}

	if hasIncompleteInfo {
		fmt.Fprintln(os.Stderr, "\nTip: Run with sudo for full process info: sudo port-selector --scan")
	}

	return nil
}
