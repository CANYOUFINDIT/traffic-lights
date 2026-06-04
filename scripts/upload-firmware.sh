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

python3 -m mpremote connect "$PORT" fs cp firmware/main.py :main.py
python3 -m mpremote connect "$PORT" reset
