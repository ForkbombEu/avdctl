# Parallel Emulator Example

This example demonstrates the full avdctl workflow:

1. Creates a base AVD with Android 35
2. Prewarmed golden image (boots once, settles, exports raw IMG files to directory)
3. Creates 3 customer clones (acme, globex, initech)
4. Runs all 3 emulators in parallel
5. Lists running emulators

## Prerequisites

- Android SDK installed (set `ANDROID_SDK_ROOT` or `/opt/android-sdk`)
- `qemu-img` utility (for QCOW2 operations)
- KVM support (Linux)
- At least 8GB RAM (running 3 emulators simultaneously)

## Usage

```bash
# Build the example
go build -o parallel-example .

# Run (takes ~10-15 minutes for first run)
./parallel-example
```

## What it does

The example will:
- Create `base-a35-example` AVD
- Boot it once to create golden directory at `~/avd-golden/base-a35-example-prewarmed/` (contains raw IMG files)
- Clone 3 instances: `w-acme`, `w-globex`, `w-initech`
- Start all 3 emulators in parallel (auto-assigned ports)
- Wait for all to boot
- List running emulators

## Golden Image Format

The golden image is now a **directory** containing raw IMG files:
- `userdata-qemu.img` - Main userdata partition (~6GB)
- `cache.img` - Cache partition (~66MB)
- `encryptionkey.img` - Encryption key (~18MB)

This replaced the old single QCOW2 file format. Both `prewarm` and `customize-start/finish` create this directory format.

## Using Existing Golden Images

If you already have a golden image created with `customize-start` and `customize-finish`:

```go
goldenPath := filepath.Join(goldenDir, "credimi-golden") // Your existing golden directory
cloneInfo, err := avd.CloneFromGolden(env, baseName, cloneName, goldenPath)
```

`CloneFromGolden` accepts:
- Directory path: `~/avd-golden/my-golden/` (contains IMG files)
- Legacy `.qcow2` path: `~/avd-golden/my-golden.qcow2` (automatically strips extension and looks for directory)

## Cleanup

After the example completes, stop and delete:

```bash
avdctl stop --name w-acme
avdctl stop --name w-globex
avdctl stop --name w-initech

avdctl delete base-a35-example
avdctl delete w-acme
avdctl delete w-globex
avdctl delete w-initech

rm -rf ~/avd-golden/base-a35-example-prewarmed
```

## Notes

- Each emulator uses ~2GB RAM
- Prewarm step takes ~5 minutes (boots emulator, waits for settle)
- Parallel boot takes ~2-3 minutes per emulator
- Check `/tmp/emulator-*.log` for emulator logs
- `GuessEmulatorSerial()` may fail if emulators start too quickly - add delays if needed

## Alternative: Manual Customization Workflow

Instead of automated prewarm, you can customize manually:

```bash
# 1. Create base
avdctl init-base --name base-a35 --image "system-images;android-35;google_apis;x86_64" --device pixel_6

# 2. Start with GUI for manual customization
avdctl customize-start --name base-a35

# 3. Make your changes in the GUI (install APKs, configure settings, etc.)

# 4. When done, export golden
avdctl customize-finish --name base-a35 --dest ~/avd-golden/my-custom-golden

# 5. Clone from your golden
avdctl clone --base base-a35 --name w-acme --golden ~/avd-golden/my-custom-golden
```
