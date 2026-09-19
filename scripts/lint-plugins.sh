#!/usr/bin/env bash
# Validates the plugin tree: frontmatter, naming, and that every copy matches
# the canonical sources.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="$ROOT/plugins/skills-canonical"
fail=0

check_frontmatter() {
  local f="$1" name="$2"
  [ -f "$f" ] || { echo "lint-error: missing $f"; fail=1; return; }
  grep -q '^---$' "$f" || { echo "lint-error: $f has no frontmatter"; fail=1; }
  grep -q "^name: $name$" "$f" || { echo "lint-error: $f frontmatter name != $name"; fail=1; }
  grep -q '^description:' "$f" || { echo "lint-error: $f missing description"; fail=1; }
}
check_frontmatter "$SRC/remote-shell/SKILL.md" remote-shell
check_frontmatter "$SRC/remote-computer-use/SKILL.md" remote-computer-use

expect="$ROOT/plugins/codex/remote-shell/skills:$ROOT/plugins/claude/remote-shell/skills:$ROOT/plugins/opencode/remote-shell/skills:$ROOT/.opencode/skills"
for dir in ${expect//:/ }; do
  for s in remote-shell remote-computer-use; do
    diff -r "$SRC/$s" "$dir/$s" >/dev/null 2>&1 \
      || { echo "lint-error: $dir/$s differs from canonical"; fail=1; }
  done
  for f in bootstrap.sh bootstrap.ps1 VERSION; do
    diff "$SRC/../bin/$f" "$dir/../scripts/$f" >/dev/null 2>&1 \
      || { echo "lint-error: $dir/../scripts/$f differs from canonical"; fail=1; }
  done
done

# manifests parse as JSON
for m in \
  "$ROOT/plugins/codex/remote-shell/plugin.json" \
  "$ROOT/plugins/claude/remote-shell/.claude-plugin/plugin.json" \
  "$ROOT/.claude-plugin/marketplace.json"; do
  python3 -c "import json,sys; json.load(open('$m'))" \
    || { echo "lint-error: invalid JSON in $m"; fail=1; }
done

[ "$fail" = 0 ] || { echo "plugin lint FAILED"; exit 1; }
echo "plugin lint OK: skills synced, frontmatter valid, manifests valid"