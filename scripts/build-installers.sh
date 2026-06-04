#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

mkdir -p dist
rm -f \
  dist/CodexTrafficLightInstaller-windows-amd64.exe \
  dist/CodexTrafficLightInstaller-windows-386.exe \
  dist/CodexTrafficLightInstaller-windows-x86_64.exe \
  dist/CodexTrafficLightInstaller-windows-x86_32.exe

go test ./...

go build -trimpath -ldflags="-s -w" \
  -o dist/CodexTrafficLightInstaller-macos-arm64 \
  ./cmd/codex-traffic-light

GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" \
  -o dist/CodexTrafficLightInstaller-windows-x86_64.exe \
  ./cmd/codex-traffic-light

GOOS=windows GOARCH=386 go build -trimpath -ldflags="-s -w" \
  -o dist/CodexTrafficLightInstaller-windows-x86_32.exe \
  ./cmd/codex-traffic-light

cp packaging/README-macos-arm64-zh.txt dist/README-macos-arm64-zh.txt
cp packaging/README-windows-zh.txt dist/README-windows-zh.txt

chmod +x dist/CodexTrafficLightInstaller-macos-arm64
if command -v codesign >/dev/null 2>&1; then
  codesign --force --sign - dist/CodexTrafficLightInstaller-macos-arm64
fi

rm -f \
  dist/CodexTrafficLightInstaller-macos-arm64.zip \
  dist/CodexTrafficLightInstaller-windows-amd64.zip \
  dist/CodexTrafficLightInstaller-windows-386.zip \
  dist/CodexTrafficLightInstaller-windows-x86_64.zip \
  dist/CodexTrafficLightInstaller-windows-x86_32.zip

(cd dist && zip -9 CodexTrafficLightInstaller-macos-arm64.zip \
  CodexTrafficLightInstaller-macos-arm64 README-macos-arm64-zh.txt)

(cd dist && zip -9 CodexTrafficLightInstaller-windows-x86_64.zip \
  CodexTrafficLightInstaller-windows-x86_64.exe README-windows-zh.txt)

(cd dist && zip -9 CodexTrafficLightInstaller-windows-x86_32.zip \
  CodexTrafficLightInstaller-windows-x86_32.exe README-windows-zh.txt)

shasum -a 256 \
  dist/CodexTrafficLightInstaller-macos-arm64.zip \
  dist/CodexTrafficLightInstaller-windows-x86_64.zip \
  dist/CodexTrafficLightInstaller-windows-x86_32.zip
