#!/usr/bin/env bash
# Fetch/build inputs on the developer machine (GitHub is not assumed IPv6-capable).
set -euo pipefail
cd "$(dirname "$0")"
: "${ISOLATE_COMMIT:=8f185bb37f3f23e29b33b0c7727c91c13429abe3}"
[[ "$ISOLATE_COMMIT" =~ ^[a-f0-9]{40}$ ]]
build_dir=$(mktemp -d)
trap 'rm -rf "$build_dir"' EXIT
curl --fail --location --proto '=https' --tlsv1.2 \
  "https://codeload.github.com/ioi/isolate/tar.gz/$ISOLATE_COMMIT" -o "$build_dir/isolate.tar.gz"
printf '%s\n' "$ISOLATE_COMMIT" > "$build_dir/isolate-commit"
cp host.py sandbox.py interactive.py interactive_smoke.py testlib_smoke.py worker.py telemetry.py pool.py smoke.py install.sh fingerprint.py smoke.sh judge-worker.service runtimes.py language-smoke.json "$build_dir/"
: "${RUNTIME_ARCHIVE:=.build/runtime-additions.tar.gz}"
cp "$RUNTIME_ARCHIVE" "$build_dir/runtime.tar.gz"
mkdir -p .build
# The large runtime archive is already compressed.
tar -C "$build_dir" -I 'gzip -1' -cf .build/worker.tar.gz .
sha256sum .build/worker.tar.gz
