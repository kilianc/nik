---
name: diagnostic
summary: >
  Nightly system diagnostic. Discovers all skills and services, tests auth
  for anything that needs credentials, verifies alarm chains and skill
  outputs, checks data integrity. Load when the diagnostic alarm fires.
tools: [db_query, shell, alarm, load_skill]
---

# Diagnostic

Nightly health check that predicts what will break tomorrow. Everything
lives on the file system under `diagnostics/`.

## File layout

```
diagnostics/
  2026-03-06.md
  2026-03-07.md
```

Use `shell` to write these files. Create the directory if it doesn't exist.

## Scheduling

Maintain a daily recurring alarm for the nightly diagnostic. If you
don't have one, create it:

```
alarm goal: "Nightly system diagnostic — load diagnostic skill", fire_at: <late night>, recurrence: "every day"
```

When the alarm fires, follow the full workflow below.

## Design principle

This skill defines **rules**, not a checklist. Never hardcode skill or
service names — discover what exists at runtime and apply universal
rules. The goal is to predict what will break tomorrow so you can fix
it before it matters.

## Phase 1 — Discover

Before checking anything, build a picture of what exists right now.

### 1a — Enumerate skills

`load_skill` action `list`. Note each skill's name, summary, and tools.

### 1b — Find CLIs

```
ls skills/*/main.go
```

For each `main.go`, run it with no arguments. Go-based skill CLIs
typically print a JSON usage object with a `commands` list. Capture
the available commands for each.

### 1c — Find recurring alarms

```sql
SELECT id, goal, recurrence, next_fire_at
FROM alarm
WHERE cancelled_at IS NULL
  AND recurrence IS NOT NULL
  AND recurrence != ''
```

Map each alarm to the skill it serves by matching alarm goal text to
skill names from 1a.

### 1d — Find output directories

Scan the home directory for subdirectories that contain dated files
(the `YYYY-MM-DD.md` pattern). These are skill output directories.
Note the most recent file in each.

## Phase 2 — Test auth

The highest-value phase. For every skill with a CLI (from 1b),
determine if it needs external credentials and test them. Every test
is read-only — no mutations, no messages, no writes.

### Rules

1. **If the CLI has a `status` command** — run it. Check for
   `ready: true` or exit code 0. This is the preferred test.

2. **If no `status` but there's a read-only command** — run the
   lightest one. Success = auth works. Look for commands that read
   without side effects (words like `list`, `get`, `status`, `info`,
   `account`, `whoami`).

3. **For API-key services** (any key in config ending in `_key` or
   `_token`) — test with a minimal HTTP request if the skill
   documents an endpoint, or verify the key is non-empty.

4. **For services that pull credentials from a secrets manager** —
   test the secrets manager first. If it fails, skip downstream
   services and attribute all failures to the root cause.

5. **Skip skills with no external auth** — if the skill has no CLI
   and its tools list only contains local tools (`alarm`, `db_query`,
   `load_skill`), it doesn't need auth testing.

### How to identify auth-dependent skills

A skill likely needs auth if any of:

- It has a CLI with a `setup` command
- Its SKILL.md mentions credentials, tokens, API keys, OAuth, or a
  secrets manager
- Its tools list includes `shell` and its summary references an
  external service

When uncertain, try running the lightest available command. An auth
error is a finding worth reporting. A "command not found" or "no such
file" is not — just skip it.

### Interpreting results

- Auth failure = FAIL (the skill will break on its next activation)
- Secrets manager failure = CRITICAL (cascading — every skill that
  depends on it is broken)
- Include the actual error message to distinguish expired tokens,
  revoked keys, and network issues
- Group downstream failures under their root cause

## Phase 3 — Verify alarm chains

For every recurring alarm from 1c:

1. **Alive**: `cancelled_at IS NULL` and `next_fire_at IS NOT NULL`
2. **Scheduled**: `next_fire_at` is in the future
3. **Not duplicated**: no other active alarm with the same goal and
   overlapping schedule

Also find **dead recurring alarms**: `recurrence IS NOT NULL` but
`next_fire_at IS NULL` and `cancelled_at IS NULL`. These will never
fire again.

Severity:

- Dead alarm = FAIL
- Duplicate alarm = WARN
- Stale `next_fire_at` (in the past) = FAIL

## Phase 4 — Verify skill outputs

Using the output directories discovered in 1d:

1. Check for today's (or yesterday's, depending on schedule timing)
   dated file
2. Check that the file is non-empty
3. If the most recent file has clear section markers (headings), check
   they're all present

Severity:

- Missing for 1 day = WARN (single miss, could be transient)
- Missing for 2+ consecutive days = FAIL (systematic failure)

Cross-reference with Phase 3: if output is missing AND the alarm is
dead, the alarm is the root cause.

## Phase 5 — Shell sessions

Discover and clean up orphaned `nik-*` tmux sessions.

### 5a — List sessions

Via `shell`:

```
tmux list-sessions -F '#{session_name} #{pane_dead}' 2>/dev/null | grep '^nik-'
```

If no sessions exist, skip to the next phase.

### 5b — Kill dead sessions

Sessions with `pane_dead` = 1 are finished commands whose sessions
were never cleaned up. Kill each one:

```
tmux kill-session -t <session_name>
```

### 5c — Flag long-running sessions

For live sessions (`pane_dead` = 0), read the metadata env var:

```
tmux show-environment -t <session_name> NIK_META
```

`NIK_META` is JSON containing `started_at`. If a session has been
alive for 24h+, flag it — it may be stuck.

Severity:

- Dead sessions found and cleaned = INFO (routine, note the count)
- Live session running 24h+ = WARN (may be stuck)

## Phase 6 — Data quality

Via `db_query`:

- `PRAGMA integrity_check`
- `PRAGMA foreign_key_check`
- Stale tasks: `status IN ('running', 'pending')` with no
  `task_report` in 24h+

Keep this lean — only check things that indicate real data problems.

## Phase 7 — Spending

- 30-day spend: `SUM(cost_usd)` from `activation` for the last
  30 days
- 24h cost vs 7-day daily average
- DB file size via `shell`

## Phase 8 — Report

Write to `diagnostics/YYYY-MM-DD.md` via `shell`. Lead with failures.
The report is written for your future self — tomorrow morning,
scanning it should immediately tell you what needs attention.

### Structure

```markdown
# Diagnostic — YYYY-MM-DD

## Summary
all clear / N issues — one-line per FAIL/CRITICAL

## Auth & Services
per-skill: tested command, result, error detail if any
group downstream failures under root cause

## Alarm Chains
dead, duplicate, or stale alarms with IDs and goals

## Skill Outputs
per-directory: last file date, gap analysis

## Shell Sessions
dead sessions cleaned: N (list IDs)
long-running sessions: list with age and description

## Data Quality
PRAGMA results, stale tasks

## Spending
30-day total, 24h vs average, DB size

## Recommendations
see patterns below
```

### Recommendations

For every FAIL or CRITICAL finding, include a concrete recommendation.

**Auth failures:**

- Expired OAuth token → "re-run `setup` for [skill] to refresh the
  token"
- Revoked or invalid API key → "rotate the key in [secrets source]
  and update config"
- Secrets manager unreachable → "check the service account token or
  network connectivity; N downstream skills are blocked"
- Unknown auth error → quote the error and suggest loading the skill
  for its setup instructions

**Dead alarms:**

- "alarm [short ID] has recurrence but no next_fire_at — reschedule
  with `update_alarm` setting `next_fire_at` to the next occurrence"
- If multiple alarms are dead, note whether they share a pattern
  (e.g. all died on the same date)

**Duplicate alarms:**

- "alarms [ID1] and [ID2] have the same goal — cancel one with
  `cancel_alarm`"

**Missing skill outputs:**

- Alarm healthy but output missing → "the alarm is firing but the
  skill isn't producing output — load the skill and check for errors
  in recent activations"
- Alarm dead → "output missing because the alarm is dead (see above)"

**Long-running shell sessions:**

- "session [ID] has been alive for [duration] running `[command]` —
  check if stuck, kill with `tmux kill-session -t nik-[ID]` if no
  longer needed"

**Stale tasks:**

- "task [short ID] has been in [status] for [duration] with no
  report — check `task_status` and cancel if stuck"

**Data integrity:**

- Corruption → "PRAGMA integrity_check failed — back up the database
  immediately and investigate"
- FK violations → list affected tables and row counts

**Spending anomalies:**

- 24h cost > 2x the 7-day average → "spending spike — check recent
  activations for loops or unusually large contexts"

### Escalation

If any finding is CRITICAL (cascading failure), or 3+ findings are
FAIL, proactively message the owner. Otherwise, note findings in the
next journal entry.

## Tips

- Run auth tests sequentially — if the secrets manager fails, skip
  everything downstream
- Read yesterday's diagnostic before writing today's to track trends
- The diagnostic alarm is itself a recurring alarm — don't forget to
  reschedule it at the end
