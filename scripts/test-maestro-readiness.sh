#!/bin/bash
# Test script to verify AVD clones work with Maestro-like automation
# This simulates the workflow that was failing: clone → boot → connect → verify

set -e

CLONE_NAME="${1:-test-clone}"
BASE_NAME="${2:-credimi}"
GOLDEN_PATH="${3:-$HOME/avd-golden/credimi-golden}"
PORT=5580

echo "==> Testing AVD clone readiness for Maestro"
echo "    Clone: $CLONE_NAME"
echo "    Base: $BASE_NAME"
echo "    Golden: $GOLDEN_PATH"
echo ""

# Clean up any existing test clone
echo "[1/6] Cleaning up existing clone..."
./bin/avdctl delete "$CLONE_NAME" 2>/dev/null || true

# Create clone (should be instant with QCOW2 overlays)
echo "[2/6] Creating clone (expecting <1s)..."
time ./bin/avdctl clone --base "$BASE_NAME" --name "$CLONE_NAME" --golden "$GOLDEN_PATH"

# Start emulator
echo "[3/6] Starting emulator on port $PORT..."
./bin/avdctl run --name "$CLONE_NAME" --port "$PORT" &
sleep 5

# Wait for ADB connection (max 60s)
echo "[4/6] Waiting for ADB connection..."
TIMEOUT=60
ELAPSED=0
while [ $ELAPSED -lt $TIMEOUT ]; do
    if adb devices | grep -q "emulator-$PORT.*device"; then
        echo "✓ ADB connected after ${ELAPSED}s"
        break
    fi
    sleep 2
    ELAPSED=$((ELAPSED + 2))
done

if [ $ELAPSED -ge $TIMEOUT ]; then
    echo "✗ ADB connection timeout after ${TIMEOUT}s"
    adb devices
    ./bin/avdctl stop --name "$CLONE_NAME"
    exit 1
fi

# Wait for boot completion
echo "[5/6] Waiting for boot completion..."
BOOT_TIMEOUT=90
BOOT_ELAPSED=0
while [ $BOOT_ELAPSED -lt $BOOT_TIMEOUT ]; do
    BOOT_STATUS=$(adb -s "emulator-$PORT" shell getprop sys.boot_completed 2>/dev/null || echo "0")
    if [ "$BOOT_STATUS" = "1" ]; then
        echo "✓ Boot completed after ${BOOT_ELAPSED}s"
        break
    fi
    sleep 3
    BOOT_ELAPSED=$((BOOT_ELAPSED + 3))
done

if [ $BOOT_ELAPSED -ge $BOOT_TIMEOUT ]; then
    echo "✗ Boot timeout after ${BOOT_TIMEOUT}s"
    ./bin/avdctl ps --json
    ./bin/avdctl stop --name "$CLONE_NAME"
    exit 1
fi

# Verify emulator status
echo "[6/6] Verifying emulator status..."
./bin/avdctl ps --json | jq -e ".[] | select(.serial == \"emulator-$PORT\" and .booted == true)" > /dev/null
if [ $? -eq 0 ]; then
    echo "✓ Emulator fully booted and ready"
else
    echo "✗ Emulator not in expected state"
    ./bin/avdctl ps --json
    ./bin/avdctl stop --name "$CLONE_NAME"
    exit 1
fi

# Stop emulator
echo ""
echo "==> Test PASSED! Stopping emulator..."
./bin/avdctl stop --name "$CLONE_NAME"

echo ""
echo "✓ Clone is Maestro-ready:"
echo "  - Clone creation: <1s (QCOW2 overlays)"
echo "  - ADB connection: Working"
echo "  - Boot completion: Working"
echo "  - Device status: online (not offline)"
