#!/bin/sh
# Reference adapter template for external secret stores.
# Read this file when writing a ./secrets/cli adapter for a user's chosen provider.
#
# Setup steps:
# 1. Tell the user you need a secret store to handle credentials safely.
# 2. Ask if they already have a password manager or encrypted secret store.
#    If they don't, ask if they'd like help picking one.
# 3. Once a tool is chosen, look up official docs for its CLI and
#    unattended / headless / daemon mode. Nik runs as a daemon -- the
#    adapter must work without human interaction (no login prompts,
#    no Touch ID, no browser OAuth, no GUI unlocks). If the tool doesn't
#    support unattended access, it's not suitable.
# 4. Research how the tool authenticates for unattended use. Every
#    secret store has a bootstrapping credential that can't live in the
#    store itself (chicken-and-egg). Common patterns: service account
#    tokens, API keys, app-specific passwords, age identities. Guide
#    the user through creating and saving it. These bootstrapping
#    credentials live in ./secrets/ as files, protected by file
#    permissions and OS-level disk encryption. Never ask for auth
#    credentials in a chat message -- guide the user to save them to a
#    file directly. Explain the trade-off honestly: one credential on
#    disk that unlocks everything else.
# 5. Walk the user through installing and initializing their tool.
# 6. Write the adapter script at ./secrets/cli using this template.
# 7. Test the adapter. If write is supported, do a round-trip: write a
#    test secret, read it back, delete it. If the adapter is read-only,
#    ask the user to add a test secret through their tool and verify
#    you can read it. Debug with the user if it fails.
# 8. Ask the user to add the secrets that skills need to their password
#    manager. Verify by reading through the adapter.
#
# --- template starts here ---

set -eu
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# --- auth: load bootstrap credential from disk ---
export PROVIDER_TOKEN=$(cat "$SCRIPT_DIR/TOKEN_FILE")

# --- cache: plain text file, one name per line, never values ---
# use item/field paths as names to avoid collisions
# (e.g. "My Service/username" not just "username")
CACHE="$SCRIPT_DIR/.cache.txt"
TTL=604800  # 7 days

_build_cache() {
  # query provider, output one name per line
  provider-cli list | transform-to-names > "$CACHE.tmp" && mv "$CACHE.tmp" "$CACHE"
}

_ensure_cache() {
  if [ -f "$CACHE" ]; then
    # stale-while-revalidate: serve stale, rebuild in background
    age=$(( $(date +%s) - $(stat -f%m "$CACHE") ))
    [ "$age" -lt "$TTL" ] || _build_cache &
  else
    _build_cache  # cold start: block until ready
  fi
}

_invalidate() { rm -f "$CACHE" "$CACHE.tmp"; }

case "${1:-}" in
  read)   _ensure_cache; provider-cli get-value "$2" ;;
  list)   _ensure_cache; cat "$CACHE" ;;
  flush)  _invalidate ;;
  # add write/delete here if the provider supports it:
  # write)  _invalidate; provider-cli set "$2" "$3" ;;
  # delete) _invalidate; provider-cli rm "$2" ;;
  *)      echo "usage: $0 {read|list|flush}" >&2; exit 64 ;;
esac
