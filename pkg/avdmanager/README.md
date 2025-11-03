# avdmanager - Go Library for Android Virtual Device Management

Public Go library for managing Android Virtual Devices (AVDs) with golden image and clone workflows.

## Installation

```bash
go get github.com/forkbombeu/avdctl/pkg/avdmanager
```

## Quick Start

```go
package main

import (
    "log"
    "github.com/forkbombeu/avdctl/pkg/avdmanager"
)

func main() {
    // Create manager (auto-detects environment)
    mgr := avdmanager.New()

    // Create base AVD
    _, err := mgr.InitBase(avdmanager.InitBaseOptions{
        Name:        "base-a35",
        SystemImage: "system-images;android-35;google_apis_playstore;x86_64",
        Device:      "pixel_6",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Save golden image (after manual configuration)
    goldenPath, _, err := mgr.SaveGolden(avdmanager.SaveGoldenOptions{
        Name:        "base-a35",
        Destination: "/tmp/base-golden.qcow2",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Create clones
    for i := 1; i <= 3; i++ {
        _, err := mgr.Clone(avdmanager.CloneOptions{
            BaseName:   "base-a35",
            CloneName:  fmt.Sprintf("customer%d", i),
            GoldenPath: goldenPath,
        })
        if err != nil {
            log.Fatal(err)
        }
    }

    // Run instances in parallel
    for i := 1; i <= 3; i++ {
        port := 5580 + (i-1)*2
        _, _, err := mgr.RunOnPort(avdmanager.RunOptions{
            Name: fmt.Sprintf("customer%d", i),
            Port: port,
        })
        if err != nil {
            log.Fatal(err)
        }
    }

    // List running
    running, _ := mgr.ListRunning()
    for _, p := range running {
        log.Printf("Running: %s on %s (booted: %v)", p.Name, p.Serial, p.Booted)
    }
}
```

## Core Types

### Manager

Main entry point for AVD operations.

```go
// Auto-detect environment
mgr := avdmanager.New()

// Custom environment
mgr := avdmanager.NewWithEnv(avdmanager.Environment{
    SDKRoot:   "/opt/android-sdk",
    AVDHome:   "/custom/avd",
    GoldenDir: "/custom/golden",
})
```

### AVDInfo

Information about an AVD:

```go
type AVDInfo struct {
    Name      string // AVD name
    Path      string // Path to .avd directory
    Userdata  string // Path to userdata file
    SizeBytes int64  // Size of userdata in bytes
}
```

### ProcessInfo

Information about a running emulator:

```go
type ProcessInfo struct {
    Serial string // e.g., "emulator-5580"
    Name   string // AVD name
    Port   int    // Console port
    PID    int    // Process ID
    Booted bool   // Whether Android has fully booted
}
```

## API Reference

### Base AVD Management

#### InitBase

Create a new base AVD. Auto-installs system image if missing.

```go
info, err := mgr.InitBase(avdmanager.InitBaseOptions{
    Name:        "base-a35",
    SystemImage: "system-images;android-35;google_apis_playstore;x86_64",
    Device:      "pixel_6",
})
```

#### List

List all AVDs:

```go
avds, err := mgr.List()
for _, avd := range avds {
    fmt.Printf("%s: %s (%d bytes)\n", avd.Name, avd.Path, avd.SizeBytes)
}
```

#### Delete

Delete an AVD:

```go
err := mgr.Delete("base-a35")
```

### Golden Image Operations

#### SaveGolden

Export an AVD's userdata to a compressed QCOW2 golden image:

```go
path, size, err := mgr.SaveGolden(avdmanager.SaveGoldenOptions{
    Name:        "base-a35",
    Destination: "/tmp/golden.qcow2",
})
```

#### Prewarm

Boot an AVD once, wait for full boot, settle caches, then save as golden:

```go
path, size, err := mgr.Prewarm(avdmanager.PrewarmOptions{
    Name:        "base-a35",
    Destination: "/tmp/prewarmed.qcow2",
    ExtraSettle: 45 * time.Second,
    BootTimeout: 5 * time.Minute,
})
```

Use this for automated golden creation without manual configuration.

#### BakeAPK

Create a clone, boot it, install APKs, then export as a new golden:

```go
_, _, err := mgr.BakeAPK(avdmanager.BakeAPKOptions{
    BaseName:    "base-a35",
    CloneName:   "customer-baked",
    GoldenPath:  "/tmp/base-golden.qcow2",
    APKPaths:    []string{"/path/app1.apk", "/path/app2.apk"},
    Destination: "/tmp/baked.qcow2",
    BootTimeout: 5 * time.Minute,
})
```

### Clone Management

#### Clone

Create a lightweight clone backed by a golden image:

```go
info, err := mgr.Clone(avdmanager.CloneOptions{
    BaseName:   "base-a35",
    CloneName:  "customer1",
    GoldenPath: "/tmp/golden.qcow2",
})
```

Clones are thin QCOW2 overlays - they only store changes, not the full system.

### Emulator Operations

#### Run

Start an emulator (auto-assigns port):

```go
err := mgr.Run(avdmanager.RunOptions{
    Name: "customer1",
})
```

#### RunOnPort

Start an emulator on a specific port:

```go
serial, logPath, err := mgr.RunOnPort(avdmanager.RunOptions{
    Name: "customer1",
    Port: 5580, // Must be even
})
// Returns: serial = "emulator-5580", logPath = "/tmp/emulator-customer1-5580.log"
```

#### ListRunning

List all running emulators:

```go
running, err := mgr.ListRunning()
for _, p := range running {
    fmt.Printf("%s on %s (port %d, pid %d, booted: %v)\n",
        p.Name, p.Serial, p.Port, p.PID, p.Booted)
}
```

#### Stop

Stop an emulator by serial:

```go
err := mgr.Stop("emulator-5580")
```

#### StopByName

Stop an emulator by AVD name:

```go
err := mgr.StopByName("customer1")
```

#### WaitForBoot

Wait for Android to fully boot:

```go
err := mgr.WaitForBoot("emulator-5580", 3*time.Minute)
```

### Utility Functions

#### FindFreePort

Find a free even port pair for emulator:

```go
port, err := mgr.FindFreePort(5580, 5800)
// Returns first free even port (emulator uses port and port+1)
```

## Complete Example: Parallel Testing

```go
package main

import (
    "fmt"
    "log"
    "sync"
    "time"

    "github.com/forkbombeu/avdctl/pkg/avdmanager"
)

func main() {
    mgr := avdmanager.New()

    // Setup: Create base and golden (once)
    _, err := mgr.InitBase(avdmanager.InitBaseOptions{
        Name:        "base-a35",
        SystemImage: "system-images;android-35;google_apis_playstore;x86_64",
        Device:      "pixel_6",
    })
    if err != nil {
        log.Fatal(err)
    }

    goldenPath, _, err := mgr.SaveGolden(avdmanager.SaveGoldenOptions{
        Name:        "base-a35",
        Destination: "/tmp/base-golden.qcow2",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Create 5 clones
    numInstances := 5
    for i := 1; i <= numInstances; i++ {
        name := fmt.Sprintf("test%d", i)
        _, err := mgr.Clone(avdmanager.CloneOptions{
            BaseName:   "base-a35",
            CloneName:  name,
            GoldenPath: goldenPath,
        })
        if err != nil {
            log.Fatal(err)
        }
    }

    // Run all in parallel
    var wg sync.WaitGroup
    for i := 1; i <= numInstances; i++ {
        wg.Add(1)
        go func(idx int) {
            defer wg.Done()
            
            name := fmt.Sprintf("test%d", idx)
            port := 5580 + (idx-1)*2
            
            // Start emulator
            serial, _, err := mgr.RunOnPort(avdmanager.RunOptions{
                Name: name,
                Port: port,
            })
            if err != nil {
                log.Printf("Failed to start %s: %v", name, err)
                return
            }
            
            // Wait for boot
            if err := mgr.WaitForBoot(serial, 3*time.Minute); err != nil {
                log.Printf("Boot timeout for %s: %v", name, err)
                return
            }
            
            log.Printf("%s ready on %s", name, serial)
            
            // Run your tests here...
            // adb -s <serial> shell am instrument ...
            
            // Cleanup
            mgr.Stop(serial)
        }(i)
    }

    wg.Wait()
    log.Println("All tests complete")
}
```

## Environment Variables

The library respects these environment variables (if not overridden):

- `ANDROID_SDK_ROOT` - Android SDK path
- `ANDROID_AVD_HOME` - AVD storage directory (default: `~/.android/avd`)
- `AVDCTL_GOLDEN_DIR` - Golden images directory (default: `~/avd-golden`)
- `AVDCTL_CLONES_DIR` - Clones directory (optional)
- `AVDCTL_CONFIG_TEMPLATE` - Path to custom `config.ini` template (optional)

## Requirements

- Android SDK with:
  - `emulator`
  - `adb`
  - `avdmanager`
  - `sdkmanager`
- `qemu-img` (from qemu-utils package)
- KVM for hardware acceleration (Linux)

## Thread Safety

`Manager` instances are **not** thread-safe. For concurrent use:

- Create separate `Manager` instances per goroutine, OR
- Synchronize access with a mutex

## Best Practices

1. **Create base once, clone many** - Base AVD creation is slow, cloning is fast
2. **Use prewarming for CI** - Automates boot/save without manual config
3. **Explicit ports for parallel** - Use `RunOnPort()` with even ports (5580, 5582, 5584...)
4. **Clean up clones** - Delete clones after use to free disk space
5. **Wait for boot** - Always call `WaitForBoot()` before running tests
6. **Check logs** - If emulator fails, check the log file returned by `RunOnPort()`

## Limitations

- **Linux-only PID detection** - `ProcessInfo.PID` uses `/proc`, won't work on macOS/Windows
- **Parallel instances** - Limited by system resources (each emulator needs ~4GB RAM)
- **Ports** - Emulator uses port pairs (console + adb), must be even numbers
- **QEMU file locking** - The library automatically disables it (`QEMU_FILE_LOCKING=off`)

## License

AGPL-3.0-only

Copyright (C) 2025 Forkbomb B.V.

## See Also

- [Main README](../../README.md) - CLI usage
- [DOCKER.md](../../DOCKER.md) - Docker setup
- [CRUSH.md](../../CRUSH.md) - Development guide
