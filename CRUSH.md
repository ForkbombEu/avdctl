# CRUSH.md - avdctl Development Guide

## Project Overview

**avdctl** is a Linux-focused CLI tool for managing Android Virtual Device (AVD) lifecycle with a golden-image/clone workflow. It enables fast creation of disposable emulator instances backed by QCOW2 golden images, designed for CI environments and parallel execution.

- **Language**: Go 1.25.3
- **CLI Framework**: cobra (github.com/spf13/cobra)
- **License**: AGPL-3.0-only
- **Copyright**: Forkbomb B.V. 2025

## Architecture

```
cmd/avdctl/main.go          # CLI entry point, cobra command definitions
internal/avd/
  env.go                    # Environment detection & configuration
  ops.go                    # Core AVD operations (549 lines)
  ops_test.go               # Basic tests
config.ini.tpl              # AVD config template
```

### Key Concepts

1. **Base AVDs**: Clean Android system images created with `init-base`
2. **Golden Images**: Compressed QCOW2 snapshots of userdata (`save-golden`, `prewarm`)
3. **Clones**: Thin QCOW2 overlays backed by golden images (symlinked read-only files from base)
4. **Baking**: Pre-installing APKs into a golden image by booting, installing, and exporting

## Essential Commands

### Build & Test

```bash
# Build (using Taskfile)
task build                  # → bin/avdctl

# Build (using go directly)
go build -o bin/avdctl ./cmd/avdctl

# Test
task test                   # OR: go test ./...

# Run binary
task a -- <args>            # passes args to ./bin/avdctl
./bin/avdctl --help
```

### Common Workflows

```bash
# 1. Create base AVD
task init-base              # Uses vars from Taskfile
./bin/avdctl init-base --name base-a35 --image "system-images;android-35;google_apis;x86_64" --device pixel_6

# 2. Prewarm (boot once, settle, export golden)
task prewarm
./bin/avdctl prewarm --name base-a35 --dest ~/avd-golden/base-a35-prewarmed.qcow2

# 3. Clone from golden
task clone-acme             # → w-acme
./bin/avdctl clone --base base-a35 --name w-acme --golden ~/avd-golden/base-a35-prewarmed.qcow2

# 4. Run headless
task run-acme               # Port 5580
./bin/avdctl run --name w-acme --port 5580

# 5. List running emulators
task ps
./bin/avdctl ps             # Shows: name, serial, port, PID, boot status

# 6. Stop emulator
task stop NAME=w-acme
./bin/avdctl stop --name w-acme

# 7. Bake APKs into a new golden
./bin/avdctl bake-apk --base base-a35 --name w-acme-baked \
  --golden ~/avd-golden/base-a35-prewarmed.qcow2 \
  --apk /path/app1.apk --apk /path/app2.apk \
  --dest ~/avd-golden/w-acme-baked.qcow2

# 8. List AVDs
./bin/avdctl list
./bin/avdctl list --json

# 9. Delete AVD
./bin/avdctl delete w-acme

# Full from-scratch flow
task fresh                  # clean → init-base → prewarm → clone-acme → clone-gino
```

## Environment Variables

Set these in your shell or Taskfile (see `Taskfile.yml` for defaults):

| Variable | Default | Purpose |
|----------|---------|---------|
| `ANDROID_SDK_ROOT` | `/opt/android-sdk` | Android SDK path |
| `ANDROID_AVD_HOME` | `~/.android/avd` | AVD storage directory |
| `AVDCTL_GOLDEN_DIR` | `~/avd-golden` | Golden QCOW2 images |
| `AVDCTL_CLONES_DIR` | `~/avd-clones` | (Reserved for future use) |
| `AVDCTL_CONFIG_TEMPLATE` | (optional) | Path to custom `config.ini.tpl` |

**Detection logic**: `internal/avd/env.go:25-52`

## Code Organization

### internal/avd/env.go

- `Env` struct: Holds all paths (SDK, AVD home, tools)
- `Detect()`: Auto-detects environment from env vars + defaults
- Tool paths: `emulator`, `adb`, `avdmanager`, `sdkmanager`, `qemu-img`

### internal/avd/ops.go

**Key Functions** (alphabetically):

- `BakeAPK`: Clone → boot → install APKs → shutdown → return userdata path
- `CloneFromGolden`: Create thin QCOW2 overlay backed by golden, symlink base files
- `Delete`: Remove `.avd` directory and `.ini` file
- `FindFreeEvenPort`: Find available TCP port pair for emulator (uses both `port` and `port+1`)
- `GetAVDNameFromSerial`: Query emulator console for AVD name via `adb emu avd name`
- `GuessEmulatorSerial`: Parse `adb devices` for first `emulator-*` device
- `InitBase`: Create base AVD via `avdmanager`, auto-install system image if missing
- `KillEmulator`: Gracefully kill emulator via `adb emu kill`
- `List`: List AVDs under `ANDROID_AVD_HOME`
- `ListRunning`: Parse `adb devices`, query boot status + PID for each emulator
- `PrewarmGolden`: Boot AVD on fixed port, wait for boot, settle, kill, export golden
- `RunAVD`: Auto-pick free even port and start emulator headless
- `SaveGolden`: Compress userdata to QCOW2 via `qemu-img convert -O qcow2 -c`
- `StartEmulator`: Low-level emulator start (returns `*exec.Cmd`)
- `StartEmulatorOnPort`: Start emulator on specific port with logging
- `StopBySerial`: Kill emulator by serial (wrapper for `adb emu kill`)
- `WaitForBoot`: Poll `getprop sys.boot_completed` until `1` or timeout
- `sanitizeConfigINI`: Strip snapshot/quickboot settings, enforce `userdata.useQcow2=yes`
- `waitForEmulatorSerial`: Poll `adb devices` until specific serial appears

**Internal Helpers**:

- `run(bin, args...)`: Execute command, return error with combined output on failure
- `ensureADB`: Idempotent `adb start-server`
- `ensureSysImg`: Check if system image exists, install via `sdkmanager` if missing
- `findEmulatorPID`: Best-effort PID lookup via `/proc/[0-9]*/cmdline` (Linux-only)
- `infoOf`: Return `Info` struct for an AVD

### cmd/avdctl/main.go

**Commands** (see `--help` for full flags):

| Command | Purpose |
|---------|---------|
| `init-base` | Create base AVD (installs system image if needed) |
| `save-golden` | Export userdata to compressed QCOW2 |
| `prewarm` | Boot once, settle caches, export golden (no snapshots) |
| `clone` | Create symlinked clone backed by golden QCOW2 |
| `run` | Run AVD headless (supports `--port` for parallel instances) |
| `bake-apk` | Clone → boot → install APKs → export new golden |
| `list` | List AVDs (supports `--json`) |
| `ps` | List running emulators (supports `--json`) |
| `status` | Show status for running emulator by `--name` or `--serial` |
| `stop` | Stop emulator by `--name` or `--serial` |
| `delete` | Delete AVD and .ini file |

## Naming Conventions

### AVD Names

- **Base AVDs**: `base-a<API>` (e.g., `base-a35` for Android 35)
- **Customer Clones**: `w-<slug>` (e.g., `w-acme`, `w-gino`)
- **Baked Images**: `w-<slug>-baked` or descriptive suffix

### Code Style

- **Struct naming**: CamelCase (`ProcInfo`, `Env`)
- **Exported functions**: CamelCase (`InitBase`, `CloneFromGolden`)
- **Internal helpers**: camelCase (`run`, `ensureADB`, `infoOf`)
- **Error handling**: Always wrap errors with context (`fmt.Errorf("op failed: %w", err)`)
- **Go conventions**: Standard Go formatting (tabs for indentation, `gofmt` compatible)

### File Conventions

- **AVD directories**: `$ANDROID_AVD_HOME/<name>.avd/`
- **AVD metadata**: `$ANDROID_AVD_HOME/<name>.ini`
- **Userdata files**: 
  - QCOW2: `userdata-qemu.img.qcow2`
  - Raw: `userdata.img` (fallback for base AVDs)
- **Golden images**: `$AVDCTL_GOLDEN_DIR/<name>-prewarmed.qcow2` or `<name>-baked.qcow2`
- **Emulator logs**: `/tmp/emulator-<name>-<port>.log`

## Testing Approach

### Current Tests

- `internal/avd/ops_test.go`: Single smoke test for `Detect()` (line 5-10)

### Manual Testing

Use Taskfile workflows:

```bash
# Full integration test
task fresh

# Test parallel instances
task run-acme &   # port 5580
task run-gino &   # port 5582
task ps
task stop NAME=w-acme
task stop NAME=w-gino
```

### No Formal Test Suite

- **Integration testing**: Manual via Taskfile + real emulator
- **Unit tests**: Minimal (only `TestDetect`)
- **CI/CD**: No GitHub Actions or automated testing detected

## Important Gotchas

### 1. Emulator Ports Must Be Even

Emulators use a port pair: `<port>` (console) and `<port+1>` (ADB). Always specify/check even ports.

**Related code**: `internal/avd/ops.go:397-399`, `cmd/avdctl/main.go:165-169`

### 2. Snapshots/QuickBoot Are Explicitly Disabled

All operations force cold boot (`-no-snapshot-load`, `-no-snapshot-save`). Golden images rely on QCOW2 overlays, not emulator snapshots. Emulators also run with `-read-only` to allow multiple instances sharing base AVD files.

**Related code**:
- `internal/avd/ops.go:225-242` (`sanitizeConfigINI`)
- `internal/avd/ops.go:257-270` (`StartEmulator` args with `-read-only`)
- `internal/avd/ops.go:423-432` (`StartEmulatorOnPort` args with `-read-only`)

### 3. Config Sanitization

When cloning, `config.ini` is sanitized to remove snapshot/quickboot settings and enforce:
- `QuickBoot.mode=disabled`
- `snapshot.present=false`
- `fastboot.forceColdBoot=yes`
- `userdata.useQcow2=yes`

**Function**: `sanitizeConfigINI` (`internal/avd/ops.go:225-243`)

### 4. System Image Auto-Installation

`init-base` will automatically download/install the system image via `sdkmanager` if missing. May prompt for license acceptance.

**Function**: `ensureSysImg` (`internal/avd/ops.go:63-78`)

### 5. ADB Must Be Running

Many operations call `ensureADB` (idempotent `adb start-server`) before proceeding.

**Related code**: `internal/avd/ops.go:392`, usage in `PrewarmGolden`, `RunAVD`, `ListRunning`

### 6. PID Detection Is Linux-Only

`findEmulatorPID` scans `/proc/[0-9]*/cmdline` for the `-port` argument. Won't work on macOS/Windows.

**Function**: `internal/avd/ops.go:524-544`

### 7. Clone Uses QCOW2 Overlays (Instant, Copy-on-Write)

Clones are created instantly (~0.15s) using QCOW2 overlays backed by golden images. Symlinks are used for read-only base AVD files (system images, ROMs). Writable files (userdata, cache, encryptionkey, sdcard) get thin QCOW2 overlays (~196KB each) backed by golden raw IMG files. This provides true copy-on-write: clones start small and only grow as data diverges from the golden.

**Function**: `CloneFromGolden` (`internal/avd/ops.go:147-300`)
**Key changes (Nov 2025)**: Restored QCOW2 overlay approach (was temporarily changed to slow memory-copy in commit 612c238). QCOW2 overlays backed by raw golden images (`-F raw -b <golden>`), config sets `userdata.useQcow2=yes`, emulators run with `QEMU_FILE_LOCKING=off` for parallel instances.

**Why this matters**: Previous implementation copied 6GB+ files into memory during cloning, causing timeouts with Maestro and other automation tools. QCOW2 overlays are instant and allow multiple clones to share golden backing files safely.

### 8. Prewarm Uses Fixed Port 5580

`PrewarmGolden` hardcodes port 5580. If you run multiple prewarming operations concurrently, you'll get port conflicts.

**Related code**: `internal/avd/ops.go:320`

**Note**: Prewarm now restarts ADB server before starting to clear stale state. If boot detection times out but userdata file exists (>1MB), it will proceed with SaveGolden anyway, as the boot likely succeeded despite ADB connection issues.

### 9. Emulator Logs Are Saved to /tmp

When using `StartEmulatorOnPort`, logs go to `/tmp/emulator-<name>-<port>.log`. Check these for troubleshooting boot issues.

**Related code**: `internal/avd/ops.go:400-417`

### 10. QEMU File Locking Disabled for Parallel Instances

Emulators set `QEMU_FILE_LOCKING=off` environment variable to allow multiple instances sharing the same golden backing file without write lock conflicts.

**Related code**: `internal/avd/ops.go:444`, `internal/avd/ops.go:271`

## Docker Support

**dockerfile** is present (Go 1.25, Debian Bookworm base). No Compose or orchestration. Expects Android SDK tools to be bind-mounted or available in PATH at runtime.

**Build**:
```bash
docker build -t avdctl .
```

**Usage** (requires SDK + KVM access):
```bash
docker run --rm -v $ANDROID_SDK_ROOT:/opt/android-sdk -v ~/.android:/root/.android avdctl list
```

## Development Workflow

### Adding a New Command

1. Add command definition in `cmd/avdctl/main.go` (follow existing patterns)
2. Implement operation in `internal/avd/ops.go` if needed
3. Test manually via `task build && ./bin/avdctl <command>`
4. Update README.md examples if relevant

### Adding a New Operation

1. Define function in `internal/avd/ops.go`
2. Follow error wrapping conventions: `fmt.Errorf("context: %w", err)`
3. Use `Env` struct for all paths/tools
4. Return `Info` struct or `(string, int64, error)` for file operations
5. Add to command handler in `main.go`

### Modifying Config Template

Edit `config.ini.tpl` carefully. Key settings enforced by `sanitizeConfigINI`:
- `QuickBoot.mode=disabled`
- `snapshot.present=false`
- `fastboot.forceColdBoot=yes`
- `userdata.useQcow2=yes`

**Do not enable snapshots** or QCOW2 overlays will break.

## Dependencies

From `go.mod`:

```
github.com/forkbombeu/avdctl
  require github.com/spf13/cobra v1.10.1
  require (
    github.com/inconshreveable/mousetrap v1.1.0 // indirect
    github.com/spf13/pflag v1.0.9 // indirect
  )
```

### External Tools Required

Must be in `$PATH` or specified via `$ANDROID_SDK_ROOT`:

- `emulator` (Android Emulator)
- `adb` (Android Debug Bridge)
- `avdmanager` (AVD Manager CLI)
- `sdkmanager` (SDK Manager CLI)
- `qemu-img` (QEMU disk image utility)

Install via Android SDK command-line tools or package manager.

## Taskfile Variables

From `Taskfile.yml`:

```yaml
vars:
  BASE_NAME: base-a35
  SYS_IMAGE: "system-images;android-35;google_apis_playstore;x86_64"
  DEVICE: "pixel_6"
  GOLDEN: "{{.AVDCTL_GOLDEN_DIR}}/{{.BASE_NAME}}-prewarmed.qcow2"
  PORT1: "5580"
  PORT2: "5582"
```

Override via environment or `task <target> VAR=value`.

## Common Tasks Reference

| Task | Description |
|------|-------------|
| `build` | Build binary to `bin/avdctl` |
| `test` | Run `go test ./...` |
| `env` | Show environment variables |
| `a` | Run `./bin/avdctl {{.CLI_ARGS}}` |
| `mkdirs` | Create `$AVDCTL_GOLDEN_DIR` and `$AVDCTL_CLONES_DIR` |
| `init-base` | Create base AVD (deps: build, mkdirs) |
| `prewarm` | Prewarm base AVD (deps: build, mkdirs) |
| `clone-acme` | Clone base to `w-acme` (deps: build) |
| `clone-gino` | Clone base to `w-gino` (deps: build) |
| `run-acme` | Run `w-acme` on port 5580 (deps: build) |
| `run-gino` | Run `w-gino` on port 5582 (deps: build) |
| `ps` | List running emulators |
| `stop` | Stop emulator (default: `w-acme`, override with `NAME=...`) |
| `clean-avds` | Kill adb server, delete all AVDs in `$ANDROID_AVD_HOME` |
| `fresh` | Full from-scratch flow (clean → init → prewarm → clone) |

## Diagnostics

Current LSP hints (non-blocking, modernization suggestions):

- `ops.go:265,440,501`: Use `strings.SplitSeq` for efficiency (Go 1.25+ feature)
- `ops.go:540`: Replace `[]byte(fmt.Sprintf(...))` with `fmt.Appendf`

No errors or warnings.

## License & Attribution

All code must include header:

```go
// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only
```

Present in:
- `cmd/avdctl/main.go:1-2`
- `internal/avd/env.go:1-2`
- `internal/avd/ops.go:1-2`
- `dockerfile:1-2`
- `config.ini.tpl:1-3`

## Quick Start for New Developers

1. **Install prerequisites**:
   ```bash
   # Android SDK (if not present)
   # Install to /opt/android-sdk or set $ANDROID_SDK_ROOT
   
   # Install qemu-img
   sudo apt install qemu-utils  # Debian/Ubuntu
   ```

2. **Build**:
   ```bash
   task build
   ```

3. **Set environment** (optional, Taskfile has defaults):
   ```bash
   export ANDROID_SDK_ROOT=/opt/android-sdk
   export ANDROID_AVD_HOME=$HOME/.android/avd
   export AVDCTL_GOLDEN_DIR=$HOME/avd-golden
   ```

4. **Run full workflow**:
   ```bash
   task fresh    # Creates base-a35, prewarmed golden, two clones
   task run-acme # Start emulator
   task ps       # Verify running
   task stop NAME=w-acme
   ```

5. **Explore commands**:
   ```bash
   ./bin/avdctl --help
   ./bin/avdctl list --json
   ```

## Related Documentation

- **README.md**: Quick usage examples (30 lines, focused on CLI usage)
- **Taskfile.yml**: Build tasks and workflow automation
- **config.ini.tpl**: AVD configuration template with comments

## Future Enhancements (Not Implemented)

- `AVDCTL_CLONES_DIR` is defined but unused (reserved for separate clone storage)
- No automated tests beyond `TestDetect`
- No CI/CD pipeline
- PID detection only works on Linux (uses `/proc`)
- `status` command could show more metrics (CPU, memory)
- Parallel prewarming would need dynamic port allocation

---

**Last Updated**: 2025-10-31  
**For**: avdctl development (github.com/forkbombeu/avdctl)
