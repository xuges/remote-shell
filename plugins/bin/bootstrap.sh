#!/usr/bin/env bash
# remote-shell binary installer.
#
# Downloads the prebuilt release binaries for this platform, verifies them
# against the published sha256 checksums, and installs them into
# ~/.remote-shell/bin. No Go toolchain is required.
#
# Usage:
#   bootstrap.sh [--version vX.Y.Z|latest] [--source-build] [--prefix DIR]
#
# Environment overrides (mainly for testing):
#   REMOTE_SHELL_DIST_BASE_URL  base URL for assets (default: GitHub releases)
set -euo pipefail

COMMANDS="start-remote-shell remote-shell remote-shell-info stop-remote-shell"
DEFAULT_VERSION="$(cat "$(dirname "$(realpath "${BASH_SOURCE[0]}")")/VERSION")"
GITHUB_REPO="xuges/remote-shell"
PREFIX="${REMOTE_SHELL_PREFIX:-$HOME/.remote-shell}"
INSTALL_DIR="$PREFIX/bin"

log()  { printf '\033[1;32m[bootstrap]\033[0m %s\n' "$*"; }
err()  { printf '\033[1;31m[bootstrap]\033[0m %s\n' "$*" >&2; }

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    return 1
  fi
}
checksum_apply() {
  local file="$1" sums="$2"
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$(dirname "$file")" && sha256sum -c "$sums" --ignore-missing >/dev/null 2>&1)
  elif command -v shasum >/dev/null 2>&1; then
    (cd "$(dirname "$file")" && shasum -a 256 -c "$sums" --ignore-missing >/dev/null 2>&1)
  else
    return 99
  fi
}

VERSION="$DEFAULT_VERSION"
SOURCE_BUILD=0
while [ $# -gt 0 ]; do
  case "$1" in
    --version) VERSION="${2:?--version needs a value}"; shift 2;;
    --version=*) VERSION="${1#*=}"; shift;;
    --source-build) SOURCE_BUILD=1; shift;;
    --prefix) PREFIX="${2:?--prefix needs a value}"; INSTALL_DIR="$PREFIX/bin"; shift 2;;
    -h|--help) sed -n '2,14p' "${BASH_SOURCE[0]}"; exit 0;;
    *) err "unknown argument: $1"; exit 2;;
  esac
done

# 1. Nothing to do if the pinned binaries are already present and matching
# (either on PATH or in the install directory).
if [ "$SOURCE_BUILD" != 1 ]; then
  installed() { command -v "$1" >/dev/null 2>&1 || [ -x "$INSTALL_DIR/$1" ]; }
  all_present=1
  for c in $COMMANDS; do
    installed "$c" || { all_present=0; break; }
  done
  if [ "$all_present" = 1 ]; then
    for c in $COMMANDS; do
      if command -v "$c" >/dev/null 2>&1; then bin="$c"; else bin="$INSTALL_DIR/$c"; fi
      if ! "$bin" -version 2>/dev/null | grep -q "^$c $VERSION"; then
        log "$c present but version != $VERSION; reinstalling"
        all_present=0
        break
      fi
    done
  fi
  [ "$all_present" = 1 ] && { log "binaries already installed ($VERSION). Done."; exit 0; }
fi

require_cmd ssh && ssh -V >/dev/null 2>&1 || {
  err "OpenSSH client not found on PATH. Install OpenSSH 8.9+ first."
  exit 1
}

# 2. Figure out the platform asset name.
case "$(uname -s)" in
  Linux)  OS=linux;;
  Darwin) OS=darwin;;
  *)      err "unsupported OS: $(uname -s)"; exit 1;;
esac
case "$(uname -m)" in
  x86_64|amd64)   ARCH=amd64;;
  aarch64|arm64)  ARCH=arm64;;
  *)              err "unsupported arch: $(uname -m)"; exit 1;;
esac
if [ "$VERSION" = "latest" ]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/$GITHUB_REPO/releases/latest" | sed -n 's/.*"tag_name": "\([^"]*\)".*/\1/p' | head -1)"
  [ -n "$VERSION" ] || { err "could not resolve latest version"; exit 1; }
fi

DIST_BASE="${REMOTE_SHELL_DIST_BASE_URL:-https://github.com/$GITHUB_REPO/releases/download/$VERSION}"
STEM="remote-shell-$VERSION-$OS-$ARCH"
EXT=tar.gz
PACKAGE="$DIST_BASE/$STEM.$EXT"
SUMS_URL="$DIST_BASE/sha256sums.txt"

log "installing $STEM from $DIST_BASE into $INSTALL_DIR"
mkdir -p "$INSTALL_DIR"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

curl -fsSL "$PACKAGE" -o "$TMP/$STEM.$EXT"
curl -fsSL "$SUMS_URL" -o "$TMP/sha256sums.txt"

if ! checksum_apply "$TMP/$STEM.$EXT" "$TMP/sha256sums.txt"; then
  err "checksum mismatch for $STEM.$EXT — refusing to install. Do not disable verification."
  exit 1
fi

tar -xzf "$TMP/$STEM.$EXT" -C "$TMP"
for c in $COMMANDS; do
  chmod 0755 "$TMP/$STEM/$c"
  cp "$TMP/$STEM/$c" "$INSTALL_DIR/$c"
done
log "installed $VERSION into $INSTALL_DIR"

if ! printf '%s' "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
  log "add to your shell rc, e.g.: export PATH=\"$INSTALL_DIR:\$PATH\""
fi