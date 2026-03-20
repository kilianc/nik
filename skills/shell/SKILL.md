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

## Container management

Your shell runs in a Docker container. `workspace/shell/Dockerfile`
declares what software is installed.

To add software:
1. Read the Dockerfile with `fs_read`
2. Add the package to the `RUN` line
3. Call `shell-rebuild`
4. Verify the tool works by running a test command
5. If the environment is broken beyond repair, call
   `shell-factory-reset` to reset the Dockerfile and start over
