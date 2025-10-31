#!/bin/bash
# Copyright (C) 2025 Forkbomb B.V.
# License: AGPL-3.0-only

set -e

# Check if /dev/kvm is accessible
if [ ! -e /dev/kvm ]; then
    echo "ERROR: /dev/kvm not found. Cannot run hardware-accelerated emulator."
    echo "Make sure to run container with --device /dev/kvm"
    exit 1
fi

if [ ! -r /dev/kvm ] || [ ! -w /dev/kvm ]; then
    echo "ERROR: /dev/kvm is not readable/writable."
    echo "Run: sudo chmod 666 /dev/kvm"
    echo "Or add user to kvm group: sudo usermod -aG kvm $USER"
    exit 1
fi

# Start adb server
adb start-server 2>/dev/null || true

# Execute the command
exec "$@"
