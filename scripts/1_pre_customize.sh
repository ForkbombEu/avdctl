#!/usr/bin/env bash
set -euo pipefail

# Hardcoded configuration
BASE_NAME="base-customizable"
SYSTEM_IMAGE="system-images;android-35;google_apis_playstore;x86_64"
DEVICE="pixel_6"

echo "==> Step 1: Creating base AVD '$BASE_NAME'..."
./bin/avdctl init-base --name "$BASE_NAME" --image "$SYSTEM_IMAGE" --device "$DEVICE" || {
    echo "Base AVD already exists or failed to create"
}

echo ""
echo "==> Step 2: Preparing for customization (disabling snapshots)..."
AVD_DIR="$HOME/.android/avd/${BASE_NAME}.avd"
CONFIG="$AVD_DIR/config.ini"

# Disable snapshots
sed -i 's/^QuickBoot.mode.*/QuickBoot.mode=disabled/' "$CONFIG" || echo "QuickBoot.mode=disabled" >> "$CONFIG"
sed -i 's/^snapshot.present.*/snapshot.present=false/' "$CONFIG" || echo "snapshot.present=false" >> "$CONFIG"
grep -q "fastboot.forceColdBoot" "$CONFIG" || echo "fastboot.forceColdBoot=yes" >> "$CONFIG"
grep -q "userdata.useQcow2" "$CONFIG" || echo "userdata.useQcow2=yes" >> "$CONFIG"

# Remove snapshots
rm -rf "$AVD_DIR/snapshots"

echo ""
echo "==> Step 3: Starting emulator for customization..."
echo "    GUI will open. Apply your customizations:"
echo "    - Set wallpaper"
echo "    - Sign in to Google account"
echo "    - Enroll fingerprint"
echo "    - Install apps"
echo "    - Configure settings"
echo ""
echo "    When done, run: adb emu kill"
echo ""

QT_QPA_PLATFORM="${QT_QPA_PLATFORM:-xcb}" QEMU_FILE_LOCKING=off emulator -avd "$BASE_NAME" -no-snapshot-load -no-snapshot-save &
EMU_PID=$!

echo "Emulator started (PID: $EMU_PID)"
echo ""
echo "==> Waiting for you to customize and kill the emulator..."
echo "    Run 'adb emu kill' when done, then run ./scripts/2_post_customize.sh"
