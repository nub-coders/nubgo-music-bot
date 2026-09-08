#!/bin/sh
set -e
# Fetches the NTgCalls native library (Linux x86-64) used by the CGO bindings.
# Run once after `make build` on a machine with internet access, or in the
# Docker builder stage. The .so is architecture-specific and ~120 MB, so it is
# kept out of git.
URL="${NTGCALLS_URL:-https://github.com/nub-coders/ntgcalls/releases/latest/download/libntgcalls-linux-x86_64.so}"
DIR="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$DIR/libs/lib" "$DIR/libs/include"
curl --fail --location --retry 3 --retry-delay 2 -o "$DIR/libs/lib/libntgcalls.so" "$URL"
echo "NTgCalls library installed at $DIR/libs/lib/libntgcalls.so"