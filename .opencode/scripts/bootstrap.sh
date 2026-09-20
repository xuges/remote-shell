#!/usr/bin/env bash
# Download and verify CLI binaries. Requires Bash, curl, tar and OpenSSH.
set -euo pipefail

COMMANDS=(start-remote-shell remote-shell remote-shell-info stop-remote-shell)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERSION="$(cat "$SCRIPT_DIR/VERSION")"
PREFIX="${REMOTE_SHELL_PREFIX:-$HOME/.remote-shell}"
REPO=xuges/remote-shell
log() { printf '[bootstrap] %s\n' "$*"; }
die() { printf '[bootstrap] %s\n' "$*" >&2; exit 1; }

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version|--prefix)
      [ "$#" -ge 2 ] && [ -n "$2" ] || die "$1 requires a value"
      case "$1" in --version) VERSION="$2";; --prefix) PREFIX="$2";; esac
      shift 2;;
    --version=*) VERSION="${1#*=}"; shift;;
    -h|--help)
      printf 'Usage: bootstrap.sh [--version vX.Y.Z|latest] [--prefix DIR]\n'
      exit 0;;
    *) die "Unknown argument: $1";;
  esac
done
case "$PREFIX" in /*) [ "$PREFIX" != / ] || die '--prefix must not be /';; *) die '--prefix must be absolute';; esac
INSTALL_DIR="$PREFIX/bin"
for c in ssh curl tar; do command -v "$c" >/dev/null 2>&1 || die "Missing required command: $c"; done
ssh -V >/dev/null 2>&1 || die 'OpenSSH client is not working'
if [ "$VERSION" = latest ]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
fi
[[ "$VERSION" =~ ^v[0-9][A-Za-z0-9._-]*$ ]] || die "Invalid release version: $VERSION"

all_present=1
for c in "${COMMANDS[@]}"; do
  if [ ! -x "$INSTALL_DIR/$c" ] || [ "$("$INSTALL_DIR/$c" --version 2>/dev/null)" != "$c $VERSION" ]; then
    all_present=0
    break
  fi
done
if [ "$all_present" = 1 ]; then log "Binaries ready: $INSTALL_DIR ($VERSION)"; exit 0; fi

case "$(uname -s)" in Linux) OS=linux;; Darwin) OS=darwin;; *) die 'Use bootstrap.ps1 on Windows, or install inside WSL';; esac
case "$(uname -m)" in x86_64|amd64) ARCH=amd64;; aarch64|arm64) ARCH=arm64;; *) die "Unsupported architecture: $(uname -m)";; esac
STEM="remote-shell-$VERSION-$OS-$ARCH"
ASSET="$STEM.tar.gz"
BASE="${REMOTE_SHELL_DIST_BASE_URL:-https://github.com/$REPO/releases/download/$VERSION}"
TMP="$(mktemp -d)"
STAGE=''
trap 'rm -rf "$TMP"; if [ -n "$STAGE" ]; then rm -rf "$STAGE"; fi' EXIT
log "Downloading $ASSET"
curl -fsSL "$BASE/$ASSET" -o "$TMP/$ASSET"
curl -fsSL "$BASE/sha256sums.txt" -o "$TMP/sha256sums.txt"

# Check exactly this asset. macOS shasum has no --ignore-missing option.
expected="$(awk -v asset="$ASSET" '$2 == asset || $2 == "*" asset {print $1}' "$TMP/sha256sums.txt")"
[[ "$expected" =~ ^[0-9a-fA-F]{64}$ ]] || die "Missing or ambiguous checksum for $ASSET"
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$TMP/$ASSET" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "$TMP/$ASSET" | awk '{print $1}')"
else
  die 'Install sha256sum or shasum to verify the download'
fi
[ "$(printf '%s' "$expected" | tr A-F a-f)" = "$actual" ] || die "Checksum mismatch for $ASSET; nothing installed"
tar -xzf "$TMP/$ASSET" -C "$TMP"
# Validate the whole package before replacing any installed executable.
for c in "${COMMANDS[@]}"; do
  [ -f "$TMP/$STEM/$c" ] && [ ! -L "$TMP/$STEM/$c" ] || die "Package missing executable: $c"
  chmod 0755 "$TMP/$STEM/$c"
  [ "$("$TMP/$STEM/$c" --version)" = "$c $VERSION" ] || die "Unexpected executable version: $c"
done
mkdir -p "$INSTALL_DIR"
STAGE="$(mktemp -d "$INSTALL_DIR/.install.XXXXXX")"
for c in "${COMMANDS[@]}"; do cp "$TMP/$STEM/$c" "$STAGE/$c"; done
for c in "${COMMANDS[@]}"; do mv -f "$STAGE/$c" "$INSTALL_DIR/$c"; done
log "Installed $VERSION into $INSTALL_DIR"
