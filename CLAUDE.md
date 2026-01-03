# CLAUDE.md - Instructions for AI Agents

## About the Project

**port-selector** — CLI utility in Go for automatic free port selection from a given range. Designed for AI-driven development with multiple parallel agents.

## Tech Stack

- **Language:** Go 1.21+
- **Version management:** mise (.mise.toml)
- **CI/CD:** GitHub Actions
- **Releases:** goreleaser or manual build via workflow

## Project Structure

```
port-selector/
├── cmd/port-selector/main.go    # Entry point, argument parsing
├── internal/
│   ├── allocations/             # Port allocations with flock-based locking
│   │   ├── allocations.go       # Store, Load, Save, WithStore
│   │   ├── lock_unix.go         # Unix flock implementation
│   │   └── lock_windows.go      # Windows stub (no locking)
│   ├── config/config.go         # Read/create YAML config
│   ├── logger/logger.go         # Structured logging for state changes
│   └── port/checker.go          # Port availability checking
├── .github/workflows/release.yml
├── .mise.toml
├── go.mod
└── go.sum
```

## Key Requirements

### Functional

1. **No arguments** → outputs free port to STDOUT
2. **-h, --help** → help message
3. **-v, --version** → version (embedded at build via `-ldflags`)
4. **Config** in `~/.config/port-selector/default.yaml`:
   ```yaml
   portStart: 3000
   portEnd: 4000
   freezePeriodMinutes: 1440
   # log: ~/.config/port-selector/port-selector.log
   ```
5. **Allocations** in `~/.config/port-selector/allocations.yaml`:
   ```yaml
   last_issued_port: 3005
   allocations:
     3000:
       directory: /path/to/project
       assigned_at: 2024-01-15T10:30:00Z
       last_used_at: 2024-01-15T12:00:00Z
       locked: false
   ```
6. **Wrap-around** — after reaching portEnd, start from portStart
7. **Error** to STDERR with exit code 1 if all ports are busy

### Non-functional

- Minimal dependencies (only Go stdlib + yaml parser)
- Fast startup (< 100ms)
- Flock-based file locking (to prevent race conditions on Unix)
- Platform support: Linux, macOS (Windows builds but without file locking)

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

### Config Handling

- Use `os.UserConfigDir()` for cross-platform compatibility
- Create directories with `os.MkdirAll(..., 0755)`
- Use `gopkg.in/yaml.v3` for YAML

### Error Handling

```go
// Output errors to STDERR
fmt.Fprintln(os.Stderr, "error: all ports in range are busy")
os.Exit(1)

// Successful output to STDOUT (port only!)
fmt.Println(port)
```

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
```

### Usage Pattern

```go
// Log state changes with relevant fields
logger.Log(logger.AllocAdd,
    logger.Field("port", port),
    logger.Field("dir", directory))

// Field() auto-quotes values with spaces
logger.Field("dir", "/path with spaces")  // -> dir="/path with spaces"
```

### Requirements for New Code

1. **Any function that modifies allocations** must call `logger.Log()` with appropriate event type
2. **Include relevant context** using `logger.Field()` (port, directory, count, etc.)
3. **Logger is safe when nil** — if logging is disabled, calls are no-op
4. **No need to check if logger is enabled** — just call `logger.Log()`

## Important Details

1. **STDOUT for port only** — no additional text
2. **File locking** — flock-based locking on Unix for concurrent access safety
3. **Graceful handling** — if no permissions for config, continue with defaults
4. **Don't block port** — only check and immediately close listener

## Usage Example

```bash
# Agent runs
$ port-selector
3000

# Next agent
$ port-selector
3001

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
