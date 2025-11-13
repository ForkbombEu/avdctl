#!/usr/bin/env bash
set -euo pipefail
NAME="${1:-}"
DEST="${2:-}"
if [ -z "$NAME" ]; then echo "usage: $0 <avd-name> [dest-dir]"; exit 1; fi
if [ -z "$DEST" ]; then DEST="$HOME/avd-golden/${NAME}-custom"; fi
# Stop emulator if running and export all writable images to golden directory
./bin/avdctl customize-finish --name "$NAME" --dest "$DEST"
echo "Golden saved to: $DEST"
ls -lh "$DEST/"