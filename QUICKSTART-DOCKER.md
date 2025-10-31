# Docker Quick Start Guide

Step-by-step guide to get up and running with avdctl in Docker.

## Step 1: Build and Start Container

```bash
# Build and start in background
docker-compose up -d --build

# Watch build progress (if needed)
docker-compose logs -f
```

## Step 2: Enter Container and Create Base AVD

```bash
# Enter container shell
docker-compose exec avdctl bash

# Inside container: Create base AVD
avdctl init-base --name base-a35

# This will:
# - Download Android 35 system image with Play Store (~1-2 GB)
# - Create base AVD in /root/.android/avd/base-a35.avd
# - Takes 5-10 minutes depending on internet speed

# Exit container when done
exit
```

## Step 3: Find AVD Volume Location on Host

```bash
# Get the volume mount point
docker volume inspect avdctl_avd-home --format '{{ .Mountpoint }}'

# Example output: /var/lib/docker/volumes/avdctl_avd-home/_data

# List AVDs (requires sudo)
sudo ls -la /var/lib/docker/volumes/avdctl_avd-home/_data/
```

You should see:
- `base-a35.avd/` - Directory with AVD files
- `base-a35.ini` - AVD configuration

## Step 4: Configure AVD from Host with Emulator GUI

**Option A: Use Host Emulator Directly on Docker Volume**

```bash
# Set AVD home to Docker volume
export ANDROID_AVD_HOME=/var/lib/docker/volumes/avdctl_avd-home/_data

# Launch emulator with GUI (requires Android SDK on host)
emulator -avd base-a35 -no-snapshot

# Or with writable mode (needed for first boot)
sudo emulator -avd base-a35 -no-snapshot
```

**Option B: Copy AVD to Host, Configure, Copy Back**

```bash
# Copy AVD to your host AVD directory
sudo cp -r /var/lib/docker/volumes/avdctl_avd-home/_data/base-a35.* ~/.android/avd/

# Fix permissions
sudo chown -R $USER:$USER ~/.android/avd/base-a35.*

# Launch emulator normally
emulator -avd base-a35 -no-snapshot
```

### In the Emulator GUI, Configure:

✅ **Skip to start, then go to Settings:**
1. **Google Account**
   - Settings → Accounts → Add account → Google
   - Sign in with your test Google account
   - This enables Play Store

2. **Fingerprint** (optional)
   - Settings → Security → Fingerprint
   - Use emulator's "Touch sensor" button (sidebar)
   - Enroll at least one fingerprint

3. **Developer Options**
   - Settings → About phone → Tap "Build number" 7 times
   - Settings → System → Developer options
   - Enable "Stay awake"
   - Set all animation scales to 0.5x or off

4. **Install Base Apps** (optional)
   - Open Play Store
   - Install any apps you want in all clones
   - Wait for installations to complete

5. **Let It Settle**
   - Leave emulator idle for 30-60 seconds
   - Let background processes finish

6. **Close Cleanly**
   ```bash
   # From another terminal
   adb emu kill
   ```

**If using Option B, copy back:**

```bash
# Copy configured AVD back to Docker volume
sudo cp -r ~/.android/avd/base-a35.* /var/lib/docker/volumes/avdctl_avd-home/_data/
```

## Step 5: Save Golden Image

```bash
# Enter container again
docker-compose exec avdctl bash

# Save the configured AVD as golden image
avdctl save-golden --name base-a35 \
  --dest /avd-golden/base-a35-configured.qcow2

# Verify
ls -lh /avd-golden/
# Should show base-a35-configured.qcow2 (500MB-2GB)

exit
```

## Step 6: Create Customer Clones

```bash
# Enter container
docker-compose exec avdctl bash

# Create clones (fast - just creates thin overlays)
avdctl clone --base base-a35 --name w-customer1 \
  --golden /avd-golden/base-a35-configured.qcow2

avdctl clone --base base-a35 --name w-customer2 \
  --golden /avd-golden/base-a35-configured.qcow2

avdctl clone --base base-a35 --name w-customer3 \
  --golden /avd-golden/base-a35-configured.qcow2

# List clones
avdctl list
```

## Step 7: Run Clones in Parallel

```bash
# Still inside container

# Start multiple instances (background)
avdctl run --name w-customer1 --port 5580 &
avdctl run --name w-customer2 --port 5582 &
avdctl run --name w-customer3 --port 5584 &

# Wait for boot
sleep 30

# Check status
avdctl ps

# Should show all 3 running
```

## Step 8: Connect from Host

```bash
# On host (new terminal)
adb devices

# Should see:
# emulator-5580   device
# emulator-5582   device
# emulator-5584   device

# Run commands
adb -s emulator-5580 shell getprop ro.build.version.release
adb -s emulator-5582 shell pm list packages | grep google
adb -s emulator-5584 shell dumpsys window | grep mCurrentFocus

# Install APK to specific emulator
adb -s emulator-5580 install myapp.apk
```

## Step 9: Stop Instances

```bash
# Inside container
avdctl stop --name w-customer1
avdctl stop --name w-customer2
avdctl stop --name w-customer3

# Verify all stopped
avdctl ps
# Should show: (no emulators)
```

---

## Quick Reference Commands

### Container Management
```bash
# Start
docker-compose up -d

# Stop
docker-compose down

# Logs
docker-compose logs -f

# Enter shell
docker-compose exec avdctl bash

# Restart
docker-compose restart
```

### AVD Management (inside container)
```bash
# List AVDs
avdctl list
avdctl list --json

# List running
avdctl ps
avdctl ps --json

# Stop instance
avdctl stop --name w-customer1
avdctl stop --serial emulator-5580

# Delete clone
avdctl delete w-customer1
```

### Volume Access (on host)
```bash
# AVD location
docker volume inspect avdctl_avd-home --format '{{ .Mountpoint }}'

# Golden images location
docker volume inspect avdctl_avd-golden --format '{{ .Mountpoint }}'

# List AVDs
sudo ls -la $(docker volume inspect avdctl_avd-home --format '{{ .Mountpoint }}')

# List golden images
sudo ls -lh $(docker volume inspect avdctl_avd-golden --format '{{ .Mountpoint }}')
```

---

## Troubleshooting

### "Cannot find AVD" after configuring from host

If you used Option A (direct access to volume) and got permission errors:

```bash
# Fix permissions in container
docker-compose exec avdctl bash
chown -R root:root /root/.android/avd
exit
```

### Emulator won't start - KVM error

```bash
# Check KVM on host
ls -l /dev/kvm

# Fix permissions
sudo chmod 666 /dev/kvm

# Restart container
docker-compose restart
```

### Can't see emulator from host ADB

```bash
# The container exposes ports 5554-5586
# But adb needs to connect explicitly

# Inside container, check what's running
docker-compose exec avdctl avdctl ps

# On host, connect to those ports
adb connect localhost:5555  # for emulator-5554
adb connect localhost:5557  # for emulator-5556
adb connect localhost:5581  # for emulator-5580
```

### Golden image is too large

```bash
# Compress more aggressively (inside container)
qemu-img convert -O qcow2 -c -o compression_type=zstd \
  /avd-golden/base-a35-configured.qcow2 \
  /avd-golden/base-a35-compressed.qcow2

# Use compressed version for clones
avdctl clone --base base-a35 --name w-test \
  --golden /avd-golden/base-a35-compressed.qcow2
```

### Want to start fresh

```bash
# Stop and remove container
docker-compose down

# Delete volumes (WARNING: destroys all AVDs and golden images!)
docker volume rm avdctl_avd-home avdctl_avd-golden

# Start fresh
docker-compose up -d --build
```

---

## Alternative: Headless Configuration (No GUI)

If you don't have Android SDK on host or don't want to use GUI:

```bash
# Inside container, use prewarm
docker-compose exec avdctl bash

# Create base
avdctl init-base --name base-a35

# Auto-boot and save (no manual config)
avdctl prewarm --name base-a35 \
  --dest /avd-golden/base-a35-prewarmed.qcow2 \
  --extra 45s \
  --timeout 5m

# This creates a clean, booted golden image
# BUT: No Google account, no Play Store login, no fingerprint
```

**Note:** Without GUI configuration, clones won't have Google account or Play Store access. Good for testing apps that don't need Google services.

---

## Next Steps

- Read **DOCKER.md** for advanced usage (baking APKs, backups, CI/CD)
- Read **README.md** for native (non-Docker) usage
- Read **CRUSH.md** for development details

---

## Summary Workflow

```
1. docker-compose up -d --build           # Start container
2. docker-compose exec avdctl bash        # Enter container
3. avdctl init-base --name base-a35       # Create base
4. exit                                   # Exit container
5. sudo emulator -avd base-a35 ...        # Configure with GUI on host
   (or copy to ~/.android/avd, configure, copy back)
6. docker-compose exec avdctl bash        # Enter container again
7. avdctl save-golden --name base-a35 ... # Save golden
8. avdctl clone ... (repeat for N customers)
9. avdctl run --name w-customer1 ... &    # Run in parallel
10. avdctl ps                              # Monitor
```

Enjoy! 🚀
