# Use Existing Golden Example

This example shows how to use a golden image you already created with `customize-start`/`customize-finish` to spawn multiple parallel emulator instances.

## What it does

1. Uses your existing golden directory (`~/avd-golden/credimi-golden`)
2. Creates 3 customer clones: `w-acme`, `w-globex`, `w-initech`
3. Runs all 3 emulators in parallel
4. Lists running emulators

## Prerequisites

- Existing golden image created with:
  ```bash
  avdctl customize-start --name base-customizable-full
  # ... make your customizations in GUI ...
  avdctl customize-finish --name base-customizable-full --dest ~/avd-golden/credimi-golden
  ```

- At least 8GB RAM (running 3 emulators simultaneously)

## Usage

```bash
# Build and run
go build -o use-existing .
./use-existing
```

## Customization

Edit `main.go` to use your own base and golden:

```go
baseName := "your-base-avd-name"
goldenPath := filepath.Join(os.Getenv("HOME"), "avd-golden", "your-golden-dir")
customers := []string{"customer1", "customer2", "customer3"}
```

## Cleanup

```bash
avdctl stop --name w-acme
avdctl stop --name w-globex
avdctl stop --name w-initech

avdctl delete w-acme
avdctl delete w-globex
avdctl delete w-initech
```

Your golden image remains untouched at `~/avd-golden/credimi-golden`.

## How It Works

`CloneFromGolden` creates thin clones by:
1. Copying raw IMG files from golden directory (`userdata-qemu.img`, `cache.img`, `encryptionkey.img`)
2. Symlinking read-only files from base AVD (system images, ROMs)
3. Creating a sanitized `config.ini` for each clone

Each clone is independent - changes only affect that clone's userdata file.
