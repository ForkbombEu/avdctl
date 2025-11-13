#!/usr/bin/env bash
set -euo pipefail
NAME="${1:-}"
if [ -z "$NAME" ]; then echo "usage: $0 <avd-name>"; exit 1; fi
# Start GUI emulator for manual customization with snapshots disabled
QT_QPA_PLATFORM="${QT_QPA_PLATFORM:-xcb}" ./bin/avdctl customize-start --name "$NAME"