# Fix: Cloning Performance and Maestro Compatibility

## Problem

AVD cloning was failing when used with Maestro automation, causing "device offline" errors and timeouts. Investigation revealed:

1. **Root Cause**: Commit 612c238 (Nov 11, 2025) changed from QCOW2 overlays to in-memory copying of raw IMG files
2. **Impact**: 
   - Cloning a 6GB userdata image took minutes (loading entire file into memory)
   - Emulators often didn't finish booting before Maestro timeout
   - ADB reported devices as "offline" during slow initialization
3. **Error**: `java.io.IOException: Command failed (host:transport:emulator-5580): device offline`

## Solution

Restored the original QCOW2 overlay approach with improvements:

### Changes Made

**File**: `internal/avd/ops.go`

1. **CloneFromGolden** (lines 242-264):
   - Changed from `os.ReadFile`/`os.WriteFile` (copies 6GB into memory)
   - To `qemu-img create -f qcow2 -F raw -b <golden>` (instant overlay creation)
   - Result: ~0.15s cloning vs minutes

2. **Config handling** (lines 194-203):
   - Changed `userdata.useQcow2=no` → `userdata.useQcow2=yes`
   - Enables QCOW2 support in emulator

3. **Info reporting** (lines 282-295):
   - Added fallback to check `.qcow2` overlay first
   - Maintains backward compatibility with raw IMG

### Technical Details

**QCOW2 Overlay Structure**:
```
Golden (read-only):  ~/avd-golden/credimi-golden/userdata-qemu.img (6GB, raw)
Clone overlay:       ~/.android/avd/didroom.avd/userdata-qemu.img.qcow2 (196KB)
```

**Backing file** (checked with `qemu-img info`):
```
image: userdata-qemu.img.qcow2
file format: qcow2
virtual size: 6.0G
disk size: 196K  ← Only stores deltas!
backing file: /home/puria/avd-golden/credimi-golden/userdata-qemu.img
backing file format: raw
```

**Performance**:
- Clone creation: 0.15s (was: minutes)
- Disk space per clone: ~800KB total for all overlays (was: 6GB+ per clone)
- Boot time: Unchanged (~45s to full boot)
- ADB connection: Immediate (was: often timed out)

### Why QCOW2 Overlays Work

1. **Instant cloning**: `qemu-img create` only writes metadata, no data copying
2. **Copy-on-write**: Writes go to overlay, reads fall back to backing file
3. **Space efficient**: Clones share backing file, only store deltas
4. **Parallel-safe**: Multiple overlays can share same backing file (read-only)
5. **Emulator compatible**: Android emulator natively supports QCOW2 when `userdata.useQcow2=yes`

### Testing

Verified with:
```bash
# Create clone
./bin/avdctl clone --base credimi --name test-clone --golden ~/avd-golden/credimi-golden
# → 0.154s

# Boot and verify
./bin/avdctl run --name test-clone --port 5580
sleep 45
./bin/avdctl ps --json
# → {"serial": "emulator-5580", "name": "test-clone", "booted": true}

adb devices
# → emulator-5580    device  (not "offline")
```

Automated test script: `scripts/test-maestro-readiness.sh`

## Related Changes

- Updated `CRUSH.md` to document QCOW2 approach and performance characteristics
- Added test script for Maestro readiness verification
- Confirmed backward compatibility (code checks for both `.qcow2` and raw `.img`)

## Why Original Change Was Made

Commit 612c238 message: "fix: allow easy customization and fix cloning"

Likely attempted to fix an issue with QCOW2 overlays but introduced worse performance bug. The proper fix is to use QCOW2 overlays correctly with `userdata.useQcow2=yes` in config.

## Rollback Safety

This change restores original behavior from initial commit 143a5c2. All existing workflows should continue working:
- `prewarm`: Creates golden raw IMG files (unchanged)
- `clone`: Now uses QCOW2 overlays (fast)
- `run`: Works with both QCOW2 and raw (backward compatible)
- `bake-apk`: Works with QCOW2 clones

## Files Changed

1. `internal/avd/ops.go`: CloneFromGolden function (~30 lines)
2. `CRUSH.md`: Updated documentation (gotcha #7)
3. `scripts/test-maestro-readiness.sh`: New test script (70 lines)

---

**Fixed**: 2025-11-13
**Tested**: Local emulator (Android 35, x86_64)
**Status**: Ready for production use with Maestro
