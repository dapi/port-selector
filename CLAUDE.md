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
│   ├── config/config.go         # Read/create YAML config
│   ├── cache/cache.go           # Last-used file handling
│   ├── history/history.go       # Issued ports history (freeze period)
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
   ```
5. **Cache** of last port in `~/.config/port-selector/last-used`
6. **History** of issued ports in `~/.config/port-selector/issued-ports.yaml`
7. **Wrap-around** — after reaching portEnd, start from portStart
8. **Error** to STDERR with exit code 1 if all ports are busy

### Non-functional

- Minimal dependencies (only Go stdlib + yaml parser)
- Fast startup (< 100ms)
- Atomic cache writes (to prevent race conditions)

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

## Important Details

1. **STDOUT for port only** — no additional text
2. **Atomic cache** — write to temp file, then rename
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

## Documentation

**IMPORTANT:** Documentation must be maintained in both languages:
- `README.md` — English (primary)
- `README.ru.md` — Russian

When updating documentation, always update both files to keep them in sync.
