# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.7.0] - 2026-01-04

### Added
- Logging configuration option (`log` field in config.yaml) (#47)
- `--scan` now records all ports for directories with multiple ports (#48, #50)
- Docker container ID storage and display during `--scan` (#49)

### Fixed
- Deterministic port selection for multi-port directories (#52, #54)

## [0.6.0] - 2026-01-03

### Changed
- **Breaking:** Merged `last-used`, `issued-ports.yaml`, and `allocations.yaml` into single `allocations.yaml` file (#43)
  - Port is now used as map key (guarantees uniqueness, closes #42)
  - Added `last_issued_port` field to track last issued port
  - Old config files must be deleted manually before upgrade
- Implemented flock-based file locking on Unix to prevent race conditions (#43)
- Removed `internal/cache` and `internal/history` packages (consolidated into allocations)

### Added
- Windows users now see a one-time warning about missing file locking
- Unlock errors are now logged to stderr (not just debug mode)

## [0.5.0] - 2026-01-03

### Added
- `--verbose` flag for debug output (#35)
  - Shows detailed logging from all modules: main, config, allocations, port, docker
  - Debug output goes to stderr, doesn't interfere with port output
  - Can be combined with other flags: `--scan --verbose`, `--list --verbose`
- Shorten home directory paths in output (`/home/user/...` → `~/...`) (#38)
- USER column in `--list` output showing socket owner username (#32)
- Socket owner username display in `--scan` when PID is unavailable (#32)
- Sudo recommendation when ports with incomplete process info are detected (#32)
- Docker fallback for root-owned ports: tries Docker detection even without PID (#32)
- `--list` now uses Docker fallback to show live container info and project directory (#32)
- `process_name` field in allocations.yaml to persist discovered process names (#32)
- Docker container project directory detection (#29)
  - When port is used by `docker-proxy`, resolves actual project directory
  - Uses `com.docker.compose.project.working_dir` label (docker-compose)
  - Falls back to bind mount source directory

### Changed
- `--scan` now records ALL busy ports, including those without process info (#27)
  - Ports owned by root processes (e.g., docker-proxy) are recorded with `(unknown:PORT)` marker
  - Previously these ports were skipped with "not recorded" message

## [0.4.0] - 2026-01-03

### Added
- `--scan` flag to discover existing port allocations (#25)
- PID and process name columns in `--list` output for busy ports (#24)

### Fixed
- Show process info when port is in use (#22)

## [0.3.0] - 2026-01-03

### Added
- `--lock <PORT>` now allocates AND locks a free port in one step (#18)

## [0.2.0] - 2026-01-03

### Added
- Port locking with `--lock` and `--unlock` flags (#17)
- Allocation cleanup with `--forget`, `--forget-all`, `--forget-expired` flags (#16)
- TTL-based expiration for port allocations (#16)
- Directory-based port persistence and `--list` command (#15)
- Author info to README and `--help` output
- `make install` target
- Parallel AI Agents badge
- CHANGELOG.md

### Changed
- Documentation translated to English as primary language (#14)

## [0.1.0] - 2026-01-02

### Added
- Initial release
- Automatic free port selection from configurable range
- YAML configuration file support (`~/.config/port-selector/default.yaml`)
- Freeze period for issued ports to prevent reuse
- Wrap-around when reaching end of port range
- `-h, --help` and `-v, --version` flags
- CI/CD with GitHub Actions
- Cross-platform support (Linux, macOS)

[Unreleased]: https://github.com/dapi/port-selector/compare/v0.7.0...HEAD
[0.7.0]: https://github.com/dapi/port-selector/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/dapi/port-selector/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/dapi/port-selector/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/dapi/port-selector/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/dapi/port-selector/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/dapi/port-selector/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/dapi/port-selector/releases/tag/v0.1.0
