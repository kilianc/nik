---
name: secrets
summary: >
  Secure credential access via an encrypted local store. Load this skill
  when you need API keys, tokens, or other credentials for a service.
tools: [shell]
preload: true
---

# Secrets

All secret access goes through a single adapter script at `./secrets/cli`. By default it delegates to the built-in encrypted store (`nik secrets`). The user can swap it for an external provider if they choose.

Do not assume which provider is behind the adapter. Do not reference provider-specific paths, CLIs, or URI schemes anywhere -- not in messages, task plans, or reports.

## Contract

```
./secrets/cli read <name>            # print secret value to stdout
./secrets/cli write <name> <value>   # store or update a secret
./secrets/cli delete <name>          # remove a secret
./secrets/cli list                   # print secret names, one per line
```

`list` returns names to pass to `read`. It never prints values. External providers may not support `write` or `delete` -- if either fails, ask the user to manage the secret through their own tool.

## Using secrets safely

- **Always use `$()` substitution** to pass secrets to commands. The value stays in the shell and never appears in tool output.

```
API_KEY="$(./secrets/cli read some_api_key)" some_command
```

- **Never** echo, log, print, or include secret values in messages, reports, or task outputs.
- **Never** store secrets in plaintext files, environment variables, or config as a workaround.
- **Never** issue commands that would store secrets in the database, write them to log files, or send them to third parties.

## Missing secrets

When a `read` fails, first run `./secrets/cli list` and scan the output for plausible matches -- the user may have stored the secret under a different name (different separators, prefixes, abbreviations, or word order). If a likely match exists, use it.

If multiple candidates look plausible, ask the user which one is correct. Only when nothing matches, ask the user to add it under the expected name.

## Install

If `./secrets/cli` doesn't exist, copy the default adapter from `skills/secrets/cli.sh`, make it executable, and verify with `./secrets/cli list`.

## Switching providers

If the user wants to use an external password manager (1Password, Bitwarden, etc.) instead of the built-in store, rewrite `./secrets/cli` to delegate to their tool's CLI while preserving the contract above.
