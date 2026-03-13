---
name: software_inventory
summary: >
  Install, track, and verify system software. All tools are managed through
  a single idempotent setup script. Load when you need a new binary, want
  to check what's available, or a command is missing.
tools: [shell]
---

# Software Inventory

All installed software is tracked in a single setup script: `scripts/setup.sh`.
This is the source of truth. Never install tools ad-hoc -- always update the
script so installs are reproducible.

## If `scripts/setup.sh` doesn't exist

Create it with this template:

```bash
#!/usr/bin/env bash
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

log() { printf '[setup] %s\n' "$*"; }

have_cmd() { command -v "$1" >/dev/null 2>&1; }

apt_install() {
  local cmd="$1"; shift
  have_cmd "$cmd" && { log "$cmd ok"; return 0; }
  apt-get update -y && apt-get install -y "$@"
  log "$cmd installed"
}

npm_install() {
  local cmd="$1" pkg="$2"
  have_cmd "$cmd" && { log "$cmd ok"; return 0; }
  npm install -g "$pkg"
  log "$cmd installed"
}

pip_install() {
  local cmd="$1" pkg="$2"
  have_cmd "$cmd" && { log "$cmd ok"; return 0; }
  python3 -m pip install --user "$pkg"
  log "$cmd installed"
}

go_install() {
  local cmd="$1" pkg="$2"
  have_cmd "$cmd" && { log "$cmd ok"; return 0; }
  go install "$pkg"
  log "$cmd installed"
}

# --- system packages ---


# --- node packages ---


# --- python packages ---


# --- go packages ---


# --- persist ---
if [ -f /.dockerenv ] && have_cmd docker; then
  log "persisting container state"
  docker commit "$(hostname)" nik:latest 2>/dev/null || true
fi

log "done"
```

Customize the sections as needed. Each section should use the matching
helper.

## Adding software

1. Open `scripts/setup.sh` and add a line in the right section, e.g.
   `apt_install jq jq` or `npm_install gws @googleworkspace/cli`.
2. Run: `bash scripts/setup.sh`
3. On Docker, the script auto-commits. On bare metal, no extra step needed.

## Verifying

Run the script. It's idempotent -- prints status for every tool and only
installs what's missing.

## Removing software

Remove the line from the script. On the next clean rebuild (or manual
uninstall), the tool won't be reinstalled.
