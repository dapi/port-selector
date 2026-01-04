# port-selector

[![CI](https://github.com/dapi/port-selector/actions/workflows/ci.yml/badge.svg)](https://github.com/dapi/port-selector/actions/workflows/ci.yml)
[![Release](https://github.com/dapi/port-selector/actions/workflows/release.yml/badge.svg)](https://github.com/dapi/port-selector/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/dapi/port-selector)](https://goreportcard.com/report/github.com/dapi/port-selector)
[![Parallel AI Agents](https://img.shields.io/badge/Parallel_AI-Agents_Ready-00d4aa)](https://github.com/dapi/port-selector)

[🇷🇺 Русская версия](README.ru.md)

CLI utility for automatic free port selection from a configured range.

## Motivation

When developing with AI agents (Claude Code, Cursor, Copilot Workspace, etc.), you often have multiple parallel agents working on tasks in separate git worktrees. Each agent may need to start web servers for e2e testing, and they all need free ports.

**Problem:** When 5-10 agents simultaneously try to start dev servers on port 3000, conflicts occur.

**Solution:** `port-selector` automatically finds and returns the first free port from a configured range.

```
┌─────────────────────────────────────────────────────────────┐
│  Agent 1 (worktree: feature-auth)                           │
│  $ PORT=$(port-selector) && npm run dev -- --port $PORT     │
│  → Server running on http://localhost:3000                  │
├─────────────────────────────────────────────────────────────┤
│  Agent 2 (worktree: feature-dashboard)                      │
│  $ PORT=$(port-selector) && npm run dev -- --port $PORT     │
│  → Server running on http://localhost:3001                  │
├─────────────────────────────────────────────────────────────┤
│  Agent 3 (worktree: bugfix-login)                           │
│  $ PORT=$(port-selector) && npm run dev -- --port $PORT     │
│  → Server running on http://localhost:3002                  │
└─────────────────────────────────────────────────────────────┘
```

## Further Reading

The practice of running multiple AI agents in parallel using git worktrees is becoming increasingly popular. Each worktree provides complete file isolation, but all agents still share network resources — including ports. When agents run dev servers, e2e tests, or preview deployments, port conflicts become inevitable.

`port-selector` solves this by providing automatic port allocation with a freeze period, ensuring each agent gets a unique port even when multiple agents start simultaneously.

**Articles about parallel AI agent development:**

- [How we're shipping faster with Claude Code and Git Worktrees](https://incident.io/blog/shipping-faster-with-claude-code-and-git-worktrees) — incident.io's experience running multiple Claude Code sessions with custom worktree manager
- [Parallel AI Development with Git Worktrees](https://sgryt.com/posts/git-worktree-parallel-ai-development/) — the "three pillars": state isolation, parallel execution, asynchronous integration
- [How Git Worktrees Changed My AI Agent Workflow](https://nx.dev/blog/git-worktrees-ai-agents) — practical scenarios where agents work in background while you continue coding
- [Git Worktrees: The Secret Weapon for Running Multiple AI Agents](https://medium.com/@mabd.dev/git-worktrees-the-secret-weapon-for-running-multiple-ai-coding-agents-in-parallel-e9046451eb96) — why worktrees became essential in the AI-assisted development era
- [Parallel Coding Agents with Container Use and Git Worktree](https://www.youtube.com/watch?v=z1osqcNQRvw) — video walkthrough of three parallel agent workflows

## Installation

### From GitHub Releases

```bash
# Linux (amd64)
curl -L https://github.com/dapi/port-selector/releases/latest/download/port-selector-linux-amd64 -o port-selector
chmod +x port-selector
sudo mv port-selector /usr/local/bin/

# macOS (arm64 - Apple Silicon)
curl -L https://github.com/dapi/port-selector/releases/latest/download/port-selector-darwin-arm64 -o port-selector
chmod +x port-selector
sudo mv port-selector /usr/local/bin/

# macOS (amd64 - Intel)
curl -L https://github.com/dapi/port-selector/releases/latest/download/port-selector-darwin-amd64 -o port-selector
chmod +x port-selector
sudo mv port-selector /usr/local/bin/
```

### Build from Source

```bash
git clone https://github.com/dapi/port-selector.git
cd port-selector
make install
```

This will build the binary and install it to `/usr/local/bin/`.

## Usage

### Basic Usage

```bash
# Get a free port
port-selector
# Output: 3000

# Use in a script
PORT=$(port-selector)
npm run dev -- --port $PORT

# Or in one line
npm run dev -- --port $(port-selector)
```

### Integration Examples

#### Next.js / Vite / any dev server

```bash
# package.json scripts
{
  "scripts": {
    "dev": "PORT=$(port-selector) next dev -p $PORT",
    "dev:vite": "vite --port $(port-selector)"
  }
}
```

#### Docker Compose

```bash
# In .env or at startup
export APP_PORT=$(port-selector)
docker-compose up
```

#### Playwright / e2e tests

```bash
# In playwright config
export BASE_URL="http://localhost:$(port-selector)"
npx playwright test
```

#### direnv (.envrc)

Perfect for git worktree projects — port is automatically assigned when entering the directory:

```bash
# .envrc
export PORT=$(port-selector)

# Now use $PORT in any project script
# npm run dev will automatically get its unique port
```

```bash
# Example workflow with git worktree
$ cd ~/projects/myapp-feature-auth
direnv: loading .envrc
direnv: export +PORT

$ echo $PORT
3000

$ cd ~/projects/myapp-feature-dashboard
direnv: loading .envrc
direnv: export +PORT

$ echo $PORT
3001
```

#### Claude Code / AI Agents

Add to your project's CLAUDE.md:

```markdown
## Running dev server

Always use port-selector before starting dev server:
\`\`\`bash
PORT=$(port-selector) npm run dev -- --port $PORT
\`\`\`
```

### Directory-based Port Persistence

Each directory automatically gets its own dedicated port. Running `port-selector` from the same directory always returns the same port:

```bash
$ cd ~/projects/project-a
$ port-selector
3000

$ cd ~/projects/project-b
$ port-selector
3001

$ cd ~/projects/project-a
$ port-selector
3000  # Same port as before!
```

This is especially useful with git worktrees — each worktree gets a stable port.

### Managing Allocations

```bash
# List all port allocations
port-selector --list

# Output:
# PORT  STATUS  LOCKED  USER  PID  PROCESS       DIRECTORY                                              ASSIGNED
# 3000  free    yes     -     -    -             ~/code/merchantly/main                                 2026-01-03 20:53
# 3001  free    yes     -     -    -             ~/code/valera                                          2026-01-03 21:08
# 3003  free            -     -    -             ~/code/masha/master                                    2026-01-03 23:15
# 3005  busy            root  -    docker-proxy  ~/code/worktrees/feature/103-manager-reply             2026-01-04 22:32
# 3014  busy            root  -    docker-proxy  ~/code/valera                                          2026-01-04 22:32
#
# Tip: Run with sudo for full process info: sudo port-selector --list

# Clear allocation for current directory
cd ~/projects/old-project
port-selector --forget
# Cleared allocation for /home/user/projects/old-project (was port 3005)

# Clear all allocations
port-selector --forget-all
# Cleared 5 allocation(s)
```

### Port Locking

Lock a port to prevent it from being allocated to other directories. Useful for long-running services that should keep their port even when restarted:

```bash
# Lock port for current directory
cd ~/projects/my-service
port-selector --lock
# Locked port 3000

# Lock a specific port (allocates AND locks in one step)
cd ~/projects/new-service
port-selector --lock 3005
# Locked port 3005

# Unlock port for current directory
port-selector --unlock
# Unlocked port 3000

# Unlock a specific port
port-selector --unlock 3005
# Unlocked port 3005
```

When using `--lock <PORT>` with a specific port number:
- If the port is not allocated, it will be allocated to the current directory AND locked
- This is useful when you want a specific port for a new project
- The port must be free and within the configured range

When a port is locked:
- It remains allocated to its directory
- Other directories cannot get this port during allocation
- The owning directory can still use the port normally

### Discovering Existing Ports

When first adopting `port-selector` in an environment where some ports are already in use, you can scan the range to discover and record them:

```bash
port-selector --scan
# Scanning ports 3000-3200...
# Port 3005: already allocated to ~/code/worktrees/feature/103-manager-reply
# Port 3014: already allocated to ~/code/valera
#
# No new ports to record.

# When discovering new ports:
# Scanning ports 3000-3200...
# Port 3000: used by node (pid=12345, cwd=~/projects/app-a)
# Port 3007: used by docker-proxy (pid=585980, cwd=~/projects/my-compose-app)
#
# Recorded 2 port(s) to allocations.
#
# Tip: Run with sudo for full process info: sudo port-selector --scan
```

This creates allocations for busy ports, so `port-selector` will skip them when allocating new ports.

**Note:** Ports owned by root processes (like `docker-proxy`) may not have accessible process info. These ports are still recorded with `(unknown:PORT)` directory marker to prevent allocation conflicts.

#### Running with sudo

To see full process information (PID, process name) for ports owned by other users, run with sudo. **Important:** use `-E` flag to preserve your environment, otherwise config will be created in `/root/.config/`:

```bash
# Wrong: creates separate config in /root/.config/port-selector/
sudo port-selector --scan

# Correct: uses your user's config
sudo -E port-selector --scan

# Alternative: explicitly pass HOME
sudo HOME=$HOME port-selector --scan
```

### Docker Container Detection

When a port is published by Docker, the host process is `docker-proxy` with a useless `cwd=/`. `port-selector` automatically resolves the actual project directory:

```bash
port-selector --scan
# Port 3007: used by docker-proxy (pid=585980, cwd=/home/user/my-project)
#                                                  ↑ resolved from container
```

The resolution uses:
1. `com.docker.compose.project.working_dir` label (docker-compose projects)
2. Bind mount source directory (fallback for plain `docker run`)

**Note:** Requires `docker` CLI to be available.

### Command Line Arguments

```
port-selector [options]

Options:
  -h, --help        Show help message
  -v, --version     Show version
  -l, --list        List all port allocations
  -c, --lock [PORT] Lock port for current directory (or specified port)
  -u, --unlock [PORT] Unlock port for current directory (or specified port)
  --forget          Clear port allocation for current directory
  --forget-all      Clear all port allocations
  --scan            Scan port range and record busy ports with their directories
  --verbose         Enable debug output (can be combined with other flags)
```

### Debug Output

Use `--verbose` to see detailed debug information about the port selection process:

```bash
port-selector --verbose
# [DEBUG] main: starting port selection
# [DEBUG] config: loading config from /home/user/.config/port-selector/config.yaml
# [DEBUG] config: loaded: portStart=3000, portEnd=4000, freezePeriod=1440, allocationTTL=30d
# [DEBUG] main: config loaded: portStart=3000, portEnd=4000, freezePeriod=1440 min
# [DEBUG] allocations: loading from /home/user/.config/port-selector/allocations.yaml
# [DEBUG] allocations: loaded 5 allocations
# [DEBUG] main: current directory: /home/user/projects/my-app
# [DEBUG] main: found existing allocation: port 3001
# [DEBUG] main: existing port 3001 is free, reusing
# 3001
```

The `--verbose` flag can be combined with other flags:

```bash
port-selector --scan --verbose
port-selector --list --verbose
```

## Configuration

On first run, a configuration file is created:

**~/.config/port-selector/config.yaml**

```yaml
# Start port of range
portStart: 3000

# End port of range
portEnd: 4000

# Freeze period after port issuance (in minutes)
# Port won't be reused within this time
# 0 = disabled, 1440 = 24 hours (default)
freezePeriodMinutes: 1440

# Auto-expire allocations after this period
# Supports: 30d (days), 720h (hours), 24h30m (combined)
# Empty or "0" = disabled (default)
allocationTTL: 30d

# Log file path for operation logging (optional)
# Uncomment to enable logging of all allocation changes
# log: ~/.config/port-selector/port-selector.log
```

### Logging

When `log` is set, all allocation changes are written to the specified file:

```yaml
log: ~/.config/port-selector/port-selector.log
```

Log format:
```
2026-01-03T15:04:05Z ALLOC_ADD port=3001 dir=/home/user/project1 process=node
2026-01-03T15:04:10Z ALLOC_LOCK port=3001 locked=true
2026-01-03T15:05:00Z ALLOC_DELETE port=3002 dir=/home/user/forgotten
```

Logged events:
- `ALLOC_ADD` — new port allocated
- `ALLOC_UPDATE` — allocation timestamp updated (reuse)
- `ALLOC_LOCK` — port locked/unlocked
- `ALLOC_DELETE` — allocation removed (--forget)
- `ALLOC_DELETE_ALL` — all allocations removed (--forget-all)
- `ALLOC_EXPIRE` — allocation expired by TTL

### Allocation TTL

When `allocationTTL` is set, allocations older than the specified period are automatically removed during each run. This prevents accumulation of stale allocations from deleted projects:

```yaml
allocationTTL: 30d  # Allocations expire after 30 days of inactivity
```

The timestamp is updated each time a port is returned for an existing allocation, so actively used allocations never expire.

### Freeze Period

After a port is issued, it becomes "frozen" for the specified time and won't be issued again. This solves the problem when an application starts slowly and the port appears free, even though another server is about to start on it.

```
Time 10:00 - Agent 1 requests port → gets 3000
Time 10:01 - Agent 2 requests port → gets 3001 (3000 is frozen)
Time 10:02 - Agent 1 stops, port 3000 is released
Time 10:03 - Agent 3 requests port → gets 3002 (3000 is still frozen)
...
Time 34:01 - 24 hours passed, port 3000 is unfrozen
```

Issued ports history is stored in `~/.config/port-selector/issued-ports.yaml` and automatically cleaned of expired records.

### Caching

For optimization, the utility remembers the last issued port in `~/.config/port-selector/last-used`. On the next call, checking starts from this port, not from the beginning of the range.

```
First call:   checks 3000 → free → returns 3000, saves 3000
Second call:  checks 3001 → free → returns 3001, saves 3001
Third call:   checks 3002 → busy → checks 3003 → free → returns 3003
...
After 4000:   checks 3000 (wrap-around)
```

## Algorithm

```
┌────────────────────────────────────────┐
│          port-selector                 │
└──────────────────┬─────────────────────┘
                   │
                   ▼
┌────────────────────────────────────────┐
│  1. Read config                        │
│     ~/.config/port-selector/config.yaml │
│     (create if missing)                │
└──────────────────┬─────────────────────┘
                   │
                   ▼
┌────────────────────────────────────────┐
│  2. Read last-used and history         │
│     last-used → starting point         │
│     issued-ports.yaml → frozen ports   │
└──────────────────┬─────────────────────┘
                   │
                   ▼
┌────────────────────────────────────────┐
│  3. Check port:                        │
│     - Not frozen?                      │
│     - Not locked by another dir?       │
│     - Free? (net.Listen)               │
└──────────────────┬─────────────────────┘
                   │
           ┌───────┴───────┐
           │               │
       suitable      frozen/busy
           │               │
           ▼               ▼
┌──────────────────┐ ┌──────────────────┐
│ 4a. Save:        │ │ 4b. Next port    │
│  - last-used     │ │     (wrap-around │
│  - to history    │ │     after end)   │
│  Output STDOUT   │ │                  │
└──────────────────┘ └────────┬─────────┘
                              │
                    ┌─────────┴─────────┐
                    │                   │
              more ports        all checked
                    │                   │
                    ▼                   ▼
              → step 3         ┌────────────────┐
                               │ ERROR to STDERR│
                               │ exit code 1    │
                               └────────────────┘
```

## Development

### Requirements

- Go 1.21+
- mise (for version management)

### Local Build

```bash
# Install dependencies via mise
mise install

# Run tests
make test

# Build
make build

# Build and install to /usr/local/bin
make install

# Uninstall
make uninstall
```

### Project Structure

```
port-selector/
├── cmd/
│   └── port-selector/
│       └── main.go          # Entry point
├── internal/
│   ├── config/
│   │   └── config.go        # Configuration handling
│   ├── cache/
│   │   └── cache.go         # Last-used caching
│   ├── docker/
│   │   └── docker.go        # Docker container detection
│   ├── history/
│   │   └── history.go       # Issued ports history (freeze period)
│   └── port/
│       └── checker.go       # Port checking
├── .github/
│   └── workflows/
│       └── release.yml      # GitHub Actions for releases
├── .mise.toml               # mise configuration
├── go.mod
├── go.sum
├── CLAUDE.md                # Instructions for AI agents
└── README.md
```

## License

MIT

## Author

[Danil Pismenny](https://pismenny.ru) ([@dapi](https://github.com/dapi))
