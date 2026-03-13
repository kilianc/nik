---
name: vault
summary: >
  Secure credential access via a user-chosen secret store. Load this skill
  when you need API keys, tokens, or other credentials for a service.
tools: [shell]
preload: true
---

# Vault

Provider-agnostic secret access. The vault is a shell script adapter at
`./skills/vault/vault` that wraps whatever secret store the user has.
You write the adapter yourself during setup, tailored to their tool.

Uses: `shell`.

## Contract

The adapter must support four actions:

```
./skills/vault/vault read <name>            # print secret value to stdout
./skills/vault/vault write <name> <value>   # store or update a secret
./skills/vault/vault delete <name>          # remove a secret
./skills/vault/vault list                   # print secret names, one per line
```

`list` never prints values.

## Using secrets safely

- **Always use `$()` substitution** to pass secrets to commands. The
  value stays in the shell and never appears in tool output.

```
go run ./skills/tesla/main.go setup \
  "$(./skills/vault/vault read tesla_client_id)" \
  "$(./skills/vault/vault read tesla_client_secret)"
```

- **Never** echo, log, print, or include secret values in messages,
  reports, or task outputs.
- **Never** store secrets in plaintext files, environment variables,
  or config.yaml as a workaround.
- **Never** issue commands that would result in secret values being
  stored in the database, written to log files, or sent over the
  internet to third parties.

## When the vault doesn't exist

If `./skills/vault/vault` doesn't exist or fails and a secret is needed:

1. Tell the user you want to handle credentials safely.
2. Ask if they already have a password manager or secret store. If
   they don't, ask if they'd like help picking one.
3. Walk them through installing and initializing their tool.
4. Handle auth bootstrapping (see below).
5. Write the adapter script at `./skills/vault/vault`. Keep it simple
   -- `#!/bin/sh`, `set -eu`, ~20 lines. Namespace secrets with a
   prefix if the provider is shared (e.g. `nik/` in passage).
6. Test with a round-trip: write a test secret, read it back, delete
   it. Debug with the user if it fails.
7. Ask the user to add the secrets that skills need to their password
   manager (e.g. "can you add your Tesla API client_id under the name
   tesla_client_id?"). Verify by reading through the adapter.

## When a secret is missing

If the vault exists but a secret isn't found, ask the user to add it
to their password manager under the expected name. The user manages
secrets through their own tool -- don't tell them to run vault
commands. Verify it's there by reading through the adapter.

## Auth bootstrapping

Every secret store needs some way to authenticate. This is the one
credential that can't live in the vault (chicken-and-egg). When setting
up the adapter, research how the user's chosen tool handles headless /
CLI authentication and guide them through it.

Some tools need no auth file at all (the key lives in the user's home
directory or is managed by the OS). Others need a service account
token or API credentials saved to a file on disk. Never ask for auth
credentials in a chat message -- guide the user to save them to a
file directly.

For any provider that needs auth on disk, explain the trade-off
honestly: one credential on disk, protected by file permissions and
OS-level disk encryption. It unlocks everything else. Minimum possible
exposure.
