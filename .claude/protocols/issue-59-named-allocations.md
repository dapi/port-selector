# Plan: Issue #59 — Named port allocations

## Goal
Add stable, per-directory named allocations so a single worktree can reserve multiple ports (web/api/db/etc.), while preserving current default behavior.

## Requirements (from issue + clarifications)
- `port-selector` without args behaves exactly as today.
- `--name <name>` enables a named allocation scoped to the current directory.
- Default name is `main` when `--name` is not provided.
- Explicit empty name is invalid (`--name=` or `--name ""` => error).
- Same directory can have multiple named allocations that remain stable across runs.
- Different directories have independent allocations even with the same name.
- `--forget` without `--name` removes ALL allocations for the current directory.
- `--forget --name <name>` removes only that named allocation for the current directory.
- `--lock/--unlock` with `--name` applies to the allocation for that name; with a port argument, it binds to that name.
- `--list` shows a NAME column:
  - Show empty for `main` when that directory has no other names.
  - Show `main` when the same directory has other named allocations.

## Data model changes
- Add `name` to allocation entries in `allocations.yaml`.
  - Missing/empty `name` is treated as `main` when reading legacy files.
- Allocation uniqueness remains by `port` key; the `name` is an additional attribute.
- Extend allocation lookup and mutation to be directory + name aware:
  - Find allocation for `(dir, name)`.
  - Update `LastUsedAt` for `(dir, name)`.
  - Remove allocation(s) by `(dir, name)` or by directory (all names).
  - Lock/unlock for `(dir, name)` or by port + name.
- When assigning a new port for a name:
  - Do NOT remove or override allocations belonging to other names in the same directory.
  - Only rotate/supersede old ports that match the same `(dir, name)`.
- Exclusions when selecting a free port:
  - Locked ports from other directories (existing behavior).
  - Ports allocated to the same directory under different names.

## CLI behavior changes
- Add `--name <name>` (also `--name=<name>`) to all relevant commands.
- `port-selector` (no args) => default name `main`.
- `port-selector --name main` => identical to default behavior.
- `port-selector --name postgres` => stable port for `(cwd, postgres)`.
- `--forget` semantics as in requirements above.
- `--lock/--unlock` semantics as in requirements above.
- `--list` adds NAME column with conditional display rules.
- Help text updates to document `--name` and semantics.

## Implementation steps
1) CLI parsing
   - Parse `--name` and `--name=<value>` across commands.
   - Validate non-empty when explicitly provided.
   - Thread `name` into run/lock/unlock/forget/list flows.

2) Allocations store updates
   - Add `Name` field to `AllocationInfo` and `Allocation`.
   - Normalize name on load (empty => `main`).
   - Add Store methods for `(dir, name)` operations and safe cleanup scoped to name.
   - Ensure logging includes `name` field for state changes.

3) Port selection logic
   - Use `(dir, name)` to check existing allocations.
   - Exclude ports for other names in the same dir from selection.
   - Update `LastUsedAt` on the selected port for the given name.

4) List output
   - Compute per-directory name presence to decide when to print `main` vs empty.
   - Add NAME column in list output.

5) Tests
   - Add/adjust allocations tests for:
     - Find/update/remove by `(dir, name)`.
     - Default `main` handling for legacy allocations.
     - Preventing accidental reuse of another name's port in the same directory.
   - Update CLI tests if any exist for new flag parsing.

6) Docs
   - Update `printHelp()` usage.
   - Update `README.md` and `README.ru.md` with `--name` usage and examples.
   - Update allocations YAML example to include `name`.

## Acceptance criteria
- Behavior from issue examples matches exactly.
- Legacy allocations without `name` continue to work as `main`.
- `--forget` and `--lock/--unlock` honor the name semantics above.
- `--list` NAME column display matches the specified rule.
