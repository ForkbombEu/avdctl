#!/usr/bin/env bash
set -euo pipefail

# Hardcoded configuration
BASE_NAME="base-customizable"
CLONE_NAME="customer-clone"
GOLDEN_DIR="$HOME/avd-golden/${BASE_NAME}-full"

echo "==> Step 1: Waiting for emulator to fully stop..."
sleep 3

echo ""
echo "==> Step 2: Creating golden image (flattening to raw IMG format)..."
mkdir -p "$GOLDEN_DIR"

BASE_AVD="$HOME/.android/avd/${BASE_NAME}.avd"

# Convert to raw IMG format (not qcow2) - emulator can't convert these to overlays
echo "    Converting userdata to raw IMG (preserves all customizations)..."
qemu-img convert -O raw "$BASE_AVD/userdata-qemu.img.qcow2" "$GOLDEN_DIR/userdata-qemu.img" || {
    echo "ERROR: Failed to convert userdata"
    exit 1
}

echo "    Converting encryptionkey to raw IMG (Google account, fingerprint)..."
qemu-img convert -O raw "$BASE_AVD/encryptionkey.img.qcow2" "$GOLDEN_DIR/encryptionkey.img" || {
    echo "ERROR: Failed to convert encryptionkey"
    exit 1
}

echo "    Converting cache to raw IMG..."
qemu-img convert -O raw "$BASE_AVD/cache.img.qcow2" "$GOLDEN_DIR/cache.img" || {
    echo "WARNING: Failed to convert cache (optional)"
}

echo ""
echo "Golden image saved to: $GOLDEN_DIR"
ls -lh "$GOLDEN_DIR"

echo ""
echo "==> Step 3: Creating clone '$CLONE_NAME'..."
./bin/avdctl delete "$CLONE_NAME" 2>/dev/null || true

CLONE_AVD="$HOME/.android/avd/${CLONE_NAME}.avd"
mkdir -p "$CLONE_AVD"

# Copy config and disable qcow2 (force raw IMG usage)
echo "    Copying and updating config.ini..."
cp "$BASE_AVD/config.ini" "$CLONE_AVD/"
sed -i 's/^disk.dataPartition.initPath.*//' "$CLONE_AVD/config.ini"
sed -i 's/^userdata.useQcow2.*/userdata.useQcow2=no/' "$CLONE_AVD/config.ini"
grep -q "userdata.useQcow2" "$CLONE_AVD/config.ini" || echo "userdata.useQcow2=no" >> "$CLONE_AVD/config.ini"

# Copy the raw IMG files
echo "    Copying raw IMG files..."
cp "$GOLDEN_DIR/userdata-qemu.img" "$CLONE_AVD/"
cp "$GOLDEN_DIR/encryptionkey.img" "$CLONE_AVD/"
[ -f "$GOLDEN_DIR/cache.img" ] && cp "$GOLDEN_DIR/cache.img" "$CLONE_AVD/"

# Symlink read-only system files from base
echo "    Symlinking system files..."
cd "$BASE_AVD"
for file in *.img; do
    # Skip writable files
    if [[ ! "$file" =~ (userdata|cache|encryptionkey) ]]; then
        ln -sf "$BASE_AVD/$file" "$CLONE_AVD/$file"
    fi
done

# Symlink data directory if exists
if [ -d "$BASE_AVD/data" ]; then
    ln -sf "$BASE_AVD/data" "$CLONE_AVD/data"
fi

# Create .ini file
echo "    Creating ${CLONE_NAME}.ini..."
cat > "$HOME/.android/avd/${CLONE_NAME}.ini" <<EOF
avd.ini.encoding=UTF-8
path=$CLONE_AVD
path.rel=avd/${CLONE_NAME}.avd
EOF

echo ""
echo "Clone created: $CLONE_AVD"
ls -lh "$CLONE_AVD"

echo ""
echo "==> Step 4: Starting clone emulator..."
echo "    Using raw IMG files (not qcow2) to preserve customizations"
echo ""

QT_QPA_PLATFORM="${QT_QPA_PLATFORM:-xcb}" QEMU_FILE_LOCKING=off emulator -avd "$CLONE_NAME" -no-snapshot-load -no-snapshot-save &
EMU_PID=$!

echo "Emulator started (PID: $EMU_PID)"
echo ""
echo "==> Done! Customizations should NOW be present (wallpaper, Google account, fingerprint)."
echo ""
echo "Clone details:"
echo "  Name: $CLONE_NAME"
echo "  Path: $CLONE_AVD"
echo "  Golden: $GOLDEN_DIR"
echo "  Format: RAW IMG (not qcow2)"
