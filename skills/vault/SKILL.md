---
name: vault
summary: >
  Secure credential access via a user-chosen secret store. Load this skill
  when you need API keys, tokens, or other credentials for a service.
tools: [shell]
preload: true
---

# Vault

All secret access goes through a single adapter script at
`./vault/cli`. The adapter wraps whatever secret store the
user chose during setup. You write the adapter yourself, tailored to
their tool.

Do not assume which provider is behind the adapter. Do not reference
provider-specific paths, CLIs, or URI schemes anywhere -- not in
messages, task plans, or reports. If the conversation timeline mentions
old providers or helpers, ignore it; only the current adapter matters.

## Contract

```
./vault/cli read <name>            # print secret value to stdout
./vault/cli write <name> <value>   # store or update a secret (optional)
./vault/cli delete <name>          # remove a secret (optional)
./vault/cli list                   # print secret names, one per line
```

`read` and `list` are required. `write` and `delete` are optional --
many password managers grant read-only access to service accounts or
CLI tokens. If write is unavailable, ask the user to add or remove
secrets through their own tool.

`list` returns strings to pass to `read` to retrieve values.
It never prints the values themselves.

## Using secrets safely

- **Always use `$()` substitution** to pass secrets to commands. The
  value stays in the shell and never appears in tool output.

```
API_KEY="$(./vault/cli read some_api_key)" some_command
```

- **Never** echo, log, print, or include secret values in messages,
  reports, or task outputs.
- **Never** store secrets in plaintext files, environment variables,
  or config as a workaround.
- **Never** issue commands that would store secrets in the database,
  write them to log files, or send them to third parties.

## Missing secrets

When a `read` fails, don't immediately ask the user to add it.
First run `./vault/cli list` and scan the output for plausible
matches -- the user may have stored the secret under a different
name (different separators, prefixes, abbreviations, or word order).
If a likely match exists, use it. If multiple candidates look
plausible, ask the user which one is correct. Only when nothing
in the list looks related, ask the user to add it under the expected
name. The user manages secrets through their own tool -- don't tell
them to run vault commands.

## Install

If `./vault/cli` doesn't exist or fails, **stop and talk to
the user.** Do not guess, do not pick a provider, do not write an
adapter without their input. Follow these steps in order:

1. Tell the user you need a secret store to handle credentials safely.
2. Ask if they already have a password manager or encrypted secret
   store. If they don't, ask if they'd like help picking one.
3. Once a tool is chosen, **look up official docs** for its CLI and
   unattended / headless / daemon mode. Nik runs as a daemon -- the
   adapter must work without human interaction (no login prompts,
   no Touch ID, no browser OAuth, no GUI unlocks). If the tool doesn't
   support unattended access, it's not suitable.
4. Research how the tool authenticates for unattended use. Every
   secret store has a bootstrapping credential that can't live in the
   vault itself (chicken-and-egg). Common patterns: service account
   tokens, API keys, app-specific passwords, age identities. Guide
   the user through creating and saving it. These bootstrapping
   credentials live in `./vault/` as files, protected by file
   permissions and OS-level disk encryption. Never ask for auth
   credentials in a chat message -- guide the user to save them to a
   file directly. Explain the trade-off honestly: one credential on
   disk that unlocks everything else.
5. Walk the user through installing and initializing their tool.
6. Write the adapter script at `./vault/cli` following this template:

```
#!/bin/sh
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
```

7. Test the adapter. If write is supported, do a round-trip: write a
   test secret, read it back, delete it. If the adapter is read-only,
   ask the user to add a test secret through their tool and verify
   you can read it. Debug with the user if it fails.
8. Ask the user to add the secrets that skills need to their password
   manager. Verify by reading through the adapter.
