package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

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

// parseNameFromArgs extracts --name flag and returns the name and remaining arguments.
// Returns "main" as default if --name is not provided.
// Returns error if --name is provided with empty value.
func parseNameFromArgs(args []string) (string, []string, error) {
	name := "main"
	var remaining []string
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--name" {
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--name requires a value")
			}
			name = args[i+1]
			if name == "" {
				return "", nil, fmt.Errorf("--name cannot be empty")
			}
			i += 2 // skip --name and its value
		} else if strings.HasPrefix(arg, "--name=") {
			name = strings.TrimPrefix(arg, "--name=")
			if name == "" {
				return "", nil, fmt.Errorf("--name cannot be empty")
			}
			i++ // skip this arg
		} else {
			remaining = append(remaining, arg)
			i++
		}
	}
	return name, remaining, nil
}

// parseForceFromArgs extracts --force flag and returns whether it was present and remaining arguments.
func parseForceFromArgs(args []string) (bool, []string) {
	force := false
	var remaining []string
	for _, arg := range args {
		if arg == "--force" || arg == "-f" {
			force = true
		} else {
			remaining = append(remaining, arg)
		}
	}
	return force, remaining
}

// parseOptionalPortFromArgs parses an optional port number from args.
// It looks for a port number at the end of the args array.
// If a non-numeric argument is provided where a port is expected, returns an error.
func parseOptionalPortFromArgs(args []string) (int, error) {
	if len(args) == 0 {
		return 0, nil
	}

	// If the last argument looks like a flag (starts with --), there's no port specified
	lastArg := args[len(args)-1]
	if strings.HasPrefix(lastArg, "--") {
		return 0, nil
	}

	// Try to parse the last argument as a port number
	portArg, err := strconv.Atoi(lastArg)
	if err != nil {
		// If it can't be parsed as a number and doesn't look like a flag, it's an error
		return 0, fmt.Errorf("invalid port number: %s (must be 1-65535)", lastArg)
	}
	// Only return the port if it's in valid range
	if portArg < 1 || portArg > 65535 {
		return 0, fmt.Errorf("invalid port number: %s (must be 1-65535)", lastArg)
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

// truncateDirectoryPath truncates a directory path to maxLen characters.
// Tries to preserve path structure by keeping the last parts and compressing the middle.
func truncateDirectoryPath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}

	// Special case for paths starting with ~/ - treat ~ as part of first component
	prefix := ""
	rest := path
	if strings.HasPrefix(path, "~/") {
		prefix = "~/"
		rest = path[2:] // Skip "~/"
	}

	// Split the rest of the path into components
	parts := strings.Split(rest, "/")
	if len(parts) <= 2 {
		// Not enough parts to compress, use simple truncation
		if len(path) > maxLen {
			return path[:maxLen-3] + "..."
		}
		return path
	}

	// Try to keep: prefix + "..." + parent/filename
	// Example: ~/code/worktrees/feature/103-manager-reply-from-dashboard (4 parts after ~/)
	// Desired: ~/.../feature/103-manager-reply-from-dashboard
	if len(parts) >= 2 {
		// Calculate available space for path (after "~/.../")
		availableForPath := maxLen - len(prefix) - 5 // 5 for ".../"

		if availableForPath >= 15 { // Need at least some space for content
			// Start with parent/filename (last 2 parts)
			parent := parts[len(parts)-2]
			filename := parts[len(parts)-1]
			fullPath := parent + "/" + filename

			if len(fullPath) <= availableForPath {
				// Perfect fit!
				return prefix + ".../" + fullPath
			}

			// Try to truncate just the filename, keeping full parent
			spaceForFilename := availableForPath - len(parent) - 1 // 1 for "/"
			if spaceForFilename >= 8 && len(filename) > spaceForFilename {
				// Keep full parent + truncate filename with "..."
				return prefix + ".../" + parent + "/..." + filename[len(filename)-spaceForFilename+3:]
			}

			// If parent is too long, just keep the filename (last part)
			if len(filename) <= availableForPath {
				return prefix + ".../" + filename
			}

			// Last resort: truncate the filename itself
			if availableForPath > 8 && len(filename) > availableForPath {
				return prefix + ".../..." + filename[len(filename)-availableForPath+3:]
			}
		}
	}

	// Fallback: Find the last slash and try to preserve prefix
	lastSlash := strings.LastIndex(rest, "/")

	// If there's a reasonable prefix (at least 5 chars after prefix), try to preserve it
	if lastSlash > 5 && (len(prefix)+lastSlash) < maxLen-15 {
		dirPrefix := prefix + rest[:lastSlash+1]    // Include the slash
		remainingLen := maxLen - len(dirPrefix) - 3 // 3 chars for "..."

		if remainingLen > 5 {
			// Take the end of the name (last 'remainingLen' chars)
			name := rest[lastSlash+1:]
			if len(name) > remainingLen {
				return dirPrefix + "..." + name[len(name)-remainingLen:]
			}
		}
	}

	// Final fallback: truncate in the middle
	remaining := maxLen - 3
	if remaining <= 0 {
		return "..."
	}

	firstLen := remaining / 2
	secondLen := remaining - firstLen

	return path[:firstLen] + "..." + path[len(path)-secondLen:]
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
			name, remainingArgs, err := parseNameFromArgs(args[1:])
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			if err := runForget(name, remainingArgs); err != nil {
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
		case "--refresh":
			if err := runRefresh(); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "-c", "--lock":
			name, remainingArgs, err := parseNameFromArgs(args[1:])
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			force, remainingArgs := parseForceFromArgs(remainingArgs)
			portArg, err := parseOptionalPortFromArgs(remainingArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			if err := runSetLocked(name, portArg, true, force); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		case "-u", "--unlock":
			name, remainingArgs, err := parseNameFromArgs(args[1:])
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			force, remainingArgs := parseForceFromArgs(remainingArgs)
			portArg, err := parseOptionalPortFromArgs(remainingArgs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			if err := runSetLocked(name, portArg, false, force); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		default:
			// Check if the first arg is a --name flag
			if strings.HasPrefix(args[0], "--name") {
				name, remainingArgs, err := parseNameFromArgs(args)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: %v\n", err)
					os.Exit(1)
				}
				if len(remainingArgs) > 0 {
					fmt.Fprintf(os.Stderr, "error: unknown option: %s\n", remainingArgs[0])
					printHelp()
					os.Exit(1)
				}
				if err := runWithName(name); err != nil {
					fmt.Fprintf(os.Stderr, "error: %v\n", err)
					os.Exit(1)
				}
				return
			}
			fmt.Fprintf(os.Stderr, "error: unknown option: %s\n", args[0])
			printHelp()
			os.Exit(1)
		}
	}

	// No args - run with default name "main"
	if err := runWithName("main"); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// runWithName runs port selection with the given name.
func runWithName(name string) error {
	debug.Printf("main", "starting port selection with name=%s", name)

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
		// Uses priority: locked+free > locked+busy > unlocked+free > unlocked+busy(skip)
		if existing := store.FindByDirectoryAndNameWithPriority(cwd, name, port.IsPortFree); existing != nil {
			debug.Printf("main", "found existing allocation for name %s: port %d (locked=%v)", name, existing.Port, existing.Locked)
			// If locked+busy, the user's service is already running - return this port
			// If free (locked or not), return this port for reuse
			isFree := port.IsPortFree(existing.Port)
			if isFree || existing.Locked {
				if isFree {
					debug.Printf("main", "existing port %d is free, reusing", existing.Port)
				} else {
					debug.Printf("main", "existing port %d is busy but locked (user's service running), returning it", existing.Port)
				}
				// Update last_used timestamp for the specific port being issued
				if !store.UpdateLastUsedByPort(existing.Port) {
					debug.Printf("main", "warning: UpdateLastUsedByPort failed for port %d", existing.Port)
					fmt.Fprintf(os.Stderr, "warning: failed to update timestamp for port %d\n", existing.Port)
				}
				resultPort = existing.Port
				return nil
			}
			debug.Printf("main", "existing port %d is busy and unlocked, need new allocation", existing.Port)
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

		// Add ports allocated to other names in the same directory to the exclusion set
		otherNamesPorts := make(map[int]bool)
		for port, info := range store.Allocations {
			if info != nil && info.Directory == cwd && info.Name != name {
				otherNamesPorts[port] = true
			}
		}
		debug.Printf("main", "ports for other names in same directory: %d", len(otherNamesPorts))
		for p := range otherNamesPorts {
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

		// Save allocation for this directory and name (with safe cleanup of old ports for this name)
		store.SetAllocationWithName(cwd, freePort, name)

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

func runForget(name string, remainingArgs []string) error {
	if len(remainingArgs) > 0 {
		return fmt.Errorf("unknown arguments: %v", remainingArgs)
	}

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

	// If name is "main" and no --name flag was provided (remainingArgs is empty),
	// remove all allocations for the directory.
	// If --name was explicitly provided (even if it's "main"), remove only that name.
	removeAll := (name == "main" && len(remainingArgs) == 0)

	var removedPort int
	var removedCount int
	err = allocations.WithStore(configDir, func(store *allocations.Store) error {
		if removeAll {
			// Remove all allocations for this directory
			var removed []allocations.Allocation
			for port, info := range store.Allocations {
				if info != nil && info.Directory == cwd {
					removed = append(removed, allocations.Allocation{
						Port:        port,
						Directory:   info.Directory,
						Name:        info.Name,
						AssignedAt:  info.AssignedAt,
						LastUsedAt:  info.LastUsedAt,
						Locked:      info.Locked,
						ProcessName: info.ProcessName,
						ContainerID: info.ContainerID,
					})
					delete(store.Allocations, port)
				}
			}
			removedCount = len(removed)
			if removedCount == 0 {
				fmt.Printf("No allocations found for %s\n", pathutil.ShortenHomePath(cwd))
				return nil
			}
			// For backward compatibility, show the most recent port
			// Find the allocation with most recent LastUsedAt
			var mostRecent *allocations.Allocation
			for i := range removed {
				if mostRecent == nil || removed[i].LastUsedAt.After(mostRecent.LastUsedAt) {
					mostRecent = &removed[i]
				}
			}
			if mostRecent != nil {
				removedPort = mostRecent.Port
			}
		} else {
			// Remove only the specific named allocation
			removed, found := store.RemoveByDirectoryAndName(cwd, name)
			if !found {
				fmt.Printf("No allocation found for %s with name '%s'\n", pathutil.ShortenHomePath(cwd), name)
				return nil
			}
			removedPort = removed.Port
		}
		return nil
	})

	if err != nil {
		return err
	}

	if removeAll {
		if removedCount > 0 {
			fmt.Printf("Cleared %d allocation(s) for %s (most recent was port %d)\n",
				removedCount, pathutil.ShortenHomePath(cwd), removedPort)
		}
	} else {
		if removedPort > 0 {
			fmt.Printf("Cleared allocation '%s' for %s (was port %d)\n",
				name, pathutil.ShortenHomePath(cwd), removedPort)
		}
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

func runSetLocked(name string, portArg int, locked bool, force bool) error {
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
	var reassignedFrom string
	var isExternal bool
	err = allocations.WithStore(configDir, func(store *allocations.Store) error {
		var lockErr error
		if portArg > 0 {
			targetPort, reassignedFrom, isExternal, lockErr = lockSpecificPort(store, name, portArg, cwd, locked, force)
		} else {
			targetPort, lockErr = lockCurrentDirectory(store, name, cwd, locked)
		}
		return lockErr
	})

	if err != nil {
		return err
	}

	// Handle external allocation message
	if isExternal && locked {
		if alloc, _ := allocations.Load(configDir); alloc != nil {
			if info := alloc.FindByPort(targetPort); info != nil {
				processName := info.ExternalProcessName
				if processName == "" {
					processName = "unknown process"
				}
				fmt.Printf("Port %d is externally used by %s, registered as external\n", targetPort, processName)
				return nil
			}
		}
	}

	// Print warning if port was reassigned from another directory
	if reassignedFrom != "" {
		fmt.Fprintf(os.Stderr, "warning: port %d was allocated to %s\n", targetPort, pathutil.ShortenHomePath(reassignedFrom))
		fmt.Printf("Reassigned and locked port %d for '%s' in %s\n", targetPort, name, pathutil.ShortenHomePath(cwd))
	} else {
		action := "Locked"
		if !locked {
			action = "Unlocked"
		}
		fmt.Printf("%s port %d for '%s' in %s\n", action, targetPort, name, pathutil.ShortenHomePath(cwd))
	}
	return nil
}

// lockSpecificPort handles locking/unlocking a specific port number.
// Returns the port, the old directory (if reassigned), isExternal flag, and any error.
//
// Decision Matrix for --lock PORT:
// - Require --force if: port is locked for another directory
// - Block completely (even with --force) if: port is busy on another directory
// - Allow without --force if: port not allocated, or allocated but free and unlocked
// - Special case: port busy but not in allocations — register as external allocation
func lockSpecificPort(store *allocations.Store, name string, portArg int, cwd string, locked bool, force bool) (int, string, bool, error) {
	isBusy := !port.IsPortFree(portArg)
	alloc := store.FindByPort(portArg)

	if alloc != nil {
		// Port already allocated
		if alloc.Directory == cwd {
			// Port belongs to current directory - just update lock status
			if !store.SetLockedByPort(portArg, locked) {
				return 0, "", false, fmt.Errorf("internal error: allocation for port %d disappeared unexpectedly", portArg)
			}
			// Update LockedAt timestamp when locking
			if locked {
				if info := store.Allocations[portArg]; info != nil {
					info.LockedAt = time.Now().UTC()
				}
			}
			return portArg, "", false, nil
		}

		// Port belongs to another directory
		if isBusy {
			// Port is busy on another directory — block completely (even with --force)
			return 0, "", false, fmt.Errorf("port %d is in use by %s; stop the service first",
				portArg, pathutil.ShortenHomePath(alloc.Directory))
		}

		// Port is free — check if it's locked
		if alloc.Locked {
			// Require --force to reassign locked port
			if !force {
				return 0, "", false, fmt.Errorf("port %d is locked by %s\n       use --lock %d --force to reassign it to current directory",
					portArg, pathutil.ShortenHomePath(alloc.Directory), portArg)
			}
		}
		// Port is free and (unlocked OR --force provided) — allow reassignment
		oldDir := alloc.Directory
		store.RemoveByPort(portArg)
		store.SetAllocationWithName(cwd, portArg, name)
		if !store.SetLockedByPort(portArg, true) {
			return 0, "", false, fmt.Errorf("internal error: failed to lock port %d after reassignment", portArg)
		}
		// Update LockedAt timestamp
		if info := store.Allocations[portArg]; info != nil {
			info.LockedAt = time.Now().UTC()
		}
		// Unlock any previously locked ports for this directory+name (invariant: at most one locked)
		// This is done AFTER locking the new port so old locked ports are preserved during SetAllocation
		store.UnlockOtherLockedPorts(cwd, name, portArg)
		return portArg, oldDir, false, nil
	}

	// Port not allocated yet
	if !locked {
		return 0, "", false, fmt.Errorf("no allocation found for port %d", portArg)
	}

	// Try to allocate and lock the port
	cfg, err := config.Load()
	if err != nil {
		return 0, "", false, fmt.Errorf("failed to load config: %w", err)
	}

	if portArg < cfg.PortStart || portArg > cfg.PortEnd {
		return 0, "", false, fmt.Errorf("port %d is outside configured range %d-%d", portArg, cfg.PortStart, cfg.PortEnd)
	}

	if isBusy {
		// Port is busy - get process info and decide what to do
		procInfo := port.GetPortProcess(portArg)

		// Normalize paths for comparison
		cwdNormalized := filepath.Clean(cwd)
		var procCwdNormalized string
		if procInfo != nil && procInfo.Cwd != "" {
			procCwdNormalized = filepath.Clean(procInfo.Cwd)
		}

		// Case 1: Same directory - register as locked
		if procInfo != nil && procCwdNormalized == cwdNormalized {
			store.SetAllocationWithName(cwd, portArg, name)
			if !store.SetLockedByPort(portArg, true) {
				return 0, "", false, fmt.Errorf("internal error: failed to lock port %d", portArg)
			}
			if info := store.Allocations[portArg]; info != nil {
				info.LockedAt = time.Now().UTC()
			}
			return portArg, "", false, nil
		}

		// Case 2: Different directory - register as external
		if procInfo != nil {
			store.SetExternalAllocation(portArg, procInfo.PID, procInfo.User, procInfo.Name, procInfo.Cwd)
			return portArg, "", true, nil
		}

		// Case 3: No process info available - require --force
		if !force {
			return 0, "", false, fmt.Errorf("port %d is in use by unknown process", portArg)
		}
		// With --force: create allocation even though port is busy (user takes responsibility)
	}

	// Allocate and lock the port for this directory and name
	// SetAllocationWithName preserves locked ports (they won't be deleted)
	store.SetAllocationWithName(cwd, portArg, name)
	if !store.SetLockedByPort(portArg, true) {
		return 0, "", false, fmt.Errorf("internal error: failed to lock port %d after allocation", portArg)
	}
	// Update LockedAt timestamp
	if info := store.Allocations[portArg]; info != nil {
		info.LockedAt = time.Now().UTC()
	}

	// Unlock any previously locked ports for this directory+name (invariant: at most one locked)
	// This is done AFTER locking the new port so old locked ports are preserved during SetAllocation
	store.UnlockOtherLockedPorts(cwd, name, portArg)

	return portArg, "", false, nil
}

// lockCurrentDirectory handles locking/unlocking the port for the current directory and name.
func lockCurrentDirectory(store *allocations.Store, name string, cwd string, locked bool) (int, error) {
	alloc := store.FindByDirectoryAndName(cwd, name)
	if alloc == nil {
		return 0, fmt.Errorf("no allocation found for %s with name '%s' (run port-selector first)", cwd, name)
	}

	if !store.SetLockedByPort(alloc.Port, locked) {
		return 0, fmt.Errorf("internal error: allocation for %s with name '%s' disappeared unexpectedly", cwd, name)
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

	// Determine which directories have multiple names
	dirsWithMultipleNames := make(map[string]bool)
	dirNameCount := make(map[string]map[string]bool)
	allAllocs := store.SortedByPort()

	for _, alloc := range allAllocs {
		if dirNameCount[alloc.Directory] == nil {
			dirNameCount[alloc.Directory] = make(map[string]bool)
		}
		dirNameCount[alloc.Directory][alloc.Name] = true
	}

	for dir, names := range dirNameCount {
		if len(names) > 1 {
			dirsWithMultipleNames[dir] = true
		}
	}

	// First pass: collect all directory paths and determine max width (up to 40 chars)
	const maxDirWidth = 40
	allDirectories := make([]string, len(allAllocs))
	maxDirLen := 0

	for i, alloc := range allAllocs {
		directory := alloc.Directory

		// Check if port is busy and has Docker info
		if !port.IsPortFree(alloc.Port) {
			if procInfo := port.GetPortProcess(alloc.Port); procInfo != nil {
				if procInfo.ContainerID != "" && procInfo.Cwd != "" && procInfo.Cwd != "/" {
					directory = procInfo.Cwd
				}
			}
		}

		shortDir := pathutil.ShortenHomePath(directory)
		allDirectories[i] = shortDir

		if len(shortDir) > maxDirLen {
			maxDirLen = len(shortDir)
		}
	}

	// Second pass: format and print output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PORT\tDIRECTORY\tNAME\tSOURCE\tSTATUS\tLOCKED\tUSER\tPID\tPROCESS\tASSIGNED")

	hasIncompleteInfo := false

	for i, alloc := range allAllocs {
		status := "free"
		username := "-"
		pid := "-"
		process := "-"
		directory := alloc.Directory

		// Determine SOURCE and use saved external info for external allocations
		source := "free"
		if alloc.Status == "external" {
			source = "external"
			// For external allocations, use saved process info
			if alloc.ExternalUser != "" {
				username = alloc.ExternalUser
			}
			if alloc.ExternalPID > 0 {
				pid = strconv.Itoa(alloc.ExternalPID)
			}
			if alloc.ExternalProcessName != "" {
				process = truncateProcessName(alloc.ExternalProcessName)
			}
			status = "busy" // External ports are always busy
		} else if alloc.Locked {
			source = "lock"
			// Use saved process name from allocation if available
			if alloc.ProcessName != "" {
				process = truncateProcessName(alloc.ProcessName)
			}
		} else {
			// Normal allocation - use saved process name if available
			if alloc.ProcessName != "" {
				process = truncateProcessName(alloc.ProcessName)
			}
		}

		// For non-external allocations, check live port status
		if alloc.Status != "external" && !port.IsPortFree(alloc.Port) {
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

		// Always show the name (even "main")
		nameStr := alloc.Name

		timestamp := alloc.AssignedAt.Local().Format("2006-01-02 15:04")

		// Get pre-calculated directory string and truncate if needed
		shortDir := allDirectories[i]
		// If directory was updated by Docker check, re-shorten it
		if directory != alloc.Directory {
			shortDir = pathutil.ShortenHomePath(directory)
		}
		// Cap at 40 characters maximum
		if len(shortDir) > maxDirWidth {
			shortDir = truncateDirectoryPath(shortDir, maxDirWidth)
		}

		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", alloc.Port, shortDir, nameStr, source, status, locked, username, pid, process, timestamp)
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
  -h, --help           Show this help message
  -v, --version        Show version
  -l, --list           List all port allocations
  -c, --lock [PORT]    Lock port for current directory and name (or specified port)
  -u, --unlock [PORT]  Unlock port for current directory and name (or specified port)
  --force, -f          Force lock a busy port or locked port from another directory
  --forget             Clear all port allocations for current directory
  --forget --name NAME Clear port allocation for current directory with specific name
  --forget-all         Clear all port allocations
  --scan               Scan port range and record busy ports with their directories
  --refresh            Refresh external port allocations (remove stale entries)
  --name NAME          Use named allocation (default: "main")
  --verbose            Enable debug output (can be combined with other flags)

Named Allocations:
  --name <name> creates a stable, per-directory named allocation.
  The same directory can have multiple named allocations (web/api/db/etc.).
  Default name is "main" when --name is not provided.

Examples:
  port-selector                    # Use default name "main"
  port-selector --name postgres    # Named allocation for postgres
  port-selector --name web         # Named allocation for web
  port-selector --list             # Show all allocations with NAME column
  port-selector --lock             # Lock "main" allocation
  port-selector --lock --name web  # Lock "web" allocation
  port-selector --unlock --name db # Unlock "db" allocation
  port-selector --forget           # Forget all allocations for directory
  port-selector --forget --name api # Forget only "api" allocation
  port-selector --refresh          # Remove stale external port allocations

Port Locking:
  Locked ports are reserved and won't be allocated to other directories.
  Use this for long-running services.

  Using --lock with a port number will allocate AND lock that port
  to the current directory/name in one step.

  When --lock PORT targets another directory's port:
  - Free + unlocked: reassigned without --force (abandoned allocation)
  - Free + locked: requires --force to reassign
  - Busy (any): blocked completely — stop the service first

  When --lock PORT targets a busy unallocated port:
  - Requires --force (you take responsibility for the conflict)

  If the port is already in use by another directory, it will be
  registered as an external allocation instead of failing.

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

func runRefresh() error {
	if _, err := loadConfigAndInitLogger(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	var removedCount int
	var totalCount int

	err = allocations.WithStore(configDir, func(store *allocations.Store) error {
		for _, info := range store.Allocations {
			if info != nil && info.Status == "external" {
				totalCount++
			}
		}

		if totalCount == 0 {
			fmt.Println("No external port allocations found.")
			return nil
		}

		fmt.Printf("Refreshing %d external allocation(s)...\n", totalCount)
		removedCount = store.RefreshExternalAllocations(port.IsPortFree)
		return nil
	})

	if err != nil {
		return err
	}

	if removedCount > 0 {
		fmt.Printf("Removed %d stale external allocation(s).\n", removedCount)
	} else if totalCount > 0 {
		fmt.Println("All external allocations are still active.")
	}

	return nil
}
