# CLAUDE.md - Instructions for AI Agents

## About the Project

**port-selector** — CLI utility in Go for automatic free port selection from a given range. Designed for AI-driven development with multiple parallel agents. Supports named allocations, port locking, Docker container detection, and process discovery.

## Tech Stack

- **Language:** Go 1.21+
- **Version management:** mise (.mise.toml)
- **CI/CD:** GitHub Actions
- **Releases:** goreleaser or manual build via workflow

## Project Structure

```
port-selector/
├── cmd/port-selector/main.go    # Entry point, argument parsing, CLI commands
├── internal/
│   ├── allocations/             # Port allocations with flock-based locking
│   │   ├── allocations.go       # Store, Load, Save, WithStore, CRUD operations
│   │   ├── lock_unix.go         # Unix flock implementation
│   │   └── lock_windows.go      # Windows stub (no locking)
│   ├── config/config.go         # Read/create YAML config, duration parsing
│   ├── debug/debug.go           # Debug logging (--verbose flag)
│   ├── docker/docker.go         # Docker container detection and project directory resolution
│   ├── logger/logger.go         # Structured logging for state changes
│   ├── pathutil/pathutil.go     # Path utilities (~ shortening)
│   └── port/
│       ├── checker.go           # Port availability checking, free port search
│       └── procinfo.go          # Process discovery via /proc (Linux only)
├── .github/workflows/
│   ├── ci.yml                   # Tests and linting on PRs
│   └── release.yml              # Build and release on tags
├── .mise.toml
├── go.mod
└── go.sum
```

## Key Requirements

### Functional

#### Port Allocation (primary command)
1. **No arguments** → outputs free port to STDOUT for current directory
2. **`--name NAME`** → allocate a named port (default: "main"), allowing multiple ports per directory
3. Port is **stable per (directory, name)** — same directory+name always returns the same port
4. **Wrap-around** — after reaching portEnd, start from portStart
5. **Error** to STDERR with exit code 1 if all ports are busy or frozen

#### Information Commands
6. **`-h, --help`** → help message
7. **`-v, --version`** → version (embedded at build via `-ldflags`)
8. **`-l, --list`** → show all allocations in table format (PORT, DIRECTORY, NAME, SOURCE, STATUS columns)
9. **`--verbose`** → enable debug output to STDERR (combinable with any command)

#### Allocation Management
10. **`--forget`** → remove all allocations for current directory
11. **`--forget --name NAME`** → remove only the named allocation
12. **`--forget-all`** → remove all allocations globally
13. **`--scan`** → scan port range, detect busy ports, identify owning processes/containers
14. **`--refresh`** → remove stale external allocations (ports no longer in use)

#### Port Locking
15. **`-c, --lock [PORT]`** → lock port for current directory and name (prevent reuse by others)
16. **`-u, --unlock [PORT]`** → unlock port
17. **`--force, -f`** → force lock a busy port or reassign a locked port from another directory

#### Lock Decision Matrix (for `--lock PORT`)

| Port State | Allocated To | Action | `--force` needed? |
|------------|-------------|--------|-------------------|
| Free | Current dir | Update lock status | No |
| Free | Other dir (unlocked) | Reassign to current dir | No |
| Free | Other dir (locked) | Reassign to current dir | **Yes** |
| Busy | Current dir | Lock (same dir owns it) | No |
| Busy | Other dir | **Block** (even with --force) | N/A |
| Busy | Not allocated (known process, same CWD) | Register + lock | No |
| Busy | Not allocated (known process, other CWD) | Register as external | No |
| Busy | Not allocated (unknown process) | Error | Yes (user takes responsibility) |
| Free | Not allocated | Allocate + lock | No |

### Non-functional

- **Go module dependencies:** only `gopkg.in/yaml.v3`
- **Optional runtime dependency:** Docker CLI (for container detection in `--scan`)
- Fast startup (< 100ms for port allocation, `--scan` may be slower)
- Flock-based file locking (to prevent race conditions on Unix)
- Platform support: Linux (full), macOS (port allocation works, process discovery limited), Windows (builds but no file locking)

## Config

**Location:** `~/.config/port-selector/config.yaml`

```yaml
portStart: 3000
portEnd: 4000
freezePeriod: 24h
# allocationTTL: 30d
log: ~/.config/port-selector/port-selector.log
```

| Field | Default | Description |
|-------|---------|-------------|
| `portStart` | 3000 | Start of port range (1-65535) |
| `portEnd` | 4000 | End of port range (must be > portStart) |
| `freezePeriod` | 24h | Time to avoid reusing recently allocated ports (supports d/h/m/s) |
| `allocationTTL` | disabled | Auto-expire allocations after this duration (e.g., 30d, 720h) |
| `log` | ~/.config/port-selector/port-selector.log | Path to log file (empty to disable) |

**Duration format:** supports `30d` (days), `720h` (hours), `30m` (minutes), standard Go duration.

**freezePeriod vs allocationTTL:**
- `freezePeriod` — prevents reusing a port within this window (port stays "frozen" for other directories)
- `allocationTTL` — deletes the allocation record entirely after this period of inactivity
- Locked allocations are **never** expired by TTL

**Legacy:** `freezePeriodMinutes` (integer) is supported for backward compatibility; `freezePeriod` takes precedence.

## Allocations

**Location:** `~/.config/port-selector/allocations.yaml`

```yaml
last_issued_port: 3005
allocations:
  3000:
    directory: /home/user/project-a
    name: main
    assigned_at: 2024-01-15T10:30:00Z
    last_used_at: 2024-01-15T12:00:00Z
    locked: false
    process_name: node
  3001:
    directory: /home/user/project-a
    name: api
    assigned_at: 2024-01-15T11:00:00Z
    last_used_at: 2024-01-15T12:00:00Z
    locked: true
    locked_at: 2024-01-15T11:00:00Z
  3002:
    directory: /home/user/project-b
    name: main
    assigned_at: 2024-01-16T09:00:00Z
    last_used_at: 2024-01-16T09:00:00Z
    status: external
    external_pid: 1234
    external_user: root
    external_process_name: docker-proxy
    container_id: abc123def456
```

### AllocationInfo Fields

| Field | Type | Description |
|-------|------|-------------|
| `directory` | string | Working directory of the project |
| `name` | string | Allocation name (default: "main") |
| `assigned_at` | time | When the allocation was created |
| `last_used_at` | time | Last time port was issued or updated |
| `locked` | bool | Whether port is locked (prevent reuse) |
| `locked_at` | time | When the port was locked |
| `process_name` | string | Name of the process using the port |
| `container_id` | string | Docker container ID (if applicable) |
| `status` | string | "" (normal) or "external" |
| `external_pid` | int | PID of external process |
| `external_user` | string | Username of external process owner |
| `external_process_name` | string | Name of external process |

**Primary key:** port number (unique in the map).
**Lookup keys:** (directory), (directory, name), (port).
**Invariant:** at most one locked port per (directory, name) combination.

## Named Allocations

One directory can have multiple port allocations with different names:

```bash
$ cd ~/project
$ port-selector --name web    # → 3000
$ port-selector --name api    # → 3001
$ port-selector --name db     # → 3002
$ port-selector --name web    # → 3000 (stable)
```

- Default name is `"main"` (empty name normalizes to "main")
- `--forget` without `--name` removes **all** allocations for the directory
- `--forget --name api` removes only the "api" allocation
- Freeze period applies per-port, not per-name

## External Allocations

Ports used by processes not managed by port-selector:

- **Created by:** `--scan` (discovers busy ports) or `--lock PORT` (when port is busy on another directory)
- **Status:** `status: external` in allocations.yaml
- **Stored info:** PID, username, process name, container ID
- **Cleanup:** `--refresh` removes stale external allocations (ports now free)
- **Display:** shown as `SOURCE=external` in `--list`

## Docker Integration

When a port is owned by `docker-proxy`, port-selector resolves the actual project directory:

1. Find container by port: `docker ps --filter publish=PORT`
2. Try compose label: `com.docker.compose.project.working_dir`
3. Fallback to first bind mount source

**Requires:** Docker CLI (`docker` command). Gracefully degrades if unavailable.

## Process Discovery

On Linux, `--scan` and `--lock` identify port owners via:

1. Parse `/proc/net/tcp` and `/proc/net/tcp6` for listening sockets
2. Match socket inode to process via `/proc/*/fd/`
3. Read process name from `/proc/[pid]/comm`, CWD from `/proc/[pid]/cwd`
4. Resolve UID to username

**Platform limitations:**
- **Linux:** full support (may need sudo for other users' processes)
- **macOS:** process discovery not available (`/proc` doesn't exist)
- **Windows:** process discovery not available

## Development Commands

```bash
# Run tests
go test ./... -v

# Build
go build -o port-selector ./cmd/port-selector

# Build with version
go build -ldflags "-X main.version=1.0.0" -o port-selector ./cmd/port-selector

# Lint check
golangci-lint run

# Format
go fmt ./...
```

## Code Patterns

### Port Checking

```go
func IsPortFree(port int) bool {
    ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
    if err != nil {
        return false
    }
    ln.Close()
    return true
}
```

**Known limitation (TOCTOU):** Port is checked as free, then released. Another process could take it between check and actual use. Callers should handle bind failures gracefully.

### Config Handling

- Use `os.UserConfigDir()` for cross-platform compatibility
- Create directories with `os.MkdirAll(..., 0755)`
- Use `gopkg.in/yaml.v3` for YAML

### Atomic Operations

- `WithStore(configDir, fn)` — read-modify-write with flock (use for all mutations)
- `Load(configDir)` — read-only without lock (use for `--list`)
- `Save(configDir, store)` — atomic write via temp file + rename (no lock)

### Error Handling

```go
// Output errors to STDERR
fmt.Fprintln(os.Stderr, "error: all ports in range are busy")
os.Exit(1)

// Successful port output to STDOUT (port only!)
fmt.Println(port)

// Informational output (--list, --forget, --lock, etc.) to STDOUT
fmt.Printf("Locked port %d for '%s' in %s\n", port, name, dir)
```

**STDOUT rules:**
- Port allocation (no args, `--name`) → port number only
- Other commands (`--list`, `--forget`, `--lock`, etc.) → informational messages

## Testing

### Unit Tests

- Test each package separately
- Use table-driven tests
- Mock file system via interfaces

### Integration Tests

```go
func TestFindFreePort(t *testing.T) {
    // Occupy port
    ln, _ := net.Listen("tcp", ":3000")
    defer ln.Close()

    // Check that the next one is returned
    port := FindFreePort(3000, 3010, 3000)
    if port == 3000 {
        t.Error("should skip busy port")
    }
}
```

### Platform-specific Tests

- Process discovery tests (`procinfo_test.go`) skip on non-Linux with `runtime.GOOS`
- Docker tests gracefully handle missing Docker CLI
- Concurrent tests verify flock correctness with multiple goroutines

## GitHub Actions

### Release Workflow

On `v*` tag creation, must:
1. Build binaries for linux/darwin (amd64/arm64)
2. Embed version from tag
3. Upload artifacts to release

```yaml
# ldflags example
-ldflags "-X main.version=${{ github.ref_name }}"
```

## Logging

When implementing functions that **modify state** (allocations, config, etc.), you **MUST** log these changes using the `logger` package if logging is enabled.

### Event Types

```go
import "github.com/dapi/port-selector/internal/logger"

// Available event types:
logger.AllocAdd       // When a new port allocation is created
logger.AllocUpdate    // When allocation's LastUsedAt is updated
logger.AllocLock      // When allocation lock status changes
logger.AllocDelete    // When a single allocation is removed
logger.AllocDeleteAll // When all allocations are cleared
logger.AllocExpire    // When allocation expires due to TTL
logger.AllocExternal  // When registering an external port (from --scan or --lock)
logger.AllocRefresh   // When refreshing external allocations (--refresh)
```

### Usage Pattern

```go
// Log state changes with relevant fields
logger.Log(logger.AllocAdd,
    logger.Field("port", port),
    logger.Field("dir", directory),
    logger.Field("name", name))

// Field() auto-quotes values with spaces
logger.Field("dir", "/path with spaces")  // -> dir="/path with spaces"
```

### Requirements for New Code

1. **Any function that modifies allocations** must call `logger.Log()` with appropriate event type
2. **Include relevant context** using `logger.Field()` (port, directory, name, count, reason, etc.)
3. **Logger is safe when nil** — if logging is disabled, calls are no-op
4. **No need to check if logger is enabled** — just call `logger.Log()`

## Important Details

1. **STDOUT for port only** (in port allocation mode) — no additional text. Other commands output informational messages.
2. **File locking** — flock-based locking on Unix for concurrent access safety. `WithStore` uses blocking `LOCK_EX`. Lock auto-releases on process exit.
3. **Graceful handling** — if no permissions for config, continue with defaults with warning to STDERR
4. **Don't block port** — only check and immediately close listener
5. **Directory-based persistence** — port is allocated per working directory. Same directory always returns the same port.
6. **Name normalization** — empty name normalizes to "main" for backward compatibility

## Usage Example

```bash
# Basic: get a port for current directory
$ port-selector
3000

# Named allocations for multiple services
$ port-selector --name web
3001
$ port-selector --name api
3002

# Lock a port (prevent reuse)
$ port-selector --lock
Locked port 3000 for 'main' in ~/project

# List all allocations
$ port-selector --list
PORT  DIRECTORY     NAME  SOURCE  STATUS
3000  ~/project-a   main  free    locked
3001  ~/project-a   api   free
3002  ~/project-b   main  free

# Scan for busy ports in range
$ port-selector --scan

# Usage in script
$ npm run dev -- --port $(port-selector)
```

## Pre-commit Checklist

- [ ] Tests pass: `go test ./...`
- [ ] Code formatted: `go fmt ./...`
- [ ] No linter errors: `golangci-lint run`
- [ ] Binary builds: `go build ./cmd/port-selector`
- [ ] README is up to date
- [ ] CHANGELOG.md is up to date (for new features/fixes)

## Release Checklist

When creating a new release:

1. **Update CHANGELOG.md:**
   - Move items from `[Unreleased]` to new version section
   - Add release date in format `YYYY-MM-DD`
   - Add comparison link at the bottom
   - Keep `[Unreleased]` section (empty or with upcoming changes)

2. **Version format:** `[X.Y.Z] - YYYY-MM-DD`

3. **Example:**
   ```markdown
   ## [Unreleased]

   ## [0.2.0] - 2026-01-15

   ### Added
   - New feature X

   [Unreleased]: https://github.com/dapi/port-selector/compare/v0.2.0...HEAD
   [0.2.0]: https://github.com/dapi/port-selector/compare/v0.1.0...v0.2.0
   ```

4. **Create git tag:** `git tag v0.2.0 && git push origin v0.2.0`

## Documentation

**IMPORTANT:** Documentation must be maintained in both languages:
- `README.md` — English (primary)
- `README.ru.md` — Russian

When updating documentation, always update both files to keep them in sync.

# Requirements Management

- **spreadsheet_id:** 1sjbiauud5OHe1h5tI5evv9B3qjBazxvetXc5P640kOs
- **spreadsheet_url:** https://docs.google.com/spreadsheets/d/1sjbiauud5OHe1h5tI5evv9B3qjBazxvetXc5P640kOs
