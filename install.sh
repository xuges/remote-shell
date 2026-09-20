#!/usr/bin/env bash
# Install both skills and the verified CLI binaries for local AI clients.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: bash install.sh [--agent auto|codex|opencode|claude-code|all] [options]

  --agent NAME    Client to install for (default: detect installed clients)
  --prefix DIR    Binary directory parent (default: ~/.remote-shell)
  --version TAG   Binary release (default: bundled VERSION; also accepts latest)
  --ref REF       GitHub source ref when downloading skills (default: main)
  --source DIR    Install skills and bootstrap from a local checkout
  -h, --help      Show this help

Installs both remote-shell and remote-computer-use. Existing changed skill
folders are backed up. Connection config and shell startup files are untouched.
EOF
}

log() { printf '[install] %s\n' "$*"; }
die() { printf '[install] %s\n' "$*" >&2; exit 1; }

main() {
  local agent=auto prefix="${REMOTE_SHELL_PREFIX:-$HOME/.remote-shell}"
  local version='' ref=main source='' script="${BASH_SOURCE[0]:-}"
  local arg client skill dest stage backup_root='' tmp
  local -a clients=() destinations=()
  while [ "$#" -gt 0 ]; do
    arg="$1"
    case "$arg" in
      -h|--help) usage; return;;
      --agent|--prefix|--version|--ref|--source)
        [ "$#" -ge 2 ] && [ -n "$2" ] || die "$arg requires a value"
        case "$arg" in
          --agent) agent="$2";;
          --prefix) prefix="$2";;
          --version) version="$2";;
          --ref) ref="$2";;
          --source) source="$2";;
        esac
        shift 2;;
      *) die "Unknown argument: $arg (see --help)";;
    esac
  done
  case "$prefix" in /*) [ "$prefix" != / ] || die '--prefix must not be /';; *) die '--prefix must be an absolute path';; esac
  case "$agent" in
    auto)
      if command -v codex >/dev/null 2>&1 || [ -d "${CODEX_HOME:-$HOME/.codex}" ]; then clients+=(codex); fi
      if command -v opencode >/dev/null 2>&1 || [ -d "${XDG_CONFIG_HOME:-$HOME/.config}/opencode" ]; then clients+=(opencode); fi
      if command -v claude >/dev/null 2>&1 || [ -d "${CLAUDE_CONFIG_DIR:-$HOME/.claude}" ]; then clients+=(claude-code); fi
      [ -n "${clients[*]-}" ] || die 'No client detected. Use --agent codex, opencode, claude-code, or all.';;
    all) clients=(codex opencode claude-code);;
    codex|opencode|claude-code) clients=("$agent");;
    *) die "Unknown agent: $agent";;
  esac
  for client in "${clients[@]}"; do
    case "$client" in
      codex) destinations+=("$HOME/.agents/skills");;
      opencode) destinations+=("${XDG_CONFIG_HOME:-$HOME/.config}/opencode/skills");;
      claude-code) destinations+=("${CLAUDE_CONFIG_DIR:-$HOME/.claude}/skills");;
    esac
  done
  for dest in "${destinations[@]}"; do
    case "$dest" in /*) ;; *) die "Client config directory must be absolute: $dest";; esac
  done

  for arg in curl tar ssh; do command -v "$arg" >/dev/null 2>&1 || die "Missing required command: $arg"; done
  tmp="$(mktemp -d)"
  # Keep cleanup variables global: EXIT runs after main's local scope ends.
  INSTALL_TMP="$tmp"
  INSTALL_STAGE=''
  trap 'rm -rf "$INSTALL_TMP"; if [ -n "$INSTALL_STAGE" ]; then rm -rf "$INSTALL_STAGE"; fi' EXIT

  if [ -z "$source" ] && [ -n "$script" ] && [ -f "$script" ]; then
    dest="$(cd "$(dirname "$script")" && pwd)"
    if [ -f "$dest/plugins/skills-canonical/remote-shell/SKILL.md" ]; then source="$dest"; fi
  fi
  if [ -z "$source" ]; then
    log "Downloading skills from xuges/remote-shell ($ref)"
    curl --fail --location --silent --show-error \
      "${REMOTE_SHELL_SOURCE_URL:-https://github.com/xuges/remote-shell/archive/$ref.tar.gz}" -o "$tmp/source.tar.gz"
    mkdir "$tmp/source"
    tar -xzf "$tmp/source.tar.gz" -C "$tmp/source" --strip-components=1
    source="$tmp/source"
  fi
  for skill in remote-shell remote-computer-use; do
    [ -f "$source/plugins/skills-canonical/$skill/SKILL.md" ] || die "Missing skill in source: $skill"
  done
  [ -f "$source/plugins/bin/bootstrap.sh" ] || die 'Missing binary installer in source'
  [ -f "$source/examples/config.toml" ] || die 'Missing example config in source'

  if [ -n "$version" ]; then
    bash "$source/plugins/bin/bootstrap.sh" --prefix "$prefix" --version "$version"
  else
    bash "$source/plugins/bin/bootstrap.sh" --prefix "$prefix"
  fi

  for dest in "${destinations[@]}"; do
    mkdir -p "$dest"
    for skill in remote-shell remote-computer-use; do
      if [ ! -L "$dest/$skill" ] && [ -d "$dest/$skill" ] && diff -qr "$source/plugins/skills-canonical/$skill" "$dest/$skill" >/dev/null 2>&1; then
        log "Already current: $dest/$skill"
        continue
      fi
      stage="$(mktemp -d "$dest/.remote-shell-install.XXXXXX")"
      INSTALL_STAGE="$stage"
      cp -R "$source/plugins/skills-canonical/$skill" "$stage/$skill"
      if [ -e "$dest/$skill" ] || [ -L "$dest/$skill" ]; then
        if [ -z "$backup_root" ]; then
          mkdir -p "$prefix/backups"
          backup_root="$(mktemp -d "$prefix/backups/skills.XXXXXX")"
        fi
        mkdir -p "$backup_root$dest"
        mv "$dest/$skill" "$backup_root$dest/$skill"
      fi
      if ! mv "$stage/$skill" "$dest/$skill"; then
        if [ -n "$backup_root" ] && { [ -e "$backup_root$dest/$skill" ] || [ -L "$backup_root$dest/$skill" ]; }; then
          mv "$backup_root$dest/$skill" "$dest/$skill"
        fi
        die "Could not install $dest/$skill"
      fi
      rmdir "$stage"
      INSTALL_STAGE=''
      log "Installed: $dest/$skill"
    done
  done
  if [ ! -e "$prefix/config.example.toml" ] && [ ! -L "$prefix/config.example.toml" ]; then
    (umask 077; cp "$source/examples/config.toml" "$prefix/config.example.toml")
  fi
  for arg in start-remote-shell remote-shell remote-shell-info stop-remote-shell; do
    "$prefix/bin/$arg" --version
  done
  [ -z "$backup_root" ] || log "Previous skills saved in: $backup_root"
  log 'Done. Open a new AI session and ask it to use remote-shell.'
  log "Connection template: $prefix/config.example.toml"
  printf '[install] For commands in this terminal: export PATH=%q/bin:"$PATH"\n' "$prefix"
  if [ "$prefix" != "$HOME/.remote-shell" ]; then
    printf '[install] Set this in your AI environment: export REMOTE_SHELL_PREFIX=%q\n' "$prefix"
  fi
}

main "$@"
