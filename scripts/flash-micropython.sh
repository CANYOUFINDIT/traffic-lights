#!/usr/bin/env bash
set -euo pipefail

PORT="${1:-}"
if [[ -z "$PORT" ]]; then
  echo "Usage: $0 <serial-port>"
  echo
  echo "Examples:"
  echo "  $0 /dev/cu.usbmodem1101"
  echo "  $0 COM3"
  exit 1
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

python3 -m esptool --chip esp32c3 --port "$PORT" erase-flash
python3 -m esptool --chip esp32c3 --port "$PORT" --baud 460800 \
  write-flash -z 0x0 firmware/ESP32_GENERIC_C3-20260406-v1.28.0.bin

"$ROOT/scripts/upload-firmware.sh" "$PORT"
