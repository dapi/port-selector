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

### Homebrew

```bash
brew tap dapi/tap
brew install port-selector
```

Update:

```bash
brew upgrade port-selector
```

### One-liner (for your main branch **master**)

```bash
curl -fsSL https://raw.githubusercontent.com/dapi/port-selector/master/install.sh | sh
```

#### Common variants

To /usr/local/bin:

```bash
curl -fsSL https://raw.githubusercontent.com/dapi/port-selector/master/install.sh | INSTALL_DIR=/usr/local/bin sh
```

Pin version:

```bash
curl -fsSL https://raw.githubusercontent.com/dapi/port-selector/master/install.sh | VERSION=v0.8.0 sh
```

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

### Named Allocations

A single directory can have multiple named allocations for different services (web, api, database, etc.):

```bash
# Allocate ports for different services in the same directory
$ port-selector --name web
3010

$ port-selector --name api  
3011

$ port-selector --name db
3012

# List shows NAME column
$ port-selector --list
PORT  DIRECTORY         NAME   STATUS  LOCKED  USER  PID  PROCESS  ASSIGNED
3010  ~/myproject       web    free    -       -     -    -        2026-01-06 20:00
3011  ~/myproject       api    free    -       -     -    -        2026-01-06 20:01
3012  ~/myproject       db     free    -       -     -    -        2026-01-06 20:02
```

The default name is `main`, which is used when `--name` is not specified:

```bash
$ port-selector                    # Uses name "main"
$ port-selector --name main        # Same as above
```

Named allocations are useful for:
- Microservices in monorepo that need different ports
- Running multiple services from the same directory
- Separating web, API, and database ports for the same project

### Managing Allocations

```bash
# List all port allocations
port-selector --list

# Output:
PORT  DIRECTORY                 NAME  STATUS  LOCKED  USER  PID  PROCESS  ASSIGNED
3000  ~/code/merchantly/main    main  free    yes     -     -    -        2026-01-03 20:53
3001  ~/code/valera             main  free    yes     -     -    -        2026-01-03 21:08
3010  ~/myproject               web   free    -       -     -    -        2026-01-06 20:00
3011  ~/myproject               api   free    -       -     -    -        2026-01-06 20:01
#
# Tip: Run with sudo for full process info: sudo port-selector --list

# Clear all allocations for current directory
cd ~/projects/old-project
port-selector --forget
# Cleared 2 allocation(s) for /home/user/projects/old-project (most recent was port 3005)

# Clear specific named allocation
port-selector --forget --name web
# Cleared allocation 'web' for /home/user/projects/old-project (was port 3010)

# Clear all allocations
port-selector --forget-all
# Cleared 5 allocation(s)
```

### Port Locking

Lock a port to prevent it from being allocated to other directories. Useful for long-running services that should keep their port even when restarted:

```bash
# Lock port for current directory (uses "main" name)
cd ~/projects/my-service
port-selector --lock
# Locked port 3000 for 'main'

# Lock named allocation
port-selector --lock --name web
# Locked port 3010 for 'web'

# Lock a specific port (allocates AND locks in one step)
cd ~/projects/new-service
port-selector --lock 3005
# Locked port 3005 for 'main'

# Unlock port for current directory
port-selector --unlock
# Unlocked port 3000 for 'main'

# Unlock named allocation
port-selector --unlock --name web
# Unlocked port 3010 for 'web'

# Unlock a specific port
port-selector --unlock 3005
# Unlocked port 3005
```

When using `--lock <PORT>` with a specific port number:
- If the port is not allocated, it will be allocated to the current directory AND locked
- This is useful when you want a specific port for a new project
- The port must be within the configured range

Smart `--force` behavior when the port belongs to another directory:
- **Free + unlocked**: reassigned without `--force` (abandoned allocation)
- **Free + locked**: requires `--force` to reassign
- **Busy (any)**: blocked completely — stop the service first

When locking an unallocated busy port:
- Requires `--force` (you take responsibility for the conflict)

```bash
# Port locked by another directory - requires --force:
port-selector --lock 3006
# error: port 3006 is locked by ~/code/other-project
#        use --lock 3006 --force to reassign it to current directory

# Force reassign locked port:
port-selector --lock 3006 --force
# warning: port 3006 was allocated to ~/code/other-project
# Reassigned and locked port 3006 for 'main' in ~/current-project

# Port busy on another directory - cannot reassign:
port-selector --lock 3006 --force
# error: port 3006 is in use by ~/code/other-project; stop the service first
```

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
  -h, --help           Show help message
  -v, --version        Show version
  -l, --list           List all port allocations
  -c, --lock [PORT]    Lock port for current directory and name (or specified port)
  -u, --unlock [PORT]  Unlock port for current directory and name (or specified port)
  --force, -f          Force lock a busy port or locked port from another directory
  --forget             Clear all port allocations for current directory
  --forget --name NAME Clear port allocation for current directory with specific name
  --forget-all         Clear all port allocations
  --scan               Scan port range and record busy ports with their directories
  --name NAME          Use named allocation (default: "main")
  --verbose            Enable debug output (can be combined with other flags)
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

# Freeze period after port issuance
# Port won't be reused within this time
# Supports: 24h (hours), 30m (minutes), 1d (days)
# "0" = disabled, default: 24h
freezePeriod: 24h

# Auto-expire allocations after this period
# Supports: 30d (days), 720h (hours), 24h30m (combined)
# "0" = disabled (default)
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

Port freeze information is stored in `~/.config/port-selector/allocations.yaml` as part of the allocation timestamps.

### Caching

For optimization, the utility remembers the last issued port in `~/.config/port-selector/allocations.yaml` (field `last_issued_port`). On the next call, checking starts from this port, not from the beginning of the range.

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

### Allocations File Format

Port allocations are stored in `~/.config/port-selector/allocations.yaml`:

```yaml
last_issued_port: 3012
allocations:
  3000:
    directory: /home/user/code/project-a
    name: main
    assigned_at: 2026-01-06T20:00:00Z
    last_used_at: 2026-01-06T20:00:00Z
    locked: true
  3010:
    directory: /home/user/myproject
    name: web
    assigned_at: 2026-01-06T20:00:00Z
    last_used_at: 2026-01-06T20:30:00Z
  3011:
    directory: /home/user/myproject
    name: api
    assigned_at: 2026-01-06T20:01:00Z
    last_used_at: 2026-01-06T20:35:00Z
  3012:
    directory: /home/user/myproject
    name: db
    assigned_at: 2026-01-06T20:02:00Z
    last_used_at: 2026-01-06T21:15:00Z
```

The `name` field is optional. Missing or empty names are treated as `"main"` for backward compatibility.

## Project Structure

```
port-selector/
├── cmd/
│   └── port-selector/
│       └── main.go          # Entry point
├── internal/
│   ├── allocations/
│   │   └── allocations.go   # Port allocation persistence
│   ├── config/
│   │   └── config.go        # Configuration handling
│   ├── docker/
│   │   └── docker.go        # Docker container detection
│   ├── logger/
│   │   └── logger.go        # Logging
│   ├── pathutil/
│   │   └── pathutil.go      # Path utilities
│   └── port/
│       ├── checker.go       # Port checking
│       └── procinfo.go      # Process info
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
