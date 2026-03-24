---
name: diagnostic
summary: >
  Nightly system diagnostic. Tests auth for credentialed services, verifies
  alarm chains and skill outputs, checks data integrity and spending.
  Load when the diagnostic alarm fires. Use medium thinking.
tools: [db_query, shell, alarm, load_skill]
diagnostic_skip: true
reflex:
  - name: diagnostic
    every: "0 6 * * *"
---

# Diagnostic

Nightly health check that predicts what will break tomorrow. Output
goes to `diagnostics/YYYY-MM-DD.md`.

## Constraints

- Target: **under 25 rounds**. Batch shell commands into scripts.
- Load this skill exactly once. Do not re-load it mid-run.
- Read the prior diagnostic at most once.
- Do not fix anything. Report only.

## Scheduling

The recurring alarm `[NIK_DIAGNOSTIC]` triggers this workflow.

## Skip policy

A skill may opt out of nightly auth/install checks by setting
`diagnostic_skip: true` in its frontmatter. Record `SKIP` with the
reason from the skill docs.

## Step 1 — Collect

Run these in order. Each is one tool call.

### 1a — Skill inventory

`load_skill` action `list`. Note each skill's name, summary, tools,
and whether it has `diagnostic_skip`.

### 1b — Runtime probes

Run a **single** shell script that performs all of the following and
prints structured output. Adapt the script to the skills discovered
in 1a — if a CLI is missing, print SKIP for that service.

```sh
#!/bin/sh
echo "=== AUTH ==="

echo "-- vault"
timeout 10 ./vault/cli list >/dev/null 2>&1 && echo "PASS" || echo "FAIL exit=$?"

echo "-- gws"
if command -v gws >/dev/null; then
  timeout 15 env GOOGLE_WORKSPACE_CLI_CONFIG_DIR=skills/google_workspace \
    GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE=skills/google_workspace/nik.json \
    gws auth status 2>&1 | head -5 || echo "FAIL exit=$?"
else echo "SKIP no-cli"; fi

echo "-- blockware"
if command -v go >/dev/null && [ -f skills/blockware/main.go ]; then
  timeout 30 go run ./skills/blockware/main.go summary 2>&1 | head -3 && echo "PASS" || echo "FAIL exit=$?"
else echo "SKIP no-cli"; fi

echo "-- robinhood"
if command -v go >/dev/null && [ -f skills/robinhood/main.go ]; then
  timeout 30 go run ./skills/robinhood/main.go account 2>&1 | head -3 && echo "PASS" || echo "FAIL exit=$?"
else echo "SKIP no-cli"; fi

echo "-- tesla"
if command -v go >/dev/null && [ -f skills/tesla/main.go ]; then
  timeout 30 go run ./skills/tesla/main.go vehicles 2>&1 | head -3 && echo "PASS" || echo "FAIL exit=$?"
else echo "SKIP no-cli"; fi

echo "-- lights"
if command -v openhue >/dev/null; then
  timeout 15 openhue get light 2>&1 | head -3 && echo "PASS" || echo "FAIL exit=$?"
else echo "SKIP no-cli"; fi

echo "-- browse"
if command -v agent-browser >/dev/null; then
  timeout 10 agent-browser --help >/dev/null 2>&1 && echo "PASS" || echo "FAIL exit=$?"
else echo "SKIP no-cli"; fi

echo "=== OUTPUTS ==="
for d in awareness backups breathing briefings diagnostics dreams journal memories soul; do
  if [ -d "$d" ]; then
    latest=$(ls -1 "$d" | grep -E '^[0-9]{4}-[0-9]{2}-[0-9]{2}\.md$' | sort | tail -1)
    if [ -n "$latest" ]; then
      size=$(wc -c < "$d/$latest" | tr -d ' ')
      echo "$d: $latest ${size}B"
    else
      echo "$d: EMPTY"
    fi
  fi
done

echo "=== META ==="
echo "db_size=$(wc -c < nik.db | tr -d ' ')"
echo "tmux_sessions=$(tmux list-sessions -F '#{session_name} #{pane_dead}' 2>/dev/null | grep -c '^nik-' || echo 0)"
```

If vault fails, note that downstream vault-dependent services are
blocked and skip their individual probes. If a new skill with a CLI
appears in the `load_skill list` that is not in the script above,
test it with its lightest read-only command (prefer `status`, then
`list`/`get`/`account`/`info`). A "command not found" is not a
failure — record SKIP.

### 1c — Prior diagnostic (optional)

If a prior diagnostic exists, `read_file` the most recent one to
compare trends. Do this at most once.

## Step 2 — Query

Run these as `db_query` calls. Combine into as few calls as possible.

### 2a — Alarm health

```sql
SELECT
  id, goal, recurrence, next_fire_at, cancelled_at
FROM alarm
WHERE cancelled_at IS NULL
  AND recurrence IS NOT NULL
  AND recurrence != ''
ORDER BY next_fire_at
```

Check each row:
- `next_fire_at IS NULL` → FAIL (dead recurring alarm)
- `next_fire_at` in the past → FAIL (stale)
- Duplicate goals across rows → WARN

### 2b — Data quality + stale tasks

```sql
SELECT 'integrity' AS check_name, integrity_check AS result
FROM pragma_integrity_check
UNION ALL
SELECT 'fk' AS check_name,
  COALESCE(
    (SELECT GROUP_CONCAT(table_name) FROM pragma_foreign_key_check),
    'ok'
  ) AS result
```

Stale tasks:

```sql
SELECT t.id, t.goal, t.status, t.created_at,
  MAX(tr.created_at) AS last_report
FROM task t
LEFT JOIN task_report tr ON tr.task_id = t.id
WHERE t.status IN ('running', 'pending')
GROUP BY t.id
HAVING last_report IS NULL
   OR last_report < DATETIME('now', '-24 hours')
```

### 2c — Spending

```sql
SELECT
  COALESCE(SUM(cost_usd), 0) AS total_30d,
  COALESCE(SUM(CASE WHEN created_at >= DATETIME('now', '-1 day') THEN cost_usd END), 0) AS last_24h,
  COALESCE(SUM(CASE WHEN created_at >= DATETIME('now', '-7 days') THEN cost_usd END) / 7.0, 0) AS avg_7d
FROM activation
WHERE created_at >= DATETIME('now', '-30 days')
```

## Step 3 — Write report

Write `diagnostics/YYYY-MM-DD.md` via `shell`. Create the directory
if it doesn't exist. Lead with failures.

### Report structure

```markdown
# Diagnostic — YYYY-MM-DD

## Summary
all clear / N issues — one-line per FAIL/WARN

## Auth & Services
per-service: command, PASS/FAIL/SKIP, error detail if any
for install:true skills, note whether expected resources are healthy
group downstream failures under root cause (e.g. vault down)

## Alarms
dead, duplicate, or stale alarms with short IDs and goals

## Skill Outputs
per-directory: latest file, gap analysis
missing 1 day = WARN, missing 2+ days = FAIL
cross-reference: if alarm dead, that's the root cause

## Data Quality
PRAGMA results, stale tasks

## Spending
30-day total, 24h vs 7-day avg, DB size

## Recommendations
concrete action for every FAIL
```

### Recommendation patterns

- Auth failure → re-run `setup` or rotate credentials
- Dead alarm → reschedule with `update_alarm`
- Duplicate alarms → cancel one with `cancel_alarm`
- Missing output + healthy alarm → load skill, check recent activations
- Missing output + dead alarm → fix alarm first
- Stale task → check `task_status`, cancel if stuck
- Integrity failure → back up DB immediately
- Spend > 2x average → check for loops or large contexts

### Escalation

If any CRITICAL finding or 3+ FAIL findings, proactively message the
owner. Otherwise note in the next journal entry.
