---
name: shell
summary: Run commands in persistent tmux sessions, optionally inside a Docker container. Owner-only.
tools: [shell, shell-rebuild, shell-factory-reset]
---

# Shell

Your personal shell. Each `run` opens a new tmux session. Never wrap
commands in tmux/screen/nohup/bg yourself.

## Actions

- `run` -- start a command and watch. Requires `command`, `description`,
  and `next_check_at`.
- `read` -- look at a session's current output. Optional `max_wait` to
  stare, optional `next_check_at` to reschedule.
- `send` -- type text + Enter into a session. Requires `input`. Then
  behaves like read.
- `kill` -- destroy a session. Requires `session_id`.
- `list` -- show all sessions with metadata.

## Parameters

| Param | Used by | Purpose |
|---|---|---|
| `action` | all | run, read, send, kill, list |
| `command` | run | the shell command to execute |
| `description` | run | what this does and who asked (e.g. "database backup for Alice") |
| `session_id` | read, send, kill | target session |
| `input` | send | text to type + Enter |
| `max_wait` | run, read, send | seconds to watch the terminal (polls for exit, returns early). Default 10 for run, 0 for read/send. |
| `next_check_at` | run, read, send | when to come back (RFC3339 absolute timestamp). Required for run. |

## Two modes

**Staring** (`max_wait`): watch output live. The system polls every
500ms and returns early if the command finishes. Costs one round of
thinking. Good for fast commands.

**Checking in** (`next_check_at`): schedule a reminder and yield. You
get the session output delivered to you at that time and decide what to
do next. Once `next_check_at` is set, you are done with that session
this turn -- reply to the user and stop.

## Workflow

1. `run` with a `next_check_at` estimate based on expected duration.
2. If the command finishes within `max_wait`, you get the exit code and
   output immediately -- done.
3. If it's still running, you get partial output and a `session_id`.
   Reply to the user and yield. The check-in fires automatically.
4. At check-in, you see the latest output. Decide: `read` to stare
   more, `send` to provide input, `kill` to stop, or reply and yield
   again.

## Safety

- This tool is **privileged** (owner-only).
- Never run destructive commands without the owner explicitly asking.
- Use non-interactive flags (`-y`) when possible. For interactive
  prompts, use `send`.
- If something fails, explain what happened and ask before retrying.

## File organization

- `downloads/` — downloaded files, fetched assets, anything pulled from the internet
- `tmp/` — throwaway scripts and intermediate work products
- `media/` — system-managed message attachments. Never write to it directly.

## Container management

If your **Shell environment** says Docker, your shell runs in a container
(Debian bookworm) that you maintain.

`Dockerfile` in the workspace root is the source of truth for installed
software. Anything installed on the live container but not in the
Dockerfile will be lost when the container restarts.

Install software however you like — apt, npm, pip, go install, curl — as
long as you commit the install step to the Dockerfile afterward. Then
call `shell-rebuild` so the image stays in sync.

To add software:
1. Install it live in the shell to verify it works
2. Read `Dockerfile` with `read_file`
3. Add the equivalent install step to the Dockerfile
4. Call `shell-rebuild` to bake it into the image
5. If the environment is broken beyond repair, call
   `shell-factory-reset` to restore the default Dockerfile and rebuild

## Install

Ask the owner which shell mode to use:

- **Docker** (set `shell.docker_image` in config): sandboxed, reproducible environment. Software managed via Dockerfile. Can't access host network or devices directly. Rebuild with `shell-rebuild`, factory reset with `shell-factory-reset`.
- **Local** (leave `shell.docker_image` empty): runs tmux directly on the host. Full access to host tools, network, devices. No isolation or Dockerfile management. The Container management section above doesn't apply.
