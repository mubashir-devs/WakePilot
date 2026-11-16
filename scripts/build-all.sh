#!/usr/bin/env sh
# Cross-compiles WakePilot for Linux, macOS and Windows.
# Usage: scripts/build-all.sh [version]   (run `npm run build` in frontend/ first)
set -eu
cd "$(dirname "$0")/.."
VERSION="${1:-dev}"
[ -f frontend/dist/index.html ] || { echo "frontend/dist/index.html missing: build the frontend first" >&2; exit 1; }
export CGO_ENABLED=0   # pure-Go SQLite driver: no C toolchain needed
mkdir -p dist-bin
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
  os="${target%/*}"; arch="${target#*/}"
  ext=""; [ "$os" = "windows" ] && ext=".exe"
  out="dist-bin/wakepilot-${os}-${arch}${ext}"
  echo "building $out"
  GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o "$out" ./cmd/server
done
ls -lh dist-bin
