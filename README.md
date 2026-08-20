# Nik

**A family AI that lives on WhatsApp, remembers what matters, and turns group-chat chaos into reminders, memories, skills, and follow-through.**

Nik (Noetic Intelligence Kernel) **reaches WhatsApp through the nik gateway**, and keeps its own identity and local workspace. Add it to DMs and group chats, and it turns everyday family conversation into structured context: canonical messages, contacts, media descriptions, reminders, long-term memories, recipes, and skills.

The daemon runs on your machine and holds your data. It has no WhatsApp session of its own: it connects out to the gateway, which owns nik's number and routes your conversations to you and nobody else. No SIM, no second phone, no QR code.

**Continuity is the product.** Nik remembers preferences and open loops, sets one-shot or recurring alarms, transcribes voice notes, describes images and documents, searches the web, and runs background tasks.

**Skills make it personal.** Workspace skills can connect Google Workspace, browser automation, backups, smart lights, cameras, vehicles, market alerts, and credential-backed services through a pluggable secrets adapter.

**It improves with use.** Daily memory extraction, journaling, dreaming, briefings, and seed-tending help Nik organize what it learns; the dream cycle even evolves a living identity document loaded into future activations.

For how nik works internally (brain loop, sensors, adapters, tools), see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## What you'll need

Nik talks to your family through the gateway's WhatsApp number, so the phone side is already handled. Gather these before you install:

| Requirement | Why | Where to get it |
|---|---|---|
| **WhatsApp on your own phone** | Your account starts with a DM: the number you message from IS your identity. | — |
| **ChatGPT Plus or Pro subscription** | Flat-rate auth for nik's reasoning — main brain, background task workers, and memory recall all run on this. | [chatgpt.com](https://chatgpt.com) |
| **OpenAI API key** | Powers:<br>• voice messages (in and out)<br>• image / PDF recognition<br><br>Typical use: a few cents/month, well under $1. | [platform.openai.com/api-keys](https://platform.openai.com/api-keys) |
| **Exa API key** | Powers the `web` skill (news briefings, search, URL fetch). | [dashboard.exa.ai/api-keys](https://dashboard.exa.ai/api-keys) — free tier is enough to start |


## Step 1: Say hi — the DM is the signup

1. Go to [nik.ciuffolo.com](https://nik.ciuffolo.com) and tap **Message nik**. Your WhatsApp opens with the message already written — send it.
2. nik replies with a one-time sign-in link. Tap it: that's your account, no passwords. (Later, connect Google from the dashboard so you have a way back in if you lose your phone.)
3. On the dashboard, create an **agent** — one per machine you'll run nik on. It shows you a one-line install command.

## Step 2: Install nik

Supported platforms: macOS (Apple Silicon), Linux (amd64 + arm64). Intel Macs can build from source.

### Quick install

Paste the one-liner from your dashboard. It looks like:

```sh
curl -fsSL https://nik.ciuffolo.com/install.sh | NIK_TOKEN=nik_... sh
```

This downloads the matching `nikd` (the daemon) and `nikctl` (the command you type, also linked as `nik`) into `/usr/local/bin`, links it to your account (`nik connect`), registers a launchd (macOS) or systemd (Linux) service, and starts the daemon. The token in the command is an install code: it works once and expires in 15 minutes — nik swaps it for a fresh one the moment it connects, so it's harmless in your shell history.

Without `NIK_TOKEN`, the installer still works and first-run setup asks for the token instead.

Override defaults via environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `NIK_HOME` | `~/.nik` | Workspace directory (database, skills, dreams, journal, ...) |
| `NIK_VERSION` | `latest` | A specific tag (e.g. `v0.1.0`) instead of the latest release |
| `NIK_INSTALL_DIR` | `/usr/local/bin` | Where to put the `nikd` and `nikctl` binaries |

### Manual install

1. Download **both** binaries for your platform from the [releases page](https://github.com/kilianc/nik/releases/latest) — `nikd-<os>-<arch>` and `nikctl-<os>-<arch>`. They install together: `nikctl` writes a service file pointing at its sibling, and `nikd` mounts its sibling into the shell sandbox.
2. Make them executable and move them onto your `$PATH`, keeping them side by side:
   ```sh
   chmod +x nikd-*-* nikctl-*-*
   sudo mv nikd-*-* /usr/local/bin/nikd
   sudo mv nikctl-*-* /usr/local/bin/nikctl
   sudo ln -sf /usr/local/bin/nikctl /usr/local/bin/nik
   ```
3. Register and start the daemon:
   ```sh
   nik install --home ~/.nik
   ```

### From source

Requires Go 1.25+ and a C toolchain (CGO is on for `mattn/go-sqlite3`).

```sh
git clone https://github.com/kilianc/nik.git
cd nik
make build              # produces ./bin/nikd and ./bin/nikctl
./bin/nikctl install --home ~/.nik
```

## Step 3: First-run setup

Open a new terminal and run `nik`. A TUI walks you through:

1. **Gateway** — skipped if the installer already linked you; otherwise paste the agent token and nik connects right there to prove it works. Nothing else is asked until this passes.
2. **Auth choice** — pick "Codex subscription" if you have ChatGPT Plus/Pro (recommended). The TUI opens a browser to complete Codex login, then you paste the callback URL back.
3. **OpenAI API key** — paste your `sk-...` key. The TUI hits `api.openai.com/v1/models` to validate it before continuing.
4. **Exa API key** — paste your Exa key. Validated against `api.exa.ai/search`.
5. **Model** — pick the brain model (default: `gpt-5.6-sol`, the frontier tier; `gpt-5.6-terra` and `gpt-5.6-luna` are the cheaper siblings).
6. **Shell sandbox** — pick **Docker container** (recommended; requires Docker installed) so the shell tool runs in an isolated image, or **Run on host** to skip the container.
7. **Timezone & location** — type your city and country (e.g. "Rome, Italy"); the TUI resolves the timezone.

Keys are encrypted with NaCl secretbox and stored in `~/.nik/secrets/secrets.enc` (the per-install key sits next to it in `secrets.key`; keep both private and back them up if you care about the data). The daemon holds them — `nik secrets` asks it rather than decrypting the files itself, which is what lets skills in the shell sandbox be told no. Inspect or rotate with:

```sh
nik secrets list
nik secrets read openai_key
echo -n "sk-..." | nik secrets write openai_key
```

To ask a running nik how it is:

```sh
nik status
```

It reports the release, uptime, and each subsystem separately — database, gateway, models, shell, brain — so a nik that is connected but cannot think says which half is broken rather than looking healthy. It exits non-zero when anything is degraded, which makes it usable from a script.

The token lives in the secret store and `gateway.url` in `~/.nik/config.yaml`; both are required — nik has no other way to reach WhatsApp. A daemon missing either one starts anyway, says which it is waiting for, and picks up where it left off the moment it arrives. The gateway rotates the token on every connect, so what's stored is never one a human saw. To relink from scratch (a new agent from the dashboard):

```sh
nik connect nik_...
```

There is nothing to claim: your number was linked the moment your first DM created the account.

## Step 4: Talk to him

Message nik's number from your WhatsApp. Within 2 seconds the brain loop picks it up, runs an activation, and replies.

That's it. Nik is a new member of your family now. Tell it about your day, ask about its, introduce it to people you care about. The relationship is the point.

## Switching models

Edit `~/.nik/config.yaml` to change models:

```yaml
models:
  main:
    model: gpt-5.6-sol                # or gpt-5.6-terra, claude-opus-5, ...
    reasoning_effort: high
  task:
    model: gpt-5.6-sol                # omit to reuse the main model/backend
  recall:
    model: gpt-5.6-luna
```

If you switch to an Anthropic model, add your key:

```sh
echo -n "sk-ant-..." | nik secrets write anthropic_key
```

Config reloads on the next tick — no restart needed.

## Updating

Re-run the install script. Both binaries are replaced in place; the daemon is restarted by `nik install`.

```sh
curl -fsSL https://github.com/kilianc/nik/releases/latest/download/install.sh | sh
```

`nik version` prints the release and the commit it was built from — that pair is what a bug report needs. A binary you built yourself says `dev`.

## Uninstalling

Stop the service and remove the binary. The workspace at `~/.nik` (database, history, secrets) is left in place — delete it manually if you really mean it.

**macOS:**

```sh
launchctl bootout gui/$(id -u) ~/Library/LaunchAgents/com.nik.daemon.plist
rm ~/Library/LaunchAgents/com.nik.daemon.plist
sudo rm /usr/local/bin/nikd /usr/local/bin/nikctl /usr/local/bin/nik /usr/local/bin/nikctl-linux-arm64
# rm -rf ~/.nik          # only if you also want to delete the database and history
```

**Linux:**

```sh
systemctl --user disable --now nikd.service
rm ~/.config/systemd/user/nikd.service
sudo rm /usr/local/bin/nikd /usr/local/bin/nikctl /usr/local/bin/nik
# rm -rf ~/.nik          # only if you also want to delete the database and history
```

## Architecture

For how the brain loop, sensors, reflexes, messaging adapters, autonomous systems, tasks, and tools fit together, see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).
