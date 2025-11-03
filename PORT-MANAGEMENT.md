# Port Management and Parallel Execution

## Overview

`avdctl` is designed to run many emulator instances in parallel safely. All port management is dynamic and collision-free.

## Port Allocation

### No Hardcoded Ports

All functions dynamically find free ports:

- ✅ `RunAVD()` - Auto-finds free port in range 5580-5800
- ✅ `Prewarm()` - Auto-finds free port (was hardcoded 5580, now fixed)
- ✅ `RunOnPort()` - Validates port availability before starting
- ✅ `ListRunning()` - Scans full range 5554-5800

### Port Range

- **Default range**: 5554-5800 (Android emulator standard)
- **Port pairs**: Each emulator uses port N (console) and N+1 (ADB)
- **Must be even**: Odd ports are automatically rejected with clear error

### Port Validation

Every `StartEmulatorOnPort` call validates:
1. Port is even (required by Android emulator)
2. Port is in valid range (5554-5800)
3. Port and port+1 are both free
4. Returns descriptive error if any check fails

## Error Handling

### Clear Error Messages

All port-related errors include actionable information:

```
❌ "port 5581 is odd; emulator requires even port numbers (uses port and port+1)"
❌ "port %d or %d already in use"
❌ "no free port available (checked 5580-5800): Hint: Stop some emulators or increase the port range"
❌ "port %d out of valid range (5554-5800)"
```

### Function-Specific Errors

- `RunAVD()` - Returns error if no free ports in range
- `RunOnPort()` - Validates requested port before attempting to start
- `StopBySerial()` - Validates serial format and reports ADB errors
- `Prewarm()` - Returns error if port allocation fails

## Parallel Execution Patterns

### Pattern 1: Auto-Assign Ports (Recommended)

```go
mgr := avdmanager.New()

// Start multiple instances - ports assigned automatically
for i := 1; i <= 10; i++ {
    err := mgr.Run(avdmanager.RunOptions{
        Name: fmt.Sprintf("customer%d", i),
    })
    if err != nil {
        log.Printf("Failed to start customer%d: %v", i, err)
        // Continue with others
    }
}
```

### Pattern 2: Explicit Ports (Full Control)

```go
mgr := avdmanager.New()

// Start with specific ports
ports := []int{5580, 5582, 5584, 5586, 5588}
for i, port := range ports {
    serial, logPath, err := mgr.RunOnPort(avdmanager.RunOptions{
        Name: fmt.Sprintf("customer%d", i+1),
        Port: port,
    })
    if err != nil {
        log.Printf("Port %d failed: %v", port, err)
        continue
    }
    log.Printf("Started on %s (log: %s)", serial, logPath)
}
```

### Pattern 3: Dynamic Port Discovery

```go
mgr := avdmanager.New()

// Find a free port before starting
port, err := mgr.FindFreePort(5580, 5800)
if err != nil {
    log.Fatal("No free ports available")
}

serial, logPath, err := mgr.RunOnPort(avdmanager.RunOptions{
    Name: "customer1",
    Port: port,
})
```

## Monitoring Running Instances

### ListRunning Strategy

`ListRunning()` uses two strategies to find ALL emulators:

1. **ADB devices** - Fast, but may miss just-started emulators
2. **Process table scan** - Finds emulators that ADB hasn't detected yet

This ensures you always get a complete list.

### Usage

```go
running, err := mgr.ListRunning()
if err != nil {
    log.Fatal(err)
}

for _, p := range running {
    fmt.Printf("%s (%s) on port %d - %s\n", 
        p.Name, p.Serial, p.Port, 
        map[bool]string{true: "ready", false: "booting"}[p.Booted])
}
```

## Resource Limits

### System Limits

Each emulator instance requires:
- **RAM**: ~2-4 GB
- **CPU**: 2-4 cores (with KVM acceleration)
- **Disk**: 196 KB (clone overlay initially)
- **Ports**: 2 (console + ADB)

### Practical Limits

On a typical development machine:
- **16GB RAM**: ~4-6 emulators
- **32GB RAM**: ~8-12 emulators  
- **64GB RAM**: ~15-20 emulators

**Port limit**: 123 emulator pairs (5554-5800 range)

### Detecting Issues

The library will return errors when:
- ❌ No free ports available
- ❌ Port already in use
- ❌ Emulator fails to start
- ❌ Boot timeout (with helpful fallback for ADB issues)

## Best Practices

### 1. Always Check Errors

```go
serial, logPath, err := mgr.RunOnPort(...)
if err != nil {
    log.Printf("Failed: %v", err)
    // Check the log file for details
    log.Printf("See log: %s", logPath)
}
```

### 2. Use Auto-Assignment for Simplicity

```go
// Let the library find free ports
err := mgr.Run(avdmanager.RunOptions{Name: "test"})
```

### 3. Monitor with ListRunning

```go
// Check status periodically
running, _ := mgr.ListRunning()
log.Printf("Currently running: %d emulators", len(running))
```

### 4. Clean Up When Done

```go
// Stop all instances
running, _ := mgr.ListRunning()
for _, p := range running {
    mgr.Stop(p.Serial)
}
```

### 5. Handle Partial Failures

```go
// Don't let one failure stop all
for i := 1; i <= 10; i++ {
    if err := mgr.Run(...); err != nil {
        log.Printf("Instance %d failed: %v", i, err)
        continue // Keep going
    }
}
```

## Troubleshooting

### "No free port available"

**Cause**: All ports 5580-5800 are in use  
**Solution**: 
1. Stop unused emulators: `avdctl ps` then `avdctl stop --serial ...`
2. Kill zombie processes: `killall qemu-system-x86_64-headless`
3. Clear stale runtime files: `rm -f ~/.android/modem-nv-ram-*`

### "Port already in use"

**Cause**: Another process is using that port  
**Solution**: 
- Use auto-assignment instead of explicit ports
- Check what's using the port: `lsof -i :5580`

### Emulators not showing in `adb devices`

**Cause**: ADB hasn't detected them yet (normal for just-started emulators)  
**Solution**: 
- Use `avdctl ps` instead (scans process table directly)
- Wait 10-30 seconds for ADB to detect
- Use explicit serial: `adb -s emulator-5580 shell`

### "adb: more than one emulator"

**Cause**: ADB command without `-s` serial when multiple emulators running  
**Solution**: Always use `-s emulator-XXXX` with specific serial

## Summary

✅ **No hardcoded ports** - Everything is dynamic  
✅ **Clear error messages** - Know exactly what went wrong  
✅ **Collision-free** - Port availability checked before use  
✅ **Resilient** - Handles ADB flakiness gracefully  
✅ **Scalable** - Run as many instances as your hardware allows  
✅ **Observable** - Always know what's running via `ListRunning()`  

Your parallel emulator setup is production-ready! 🚀
