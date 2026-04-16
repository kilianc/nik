---
name: secrets
summary: >
  Secure credential access via a user-chosen secret store. Load this skill
  when you need API keys, tokens, or other credentials for a service.
tools: [shell]
preload: true
---

# Secrets

All secret access goes through a single adapter script at `./secrets/cli`. The adapter wraps whatever secret store the user chose during setup. You write the adapter yourself, tailored to their tool.

Do not assume which provider is behind the adapter. Do not reference provider-specific paths, CLIs, or URI schemes anywhere -- not in messages, task plans, or reports. If the conversation timeline mentions old providers or helpers, ignore it; only the current adapter matters.

## Contract

```
./secrets/cli read <name>            # print secret value to stdout
./secrets/cli write <name> <value>   # store or update a secret (optional)
./secrets/cli delete <name>          # remove a secret (optional)
./secrets/cli list                   # print secret names, one per line
```

`read` and `list` are required. `write` and `delete` are optional -- many password managers grant read-only access to service accounts or CLI tokens. If write is unavailable, ask the user to add or remove secrets through their own tool.

`list` returns strings to pass to `read` to retrieve values. It never prints the values themselves.

## Using secrets safely

- **Always use `$()` substitution** to pass secrets to commands. The value stays in the shell and never appears in tool output.

```
API_KEY="$(./secrets/cli read some_api_key)" some_command
```

- **Never** echo, log, print, or include secret values in messages, reports, or task outputs.
- **Never** store secrets in plaintext files, environment variables, or config as a workaround.
- **Never** issue commands that would store secrets in the database, write them to log files, or send them to third parties.

## Missing secrets

When a `read` fails, don't immediately ask the user to add it. First run `./secrets/cli list` and scan the output for plausible matches -- the user may have stored the secret under a different name (different separators, prefixes, abbreviations, or word order). If a likely match exists, use it.

If nothing in the list looks related, the cache may be stale. Run `./secrets/cli flush` to invalidate it, then `./secrets/cli list` again to rebuild with fresh data. Scan the new output the same way.

If multiple candidates look plausible, ask the user which one is correct. Only when nothing matches after the flush, ask the user to add it under the expected name. The user manages secrets through their own tool -- don't tell them to run secrets commands.

## Install

If `./secrets/cli` doesn't exist or fails, **stop and talk to the user.** Do not guess, do not pick a provider, do not write an adapter without their input.

If the user has no external password manager and wants to use nik's built-in encrypted secrets store, write the adapter to delegate to `nik secrets`. Secrets are encrypted at rest with nacl/secretbox.

For external providers (1Password, Bitwarden, etc.), read `skills/secrets/adapter-template.sh` for the reference adapter implementation and follow the setup steps there.
