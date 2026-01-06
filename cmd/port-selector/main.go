package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/dapi/port-selector/internal/allocations"
	"github.com/dapi/port-selector/internal/config"
	"github.com/dapi/port-selector/internal/debug"
	"github.com/dapi/port-selector/internal/logger"
	"github.com/dapi/port-selector/internal/pathutil"
	"github.com/dapi/port-selector/internal/port"
)

var version = "dev"

// initLoggerFromConfig initializes the logger using the provided config's Log path.
// Logs a warning to stderr if initialization fails.
func initLoggerFromConfig(cfg *config.Config) {
	if cfg.Log != "" {
		if err := logger.Init(cfg.Log); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to initialize logger: %v\n", err)
		}
	}
}

// loadConfigAndInitLogger loads config and initializes logger.
// Returns the loaded config and any error.
func loadConfigAndInitLogger() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	initLoggerFromConfig(cfg)
	return cfg, nil
}

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

// parseName extracts --name flag and returns the name.
func parseName(args []string) ([]string, string) {
	name := "main"
	var remaining []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--name" {
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "error: --name requires a value\n")
				os.Exit(1)
			}
			name = args[i+1]
			if name == "" {
				fmt.Fprintf(os.Stderr, "error: --name value cannot be empty\n")
				os.Exit(1)
			}
			i++ // skip the name value
		} else if strings.HasPrefix(arg, "--name=") {
			name = strings.TrimPrefix(arg, "--name=")
			if name == "" {
				fmt.Fprintf(os.Stderr, "error: --name value cannot be empty\n")
				os.Exit(1)
			}
		} else {
			remaining = append(remaining, arg)
		}
	}
	return remaining, name
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

	// Extract --name and get remaining args
	args, name := parseName(args)

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
			portArg, _ := parseOptionalPortFromArgs(args)
			if portArg > 0 {
				fmt.Fprintf(os.Stderr, "error: --forget does not accept port number (did you mean --forget-all?)\n")
				os.Exit(1)
			}
			if err := runForget(name); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "--forget-all":
			portArg, _ := parseOptionalPortFromArgs(args)
			if portArg > 0 {
				fmt.Fprintf(os.Stderr, "error: --forget-all does not accept port number\n")
				os.Exit(1)
			}
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
			if err := runSetLocked(portArg, true, name); err != nil {
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
			if err := runSetLocked(portArg, false, name); err != nil {
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

	if err := run(name); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(name string) error {
	debug.Printf("main", "starting port selection")

	// Load configuration and initialize logger
	cfg, err := loadConfigAndInitLogger()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	debug.Printf("main", "config loaded: portStart=%d, portEnd=%d, freezePeriod=%s",
		cfg.PortStart, cfg.PortEnd, cfg.GetFreezePeriod())

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

		// Check if current directory already has an allocated port for this name
		if existing := store.FindByDirectoryAndName(cwd, name); existing != nil {
			debug.Printf("main", "found existing allocation for name '%s': port %d", name, existing.Port)
			// Check if the previously allocated port is free
			if port.IsPortFree(existing.Port) {
				debug.Printf("main", "existing port %d for name '%s' is free, reusing", existing.Port, name)
				// Update last_used timestamp for the specific port being issued
				if !store.UpdateLastUsedByDirectoryAndName(cwd, name) {
					debug.Printf("main", "warning: UpdateLastUsedByDirectoryAndName failed for port %d, name '%s'", existing.Port, name)
				}
				resultPort = existing.Port
				return nil
			}
			debug.Printf("main", "existing port %d for name '%s' is busy, need new allocation", existing.Port, name)
		}

		// Get last used port for round-robin behavior
		lastUsed := store.GetLastIssuedPort()
		debug.Printf("main", "last issued port: %d", lastUsed)

		// Get frozen ports (recently used)
		frozenPorts := store.GetFrozenPorts(cfg.GetFreezePeriod())
		debug.Printf("main", "frozen ports: %d", len(frozenPorts))

		// Add locked ports from other directories to the exclusion set
		lockedPorts := store.GetLockedPortsForExclusion(cwd)
		debug.Printf("main", "locked ports from other directories: %d", len(lockedPorts))
		for p := range lockedPorts {
			frozenPorts[p] = true
		}

		// Add ports allocated to the same directory under different names
		dirPorts := store.GetAllocatedPortsForDirectory(cwd)
		for p := range dirPorts {
			info := store.Allocations[p]
			if info != nil && info.Name != name {
				frozenPorts[p] = true
				debug.Printf("main", "excluded port %d (allocated to same directory with different name '%s')", p, info.Name)
			}
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

		// Save allocation for this directory and name (with safe cleanup of old ports for this name)
		store.SetAllocationWithPortCheckAndName(cwd, freePort, "", name, port.IsPortFree)

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

func runForget(name string) error {
	if _, err := loadConfigAndInitLogger(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

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
		var removed *allocations.Allocation
		var found bool
		if name == "main" {
			// For backward compatibility, --forget without --name removes all allocations for directory
			allAllocs := store.SortedByPort()
			var dirAllocs []allocations.Allocation
			for _, alloc := range allAllocs {
				if alloc.Directory == cwd {
					dirAllocs = append(dirAllocs, alloc)
				}
			}
			if len(dirAllocs) == 0 {
				found = false
			} else if len(dirAllocs) == 1 {
				removed = &dirAllocs[0]
				delete(store.Allocations, removed.Port)
				logger.Log(logger.AllocDelete, logger.Field("port", removed.Port), logger.Field("dir", cwd), logger.Field("name", removed.Name))
				found = true
			} else {
				fmt.Printf("Multiple allocations found for %s:\n", pathutil.ShortenHomePath(cwd))
				for _, alloc := range dirAllocs {
					fmt.Printf("  port %d (name: %s)\n", alloc.Port, alloc.Name)
				}
				fmt.Println("\nUse --forget --name <name> to remove specific allocation, or --forget-all to remove all.")
				return fmt.Errorf("multiple allocations found, use --name to specify which one")
			}
		} else {
			removed, found = store.RemoveByDirectoryAndName(cwd, name)
		}
		if !found {
			fmt.Printf("No allocation found for %s (name: %s)\n", pathutil.ShortenHomePath(cwd), name)
			return nil
		}
		removedPort = removed.Port
		return nil
	})

	if err != nil {
		return err
	}

	if removedPort > 0 {
		fmt.Printf("Cleared allocation for %s (name: %s, was port %d)\n", pathutil.ShortenHomePath(cwd), name, removedPort)
	}
	return nil
}

func runForgetAll() error {
	if _, err := loadConfigAndInitLogger(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

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

func runSetLocked(portArg int, locked bool, name string) error {
	if _, err := loadConfigAndInitLogger(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

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
			targetPort, lockErr = lockSpecificPort(store, portArg, cwd, locked, name)
		} else {
			targetPort, lockErr = lockCurrentDirectory(store, cwd, locked, name)
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
func lockSpecificPort(store *allocations.Store, portArg int, cwd string, locked bool, name string) (int, error) {
	alloc := store.FindByPort(portArg)
	if alloc != nil {
		// Port already allocated - update its lock status
		if name == "" || name == "main" {
			// Legacy behavior - update without checking name
			if !store.SetLockedByPort(portArg, locked) {
				return 0, fmt.Errorf("internal error: allocation for port %d disappeared unexpectedly", portArg)
			}
		} else {
			// Require name match
			if !store.SetLockedByPortAndName(portArg, name, locked) {
				return 0, fmt.Errorf("no allocation found for port %d with name '%s'", portArg, name)
			}
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

	// Check if directory/name already has a port
	existingAlloc := store.FindByDirectoryAndName(cwd, name)
	if existingAlloc != nil {
		return 0, fmt.Errorf("directory already has port %d allocated for name '%s' (use --forget first)", existingAlloc.Port, name)
	}

	store.SetAllocationWithName(cwd, portArg, name)
	if !store.SetLockedByDirectoryAndName(cwd, name, true) {
		return 0, fmt.Errorf("internal error: failed to lock port %d after allocation", portArg)
	}

	return portArg, nil
}

// lockCurrentDirectory handles locking/unlocking the port for the current directory.
func lockCurrentDirectory(store *allocations.Store, cwd string, locked bool, name string) (int, error) {
	alloc := store.FindByDirectoryAndName(cwd, name)
	if alloc == nil {
		return 0, fmt.Errorf("no allocation found for %s with name '%s' (run port-selector first)", cwd, name)
	}

	if !store.SetLockedByDirectoryAndName(cwd, name, locked) {
		return 0, fmt.Errorf("internal error: allocation for %s name '%s' disappeared unexpectedly", cwd, name)
	}

	return alloc.Port, nil
}

func runList() error {
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	// Load without locking - this is read-only and Save() uses atomic writes
	// (temp file + rename), so the file is always in a consistent state.
	store, err := allocations.Load(configDir)
	if err != nil {
		return fmt.Errorf("failed to load allocations: %w", err)
	}
	if store.Count() == 0 {
		fmt.Println("No port allocations found.")
		return nil
	}

	// Build a map of directories that have multiple name allocations
	var allAllocs []allocations.Allocation
	directoriesWithMultipleNames := make(map[string]bool)
	for port, info := range store.Allocations {
		if info == nil {
			continue
		}
		allAllocs = append(allAllocs, allocations.Allocation{
			Port:        port,
			Directory:   info.Directory,
			AssignedAt:  info.AssignedAt,
			LastUsedAt:  info.LastUsedAt,
			Locked:      info.Locked,
			ProcessName: info.ProcessName,
			ContainerID: info.ContainerID,
			Name:        info.Name,
		})
	}
	// Group by directory and check for multiple names
	dirNameCount := make(map[string]map[string]bool)
	for _, alloc := range allAllocs {
		if dirNameCount[alloc.Directory] == nil {
			dirNameCount[alloc.Directory] = make(map[string]bool)
		}
		dirNameCount[alloc.Directory][alloc.Name] = true
		if len(dirNameCount[alloc.Directory]) > 1 {
			directoriesWithMultipleNames[alloc.Directory] = true
		}
	}

	sort.Slice(allAllocs, func(i, j int) bool {
		return allAllocs[i].Port < allAllocs[j].Port
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PORT\tSTATUS\tLOCKED\tUSER\tPID\tPROCESS\tDIRECTORY\tNAME\tASSIGNED")

	hasIncompleteInfo := false

	for _, alloc := range allAllocs {
		status := "free"
		username := "-"
		pid := "-"
		process := "-"
		directory := alloc.Directory
		name := ""

		// Show "main" only if directory has multiple names, otherwise show empty
		if alloc.Name != "main" || directoriesWithMultipleNames[alloc.Directory] {
			name = alloc.Name
		}

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
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", alloc.Port, status, locked, username, pid, process, pathutil.ShortenHomePath(directory), name, timestamp)
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
  --name <NAME>     Use named allocation (default: main)

Named Allocations:
  Use --name to create stable, named port allocations per directory.
  Examples:
    port-selector --name web           # Allocate port for 'web' service
    port-selector --name api           # Allocate another port for 'api' service
    port-selector --list               # See all allocations with NAME column
    port-selector --forget --name web  # Remove specific named allocation

  The default name is 'main' which maintains backward compatibility.

Port Locking:
  Locked ports are reserved for their directory and won't be allocated
  to other directories. Use this for long-running services.

  Using --lock with a port number will allocate AND lock that port
  to the current directory in one step (if the port is free).

Configuration:
  ~/.config/port-selector/default.yaml

  Available options:
    portStart: 3000       # Start of port range
    portEnd: 4000         # End of port range
    freezePeriod: 24h     # How long to avoid reusing a port (e.g., 24h, 30m, 0 to disable)
    allocationTTL: 30d    # Auto-expire allocations (e.g., 30d, 720h, 0 to disable)
    log: ~/.config/port-selector/port-selector.log  # Log file path (optional)

Source code:
  https://github.com/dapi/port-selector`)
}

func printVersion() {
	fmt.Printf("port-selector version %s\n", version)
}

func runScan() error {
	cfg, err := loadConfigAndInitLogger()
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

			// Add allocation for this port (don't replace existing ports for same directory)
			if procInfo != nil && procInfo.Cwd != "" {
				store.AddAllocationForScan(procInfo.Cwd, p, processName, procInfo.ContainerID)
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
						fmt.Printf("Port %d: used by docker-proxy (container=%s, cwd=%s)\n", p, procInfo.ContainerID, cwdShort)
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
