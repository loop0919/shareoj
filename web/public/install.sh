#!/usr/bin/env bash
# Keep installation inside a function so a truncated download cannot run it.
main() (
  set -euo pipefail
  fail() { printf 'shareoj installer: %s\n' "$*" >&2; exit 1; }
  [[ $# -le 1 ]] || fail 'Usage: bash install.sh [cli-vX.Y.Z]'
  local platform arch version="${1:-}" repo='https://github.com/loop0919/shareoj'
  case "$(uname -s)" in
    Linux) platform=linux ;;
    Darwin) platform=darwin ;;
    *) fail 'Supported systems: Linux (including WSL) and macOS.' ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) fail 'Supported architectures: x86_64 and arm64.' ;;
  esac
  command -v curl >/dev/null || fail 'curl is required.'
  local checksum
  if command -v sha256sum >/dev/null; then
    checksum=(sha256sum)
  elif command -v shasum >/dev/null; then
    checksum=(shasum -a 256)
  else
    fail 'sha256sum or shasum is required.'
  fi
  local fetch=(curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --connect-timeout 15 --max-time 300 --retry 2)
  if [[ -z "$version" ]]; then
    local latest
    latest=$("${fetch[@]}" --output /dev/null --write-out '%{url_effective}' "$repo/releases/latest") || fail 'Cannot find the latest release. Check your connection and whether a CLI release has been published.'
    version=${latest##*/}
  fi
  [[ "$version" =~ ^cli-v[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail 'Expected a release tag such as cli-v0.1.0.'

  local dest="${SHAREOJ_INSTALL_DIR:-$HOME/.local/bin}"
  [[ "$dest" = /* ]] || fail 'SHAREOJ_INSTALL_DIR must be an absolute path.'
  [[ ! -d "$dest/shareoj" ]] || fail "$dest/shareoj is a directory."
  mkdir -p "$dest"
  local work
  work=$(mktemp -d "$dest/.shareoj-install.XXXXXX")
  trap 'rm -rf "$work"' EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM
  local asset="shareoj-$platform-$arch" base="$repo/releases/download/$version"
  printf 'Downloading ShareOJ %s (%s/%s)...\n' "$version" "$platform" "$arch"
  "${fetch[@]}" --output "$work/$asset" "$base/$asset" || fail 'Binary download failed.'
  "${fetch[@]}" --output "$work/SHA256SUMS" "$base/SHA256SUMS" || fail 'Checksum download failed.'
  local expected actual
  expected=$(awk -v name="$asset" '$2 == name { print $1 }' "$work/SHA256SUMS")
  [[ "$expected" =~ ^[0-9a-f]{64}$ ]] || fail 'Missing or invalid checksum.'
  actual=$("${checksum[@]}" "$work/$asset")
  [[ "${actual%% *}" = "$expected" ]] || fail 'Checksum mismatch; existing installation was not changed.'
  chmod 755 "$work/$asset"
  mv -f "$work/$asset" "$dest/shareoj"
  printf 'Installed %s to %s/shareoj\n' "$version" "$dest"
  case ":$PATH:" in
    *":$dest:"*) ;;
    *) printf 'Add this to your shell profile, then open a new terminal:\n  export PATH=%q:"$PATH"\n' "$dest" ;;
  esac
)
main "$@"
