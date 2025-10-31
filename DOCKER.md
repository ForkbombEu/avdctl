# Docker Setup Guide for avdctl

This guide shows how to run `avdctl` in Docker with full Android SDK, emulator, and KVM acceleration.

## Prerequisites

1. **Linux host** with KVM support (required for hardware acceleration)
2. **Docker** and **Docker Compose** installed
3. **KVM access**:
   ```bash
   # Check if KVM is available
   ls -l /dev/kvm
   
   # If permission denied, add your user to kvm group
   sudo usermod -aG kvm $USER
   
   # Or temporarily (not recommended for production)
   sudo chmod 666 /dev/kvm
   
   # Verify
   groups | grep kvm
   ```

## Quick Start

### 1. Build and Start Container

```bash
# Build image and start container
docker-compose up -d --build

# Check logs
docker-compose logs -f

# Verify container is running
docker ps
```

### 2. Access Container Shell

```bash
# Enter container
docker-compose exec avdctl bash

# Verify tools are installed
avdctl --help
emulator -version
adb version
qemu-img --version
```

### 3. Create Base AVD Inside Container

```bash
# Inside container
avdctl init-base --name base-a35 \
  --image "system-images;android-35;google_apis_playstore;x86_64" \
  --device pixel_6
```

This creates the base AVD in `/root/.android/avd/base-a35.avd` (persisted in `avd-home` volume).

### 4. Configure Golden Image from Host

**Option A: Using Host Emulator GUI** (Recommended)

Access the AVD volume from host and boot with GUI for manual configuration:

```bash
# On host: Find the volume mount point
docker volume inspect avdctl_avd-home --format '{{ .Mountpoint }}'
# Example output: /var/lib/docker/volumes/avdctl_avd-home/_data

# Boot emulator on host with GUI (if you have Android SDK installed on host)
export ANDROID_AVD_HOME=/var/lib/docker/volumes/avdctl_avd-home/_data
emulator -avd base-a35 -no-snapshot

# Or copy AVD to host temporarily
sudo cp -r /var/lib/docker/volumes/avdctl_avd-home/_data/base-a35.* ~/.android/avd/
emulator -avd base-a35 -no-snapshot
# ... configure (Google account, fingerprint, etc.) ...
# ... close emulator ...
sudo cp -r ~/.android/avd/base-a35.* /var/lib/docker/volumes/avdctl_avd-home/_data/
```

**Manual Configuration Steps:**
- ✅ Sign in to Google account (for Play Store)
- ✅ Enroll fingerprint (Settings → Security)
- ✅ Disable animations (Developer Options)
- ✅ Install base apps
- ✅ Configure locale/timezone
- ✅ Let emulator settle (30-60s idle)
- ✅ Close cleanly: `adb emu kill`

**Option B: Headless (No Google Account Setup)**

```bash
# Inside container: Use prewarm for automated boot+save
avdctl prewarm --name base-a35 \
  --dest /avd-golden/base-a35-prewarmed.qcow2 \
  --extra 30s \
  --timeout 3m
```

### 5. Save Golden Image (After Manual Config)

```bash
# Inside container (after Option A manual config)
avdctl save-golden --name base-a35 \
  --dest /avd-golden/base-a35-configured.qcow2
```

Check golden image on host:

```bash
# On host
docker volume inspect avdctl_avd-golden --format '{{ .Mountpoint }}'
ls -lh /var/lib/docker/volumes/avdctl_avd-golden/_data/
```

### 6. Create Customer Clones

```bash
# Inside container
avdctl clone --base base-a35 --name w-customer1 \
  --golden /avd-golden/base-a35-configured.qcow2

avdctl clone --base base-a35 --name w-customer2 \
  --golden /avd-golden/base-a35-configured.qcow2

avdctl clone --base base-a35 --name w-customer3 \
  --golden /avd-golden/base-a35-configured.qcow2
```

### 7. Run Clones in Parallel

```bash
# Inside container
avdctl run --name w-customer1 --port 5580 &
avdctl run --name w-customer2 --port 5582 &
avdctl run --name w-customer3 --port 5584 &

# Check status
sleep 10
avdctl ps
adb devices
```

### 8. Connect from Host

```bash
# On host, connect to emulator ports
adb connect localhost:5555
adb connect localhost:5557
adb connect localhost:5559

# Verify
adb devices

# Run commands
adb -s emulator-5554 shell getprop ro.build.version.release
```

### 9. Stop and Cleanup

```bash
# Inside container
avdctl stop --name w-customer1
avdctl stop --name w-customer2
avdctl stop --name w-customer3

# Or stop all
avdctl ps --json | jq -r '.[].name' | xargs -I {} avdctl stop --name {}
```

---

## Volume Management

### Access Volumes from Host

```bash
# AVD home (base + clones)
sudo ls -la $(docker volume inspect avdctl_avd-home --format '{{ .Mountpoint }}')

# Golden images
sudo ls -la $(docker volume inspect avdctl_avd-golden --format '{{ .Mountpoint }}')
```

### Backup Volumes

```bash
# Backup golden images
docker run --rm -v avdctl_avd-golden:/data -v $(pwd):/backup ubuntu \
  tar czf /backup/avd-golden-backup.tar.gz -C /data .

# Backup AVD home
docker run --rm -v avdctl_avd-home:/data -v $(pwd):/backup ubuntu \
  tar czf /backup/avd-home-backup.tar.gz -C /data .
```

### Restore Volumes

```bash
# Restore golden images
docker run --rm -v avdctl_avd-golden:/data -v $(pwd):/backup ubuntu \
  tar xzf /backup/avd-golden-backup.tar.gz -C /data

# Restore AVD home
docker run --rm -v avdctl_avd-home:/data -v $(pwd):/backup ubuntu \
  tar xzf /backup/avd-home-backup.tar.gz -C /data
```

### Clean Volumes (Danger!)

```bash
# Stop container
docker-compose down

# Remove volumes (deletes all AVDs and golden images!)
docker volume rm avdctl_avd-home avdctl_avd-golden

# Recreate
docker-compose up -d
```

---

## Advanced Usage

### Using Custom Config Template

1. Edit `config.ini.tpl` on host
2. Restart container to pick up changes:
   ```bash
   docker-compose restart
   ```

### Baking APKs

```bash
# Place APKs in ./apks/ directory on host
mkdir -p apks
cp /path/to/app.apk apks/

# Inside container
avdctl bake-apk --base base-a35 --name w-baked \
  --golden /avd-golden/base-a35-configured.qcow2 \
  --apk /apks/app.apk \
  --dest /avd-golden/base-a35-with-app.qcow2
```

### Running Multiple Compose Stacks

For isolated environments:

```bash
# Project 1
cd project1
docker-compose -p proj1 up -d

# Project 2
cd project2
docker-compose -p proj2 up -d

# Different volumes and port mappings
```

### Resource Limits

Add to `docker-compose.yml`:

```yaml
services:
  avdctl:
    deploy:
      resources:
        limits:
          cpus: '8'
          memory: 16G
        reservations:
          cpus: '4'
          memory: 8G
```

---

## Troubleshooting

### KVM Not Working

```bash
# Check if KVM is available on host
lsmod | grep kvm
ls -l /dev/kvm

# Check inside container
docker-compose exec avdctl ls -l /dev/kvm

# If permission denied
sudo chmod 666 /dev/kvm
docker-compose restart
```

### Emulator Logs

```bash
# Inside container
cat /tmp/emulator-w-customer1-5580.log

# Or from host
docker-compose exec avdctl cat /tmp/emulator-w-customer1-5580.log
```

### Port Conflicts

```bash
# Check if ports are already in use on host
lsof -i :5554-5586

# Kill conflicting processes
sudo lsof -ti :5554-5586 | xargs kill -9
```

### Build Fails

```bash
# Clean build
docker-compose down
docker-compose build --no-cache
docker-compose up -d
```

### Volume Permission Issues

```bash
# Run container as root (already default in Dockerfile.full)
# Or adjust ownership
docker-compose exec avdctl chown -R root:root /root/.android
```

### ADB Connection Issues

```bash
# Restart adb server inside container
docker-compose exec avdctl adb kill-server
docker-compose exec avdctl adb start-server

# On host, restart adb and reconnect
adb kill-server
adb start-server
adb connect localhost:5555
```

---

## Complete Docker Workflow Example

```bash
# 1. Start fresh environment
docker-compose up -d --build
docker-compose exec avdctl bash

# === Inside container ===

# 2. Create base AVD
avdctl init-base --name base-a35

# 3. Exit container
exit

# === On host ===

# 4. Boot with GUI for manual configuration
export ANDROID_AVD_HOME=$(docker volume inspect avdctl_avd-home --format '{{ .Mountpoint }}')
sudo emulator -avd base-a35 -no-snapshot
# ... configure Google account, fingerprint, etc. ...
# ... close emulator ...

# === Back in container ===

docker-compose exec avdctl bash

# 5. Save golden image
avdctl save-golden --name base-a35 \
  --dest /avd-golden/base-a35-configured.qcow2

# 6. Create 3 customer clones
for i in {1..3}; do
  avdctl clone --base base-a35 --name w-customer$i \
    --golden /avd-golden/base-a35-configured.qcow2
done

# 7. Run all in parallel
avdctl run --name w-customer1 --port 5580 &
avdctl run --name w-customer2 --port 5582 &
avdctl run --name w-customer3 --port 5584 &

# 8. Monitor
sleep 15
avdctl ps
adb devices

# 9. Test from host
exit

# === On host ===

adb connect localhost:5555
adb connect localhost:5557
adb connect localhost:5559
adb devices

# 10. Stop all
docker-compose exec avdctl bash -c "avdctl ps --json | jq -r '.[].name' | xargs -I {} avdctl stop --name {}"
```

---

## CI/CD Integration

### GitLab CI Example

```yaml
variables:
  ANDROID_AVD_HOME: /builds/avd-home
  AVDCTL_GOLDEN_DIR: /builds/avd-golden

test:
  image: avdctl:latest
  services:
    - docker:dind
  before_script:
    - chmod 666 /dev/kvm
    - adb start-server
  script:
    - avdctl clone --base base-a35 --name ci-test --golden ${AVDCTL_GOLDEN_DIR}/base-a35-configured.qcow2
    - avdctl run --name ci-test --port 5580
    - sleep 30
    - adb wait-for-device
    - ./run-tests.sh
  after_script:
    - avdctl stop --name ci-test
```

### GitHub Actions Example

```yaml
name: Android Tests

on: [push]

jobs:
  test:
    runs-on: ubuntu-latest
    
    steps:
      - uses: actions/checkout@v3
      
      - name: Enable KVM
        run: |
          sudo apt-get install -y qemu-kvm
          sudo chmod 666 /dev/kvm
      
      - name: Build Docker image
        run: docker-compose build
      
      - name: Start AVD
        run: |
          docker-compose up -d
          docker-compose exec -T avdctl avdctl clone --base base-a35 --name ci --golden /avd-golden/base.qcow2
          docker-compose exec -T avdctl avdctl run --name ci --port 5580
      
      - name: Wait for boot
        run: docker-compose exec -T avdctl adb wait-for-device
      
      - name: Run tests
        run: ./run-tests.sh
      
      - name: Stop AVD
        run: docker-compose exec -T avdctl avdctl stop --name ci
```

---

## See Also

- **README.md** - General usage documentation
- **CRUSH.md** - Development guide
- **Dockerfile.full** - Full Dockerfile with all dependencies
- **docker-compose.yml** - Compose configuration
