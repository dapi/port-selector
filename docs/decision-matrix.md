# Port Selector Decision Matrix

This document defines the exact behavior of `port-selector` for all possible states.

## Terminology

| Term | Definition |
|------|------------|
| **busy** | Port fails TCP bind: `net.Listen("tcp", ":PORT")` returns error |
| **free** | Port successfully binds via `net.Listen()` |
| **locked** | Allocation has `locked: true` in allocations.yaml |
| **unlocked** | Allocation has `locked: false` or field is absent |
| **current dir** | Result of `os.Getwd()` after `filepath.Clean()` |
| **name** | Allocation name from `--name` flag, default: `main` |
| **allocated** | Port has entry in allocations.yaml for any directory |

## No-Args Behavior (`port-selector`)

Priority order for `FindByDirectoryAndName(cwd, name)`:

| Priority | Condition | Action |
|----------|-----------|--------|
| 1 | Locked port exists, free | Return locked port |
| 2 | Locked port exists, busy | Return locked port (user's service) |
| 3 | Unlocked port exists, free | Return unlocked port |
| 4 | Unlocked port exists, busy | Skip, find another or create new |
| 5 | No allocation found | Create new allocation |

**Invariant:** If a locked port exists for directory+name, always return it regardless of busy/free status.

## Lock Command Behavior (`port-selector --lock PORT`)

### Decision Matrix

| Port State | Allocated To | Locked? | --force | Result |
|------------|--------------|---------|---------|--------|
| free | not allocated | - | no | ✅ Lock port |
| free | current dir | any | no | ✅ Lock port (update) |
| free | other dir | no | no | ✅ Lock port (reassign abandoned) |
| free | other dir | yes | no | ❌ Error: `port PORT is locked for 'NAME' in DIR` |
| free | other dir | yes | yes | ✅ Lock port (reassign with --force) |
| busy | not allocated | - | no | ❌ Error: `port PORT is in use` |
| busy | not allocated | - | yes | ✅ Lock port (user takes responsibility) |
| busy | current dir | any | no | ✅ Lock port (user's service) |
| busy | other dir | any | no | ❌ Error: `port PORT is in use by DIR; stop the service first` |
| busy | other dir | any | yes | ❌ Error: same (cannot reassign busy port) |

### Key Rules

1. **Busy port on other directory is NEVER reassignable** - even with `--force`
2. **Free unlocked port on other directory IS reassignable** without `--force` (abandoned)
3. **Free locked port on other directory requires** `--force`
4. **Busy port not in allocations** requires `--force` to lock

### Side Effects on Lock

When locking a new port for directory+name:
- If old locked port exists for same directory+name → set `locked: false` on old port
- Old allocation is preserved (not deleted), only unlocked

## Error Message Formats

```
# Locked port on other directory
port 3001 is locked for 'main' in /path/to/other/dir

# Busy port on other directory
port 3001 is in use by /path/to/other/dir; stop the service first

# Busy port not in allocations
port 3001 is in use

# Process info (best-effort, may be unavailable)
port 3001 is in use by ruby (pid 12345)
```

## Invariants

1. **At most one locked port per directory+name** - enforced by unlocking old on new lock
2. **Locked port always returned** - `FindByDirectoryAndName` prioritizes locked over unlocked
3. **Busy status doesn't prevent return** - if it's user's locked port, return it
4. **Cross-directory busy port is sacred** - cannot be reassigned, it's someone's running service

---
*Last updated: 2026-02-02*
