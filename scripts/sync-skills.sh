#!/usr/bin/env bash
# Syncs the canonical skills (plugins/skills-canonical) and install scripts
# (plugins/bin) into the three per-host plugin directories plus the repo's own
# .opencode/skills. Copy complete skill trees so each package is self-contained.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_SKILLS="$ROOT/plugins/skills-canonical"
SRC_BIN="$ROOT/plugins/bin"

DESTINATIONS=(
  "$ROOT/plugins/codex/remote-shell"
  "$ROOT/plugins/claude/remote-shell"
  "$ROOT/plugins/opencode/remote-shell"
  "$ROOT/.opencode"
)

for dest in "${DESTINATIONS[@]}"; do
  rm -rf "$dest/skills"
  mkdir -p "$dest/skills" "$dest/scripts"
  cp -r "$SRC_SKILLS"/. "$dest/skills/"
  cp "$SRC_BIN/bootstrap.sh" "$SRC_BIN/bootstrap.ps1" "$SRC_BIN/VERSION" "$dest/scripts/"
done

echo "synced canonical skills + install scripts into:"
for dest in "${DESTINATIONS[@]}"; do
  echo "  $dest"
done
echo "synced binary version $(cat "$SRC_BIN/VERSION")"
