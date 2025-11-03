# avdctl - Android Virtual Device Lifecycle Manager

`avdctl` is a CLI tool for managing Android Virtual Device (AVD) golden images and clones. It enables fast creation of disposable emulator instances backed by QCOW2 golden images, perfect for CI/CD pipelines and parallel testing.

## Prerequisites

1. **Android SDK** with command-line tools installed:
   - `emulator`
   - `adb`
   - `avdmanager`
   - `sdkmanager`

2. **QEMU utilities**:
   ```bash
   # Debian/Ubuntu
   sudo apt install qemu-utils
   
   # macOS
   brew install qemu
   ```

3. **Go 1.25+** (for building from source)

4. **Task** (optional, for using Taskfile workflows):
   ```bash
   # Install from https://taskfile.dev
   go install github.com/go-task/task/v3/cmd/task@latest
   ```

## Environment Setup

Set these environment variables (or let them use defaults):

```bash
export ANDROID_SDK_ROOT=/opt/android-sdk              # Your Android SDK path
export ANDROID_AVD_HOME=$HOME/.android/avd            # Default: ~/.android/avd
export AVDCTL_GOLDEN_DIR=$HOME/avd-golden             # Default: ~/avd-golden
export AVDCTL_CONFIG_TEMPLATE=/path/to/config.ini.tpl # Optional: custom config template
```

## Quick Start

### 1. Build

```bash
# Using Task
task build

# Or using Go directly
go build -o bin/avdctl ./cmd/avdctl

# Verify
./bin/avdctl --help
```

### 2. Create a Fresh Base AVD

```bash
# Create a base AVD (Android 35, Google Play Store, Pixel 6 profile)
# This will auto-download the system image if not present
./bin/avdctl init-base --name base-a35 \
  --image "system-images;android-35;google_apis_playstore;x86_64" \
  --device pixel_6
```

### 3. Boot and Configure (Manual Setup)

Now boot the AVD with a GUI to configure it manually:

```bash
# Start emulator with window (NOT using avdctl)
emulator -avd base-a35 -no-snapshot
```

**In the emulator, perform your manual configuration:**

- ✅ Add Google account (Play Store login)
- ✅ Enroll fingerprint (Settings → Security → Fingerprint)
- ✅ Install any base apps you want in all clones
- ✅ Adjust system settings (locale, timezone, etc.)
- ✅ Disable animations (Settings → Developer Options → Window/Transition/Animator scale → off)
- ✅ Enable "Stay awake" (Developer Options)
- ✅ Configure Wi-Fi/network settings
- ✅ Accept all first-run wizards

**Important:** Let the emulator fully settle (30-60 seconds idle) after all changes.

**Then shutdown cleanly:**

```bash
# From another terminal
adb emu kill

# OR from emulator console
adb shell reboot -p
```

### 4. Save the Configured Golden Image

```bash
# Export the configured userdata as a compressed golden QCOW2
./bin/avdctl save-golden --name base-a35 \
  --dest "$HOME/avd-golden/base-a35-configured.qcow2"
```

**Alternatively, use `prewarm` for automated boot+save:**

```bash
# Boot once, wait for Android to fully start, settle caches, then save
# (No manual intervention - good for clean base images without Google account)
./bin/avdctl prewarm --name base-a35 \
  --dest "$HOME/avd-golden/base-a35-prewarmed.qcow2" \
  --extra 30s \
  --timeout 3m
```

**Use `prewarm` for clean bases, `save-golden` after manual configuration.**

---

## Working with Customers (Clones)

### Create Customer Clones

Each customer gets a lightweight clone backed by the golden image:

```bash
# Customer 1
./bin/avdctl clone --base base-a35 --name w-customer1 \
  --golden "$HOME/avd-golden/base-a35-configured.qcow2"

# Customer 2
./bin/avdctl clone --base base-a35 --name w-customer2 \
  --golden "$HOME/avd-golden/base-a35-configured.qcow2"

# Customer 3
./bin/avdctl clone --base base-a35 --name w-customer3 \
  --golden "$HOME/avd-golden/base-a35-configured.qcow2"
```

**Naming convention:** `w-<slug>` (e.g., `w-acme`, `w-contoso`, `w-initech`)

### Run Customer Emulators

```bash
# Auto-assign ports (finds free even port pair)
./bin/avdctl run --name w-customer1

# Or specify explicit ports for parallel instances
./bin/avdctl run --name w-customer1 --port 5580
./bin/avdctl run --name w-customer2 --port 5582
./bin/avdctl run --name w-customer3 --port 5584
```

**Port notes:**
- Must be **even** numbers (emulator uses port + port+1)
- Each instance needs a unique port pair
- Default range: 5554-5586 (adb auto-discovery range)

### Monitor Running Instances

```bash
# Human-readable output
./bin/avdctl ps

# JSON output
./bin/avdctl ps --json

# Check specific instance status
./bin/avdctl status --name w-customer1
./bin/avdctl status --serial emulator-5580
```

### Stop Instances

```bash
# By name
./bin/avdctl stop --name w-customer1

# By serial
./bin/avdctl stop --serial emulator-5580
```

### List All AVDs

```bash
./bin/avdctl list
./bin/avdctl list --json
```

### Delete Customer Clone

```bash
./bin/avdctl delete w-customer1
```

**Note:** This only deletes the clone's overlay (a few MB). The golden image remains untouched.

---

## Advanced Workflows

### Baking APKs into a Golden Image

Pre-install APKs into a golden image for faster clone startup:

```bash
./bin/avdctl bake-apk --base base-a35 --name w-baked \
  --golden "$HOME/avd-golden/base-a35-configured.qcow2" \
  --apk /path/to/app1.apk \
  --apk /path/to/app2.apk \
  --dest "$HOME/avd-golden/base-a35-with-apps.qcow2"
```

This creates a new golden image with APKs pre-installed. Use it for clones:

```bash
./bin/avdctl clone --base base-a35 --name w-customer-with-apps \
  --golden "$HOME/avd-golden/base-a35-with-apps.qcow2"
```

### Using Custom Config Template

If you have a custom `config.ini.tpl`, set it before cloning:

```bash
export AVDCTL_CONFIG_TEMPLATE=/path/to/custom-config.ini.tpl
./bin/avdctl clone --base base-a35 --name w-custom ...
```

### Parallel Testing Workflow

```bash
# Start 4 instances in parallel
for i in {1..4}; do
  port=$((5580 + (i-1)*2))
  ./bin/avdctl run --name w-test$i --port $port &
done

# Wait for all to boot
sleep 30
./bin/avdctl ps

# Run tests against each
adb -s emulator-5580 shell am instrument ...
adb -s emulator-5582 shell am instrument ...
adb -s emulator-5584 shell am instrument ...
adb -s emulator-5586 shell am instrument ...

# Stop all
for i in {1..4}; do
  port=$((5580 + (i-1)*2))
  ./bin/avdctl stop --serial emulator-$port
done
```

---

## Complete Example: From Scratch

```bash
# 1. Build tool
task build

# 2. Create directories
mkdir -p ~/avd-golden

# 3. Create base AVD
./bin/avdctl init-base --name base-a35

# 4. Boot manually and configure (Google account, fingerprint, etc.)
emulator -avd base-a35 -no-snapshot
# ... do manual setup in GUI ...
# ... close emulator when done ...

# 5. Save the golden image
./bin/avdctl save-golden --name base-a35 \
  --dest ~/avd-golden/base-a35-configured.qcow2

# 6. Create customer clones
./bin/avdctl clone --base base-a35 --name w-customer1 \
  --golden ~/avd-golden/base-a35-configured.qcow2

./bin/avdctl clone --base base-a35 --name w-customer2 \
  --golden ~/avd-golden/base-a35-configured.qcow2

# 7. Run both in parallel
./bin/avdctl run --name w-customer1 --port 5580 &
./bin/avdctl run --name w-customer2 --port 5582 &

# 8. Verify
sleep 10
./bin/avdctl ps
adb devices

# 9. Stop when done
./bin/avdctl stop --name w-customer1
./bin/avdctl stop --name w-customer2
```

---

## Using Taskfile (Optional)

If you prefer Task automation:

```bash
# Full workflow
task fresh          # Clean → init-base → prewarm → clone 2 customers

# Individual tasks
task build          # Build binary
task init-base      # Create base-a35
task prewarm        # Prewarm base-a35
task clone-customer # Clone a customer (edit Taskfile.yml for name)
task run-customer   # Run a customer (edit Taskfile.yml for name/port)
task ps             # List running
task stop NAME=w-customer1  # Stop instance
task clean-avds     # Delete all AVDs (danger!)
```

Edit `Taskfile.yml` to customize names, ports, and golden paths.

---

## Troubleshooting

### Emulator won't start

Check logs at `/tmp/emulator-<name>-<port>.log`:

```bash
tail -f /tmp/emulator-w-customer1-5580.log
```

### "Failed to get write lock" error

This shouldn't happen with the latest version (uses `QEMU_FILE_LOCKING=off`). If you see it:

1. Ensure you're using the latest build
2. Check for stale emulator processes: `ps aux | grep emulator`
3. Kill them: `killall qemu-system-x86_64-headless`

### Port already in use

```bash
# Find free port
./bin/avdctl run --name w-customer1  # Auto-assigns free port

# Or manually check
lsof -i :5580
```

### Clone overlay grows too large

Clones are QCOW2 overlays - they only store changes. But if a clone's userdata grows too large:

```bash
# Check size
ls -lh ~/.android/avd/w-customer1.avd/userdata-qemu.img.qcow2

# If too large, delete and recreate
./bin/avdctl delete w-customer1
./bin/avdctl clone --base base-a35 --name w-customer1 --golden ~/avd-golden/base-a35-configured.qcow2
```

### Boot is slow

- Disable animations in Developer Options (in the golden image)
- Use `--extra` flag with `prewarm` to let caches settle
- Use SSD storage for AVD home and golden directory
- Allocate more RAM in `config.ini.tpl` (default: 4GB)

---

## Architecture

- **Base AVD**: Clean Android system created via `avdmanager`
- **Golden Image**: Compressed QCOW2 snapshot of configured userdata
- **Clone**: Symlinks to base AVD read-only files + thin QCOW2 overlay backed by golden
- **Parallel Safe**: Uses `QEMU_FILE_LOCKING=off` and `-read-only` for shared backing files

**Disk Usage:**
- Base AVD: ~8GB (system image + initial userdata)
- Golden QCOW2: ~500MB-2GB (compressed, depends on configuration)
- Clone overlay: ~196KB initially, grows with changes (typically <100MB)

---

## License

AGPL-3.0-only

Copyright (C) 2025 Forkbomb B.V.

---

## Docker Support

For a fully containerized environment with Android SDK pre-installed:

- **[QUICKSTART-DOCKER.md](QUICKSTART-DOCKER.md)** - Step-by-step guide to get running quickly
- **[DOCKER.md](DOCKER.md)** - Complete Docker reference and advanced usage

```bash
# Quick start with Docker
docker-compose up -d --build
docker-compose exec avdctl bash
avdctl init-base --name base-a35
```

## Using as a Go Library

You can import `avdctl` as a library in your Go projects:

```go
import "github.com/forkbombeu/avdctl/pkg/avdmanager"

mgr := avdmanager.New()
mgr.InitBase(avdmanager.InitBaseOptions{...})
mgr.Clone(avdmanager.CloneOptions{...})
mgr.Run(avdmanager.RunOptions{...})
```

See **[pkg/avdmanager/README.md](pkg/avdmanager/README.md)** for complete API documentation and examples.

## See Also

- **[pkg/avdmanager](pkg/avdmanager)** - Go library API documentation
- **[PORT-MANAGEMENT.md](PORT-MANAGEMENT.md)** - Parallel execution and port management guide
- **[QUICKSTART-DOCKER.md](QUICKSTART-DOCKER.md)** - Docker quick start guide
- **[DOCKER.md](DOCKER.md)** - Docker setup with full Android SDK and emulator
- **CRUSH.md** - Detailed development guide for contributors
- **Taskfile.yml** - Task automation examples
- **config.ini.tpl** - AVD configuration template
