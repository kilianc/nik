---
name: maintenance
summary: >
  Nightly system maintenance: prune old data, test auth, verify alarms and
  skill outputs, check data integrity and spending, report to diagnostics/.
  Load when the maintenance reflex fires or DB size is large.
tools: [db_prune, db_query, shell, alarm, load_skill, read_file, message_send]
diagnostic_skip: true
reflex:
  - name: maintenance
    every: every day at 3am
---

# Maintenance

Nightly health check and cleanup. Prune first, then diagnose. Output
goes to `diagnostics/YYYY/MM/DD/YYYY-MM-DD.md`.

## Constraints

- Target: **under 25 rounds**. Batch shell commands into scripts.
- Load this skill exactly once. Do not re-load it mid-run.
- Read the prior diagnostic at most once.
- Do not fix anything. Report only.

## Skip policy

A skill may opt out of nightly auth/install checks by setting
`diagnostic_skip: true` in its frontmatter. Record `SKIP` with the
reason from the skill docs.

## db_prune reference

Deletes all ephemeral data older than the configured `retention` period
(config.yaml, default `720h` / 30 days). One tool call, no arguments.

**What it deletes:**
- `activation` and all children: `activation_round`, `tool_call`,
  `shell_session`, `experiment`, `experiment_variant`,
  `experiment_variant_run`
- `task` and all children: `task_report`
- Cross-references (`task.activation_id`, `activation.task_id`,
  `task.retry_for_task_id`) are detached before deletion
- `message` rows with `platform = 'system'` (task reports, alarm events,
  skill events, etc.)

**What it preserves:**
Conversations, WhatsApp messages, contacts, media, alarms, alarm
occurrences, skills, skill events, skill reflexes, and cron cache.

## Step 0 — Prune

1. Record DB size before: `wc -c < nik.db`
2. Call `db_prune`. Record `rows_deleted` and `cutoff`.
3. Record DB size after: `wc -c < nik.db`

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
for d in assessments awareness backups breathing briefings diagnostics dreams journal memories soul; do
  if [ -d "$d" ]; then
    if [ -e "$d/latest.md" ]; then
      target=$(readlink "$d/latest.md" 2>/dev/null || echo "live")
      size=$(wc -c < "$d/latest.md" | tr -d ' ')
      echo "$d: $target ${size}B"
    else
      latest=$(find "$d" -path '*/[0-9][0-9][0-9][0-9]/[0-9][0-9]/*.md' 2>/dev/null | sort | tail -1)
      if [ -n "$latest" ]; then
        size=$(wc -c < "$latest" | tr -d ' ')
        echo "$d: $latest ${size}B"
      else
        echo "$d: EMPTY"
      fi
    fi
  fi
done

echo "=== MEMORIES ==="
mem_count=$(find memories/ -path '*/[0-9][0-9][0-9][0-9]/[0-9][0-9]/*.md' 2>/dev/null | grep -c . || echo 0)
mem_total=$(wc -c < memories/latest.md 2>/dev/null | tr -d ' ')
mem_latest=$(find memories/ -path '*/[0-9][0-9][0-9][0-9]/[0-9][0-9]/*.md' 2>/dev/null | sort | tail -1)
mem_rows=$(grep -c '^|' memories/latest.md 2>/dev/null || echo 0)
mem_cursor=$(cat memories/latest-cursor.txt 2>/dev/null || echo "MISSING")
echo "snapshots=$mem_count latest_snapshot=$mem_latest"
echo "latest_bytes=$mem_total rows=$mem_rows cursor=$mem_cursor"

echo "=== SOUL ==="
soul_size=$(wc -c < soul/latest.md 2>/dev/null | tr -d ' ')
soul_latest=$(find soul/ -path '*/[0-9][0-9][0-9][0-9]/[0-9][0-9]/*.md' 2>/dev/null | sort | tail -1)
soul_snapshots=$(find soul/ -path '*/[0-9][0-9][0-9][0-9]/[0-9][0-9]/*.md' 2>/dev/null | grep -c . || echo 0)
echo "latest_bytes=$soul_size snapshots=$soul_snapshots latest_snapshot=$soul_latest"

echo "=== SEEDS ==="
seed_count=$(ls -1 seeds/*.md 2>/dev/null | grep -cv '^$' || echo 0)
seed_cursor=$(cat seeds/latest-cursor.txt 2>/dev/null || echo "MISSING")
if [ "$seed_count" -gt 0 ] 2>/dev/null; then
  seed_oldest=$(ls -1t seeds/*.md 2>/dev/null | tail -1)
  echo "active=$seed_count oldest=$seed_oldest cursor=$seed_cursor"
else
  echo "active=0 cursor=$seed_cursor"
fi

echo "=== ASSESSMENTS ==="
assess_count=$(find assessments/ -name '*.md' 2>/dev/null | grep -c . || echo 0)
assess_latest=$(find assessments/ -name '*.md' 2>/dev/null | sort | tail -1)
echo "count=$assess_count latest=$assess_latest"

echo "=== BRIEFING ==="
topics_count=$(grep -c '^\- ' briefings/topics.md 2>/dev/null || echo 0)
topics_mtime=$(stat -f '%Sm' -t '%Y-%m-%d' briefings/topics.md 2>/dev/null || echo "MISSING")
echo "topics=$topics_count topics_updated=$topics_mtime"

echo "=== ERROR LOG ==="
errlog="nik.err.log"
if [ -f "$errlog" ]; then
  today=$(date +%Y-%m-%d)
  yesterday=$(date -v-1d +%Y-%m-%d 2>/dev/null || date -d 'yesterday' +%Y-%m-%d)
  recent=$(grep -E "time=${today}|time=${yesterday}" "$errlog")
  total=$(echo "$recent" | grep -c . || echo 0)
  warns=$(echo "$recent" | grep -c 'level=WARN' || echo 0)
  errors=$(echo "$recent" | grep -c 'level=ERROR' || echo 0)
  echo "24h_lines=$total warns=$warns errors=$errors"
  echo "-- top messages"
  echo "$recent" | grep -oP 'msg="[^"]*"|msg=\S+' | sort | uniq -c | sort -rn | head -5
  echo "-- last errors"
  echo "$recent" | grep 'level=ERROR' | tail -5 | cut -c1-200
else
  echo "SKIP no-file"
fi

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

### 2d — Task effectiveness (7-day)

```sql
SELECT
  COUNT(*) AS total,
  SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS completed,
  SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) AS failed,
  SUM(CASE WHEN status = 'cancelled' THEN 1 ELSE 0 END) AS cancelled
FROM task
WHERE created_at >= DATETIME('now', '-7 days')
  AND status IN ('completed', 'failed', 'cancelled')
```

Cross-reference with assessment files: `read_file` the most recent
2-3 assessments from `assessments/` (if any exist). Note the
effectiveness scores and any recurring recommendations across tasks.
If no assessments exist, WARN that the critic skill may not be running.

### 2e — Memory cursor lag

```sql
SELECT MAX(sent_at) AS latest_msg
FROM message
WHERE kind = 'text'
  AND body != ''
```

Compare `latest_msg` to the cursor value from the shell output. If
the cursor is more than 24h behind, extraction is falling behind — WARN.

### 2f — Data health

```sql
SELECT
  (SELECT COUNT(*) FROM activation) AS activations,
  (SELECT COUNT(*) FROM task) AS tasks,
  (SELECT COUNT(*) FROM tool_call) AS tool_calls,
  (SELECT COUNT(*) FROM shell_session) AS shell_sessions,
  (SELECT COUNT(*) FROM activation_round) AS activation_rounds,
  (SELECT COUNT(*) FROM message WHERE platform = 'system') AS system_msgs,
  (SELECT COUNT(*) FROM message WHERE platform = 'whatsapp') AS whatsapp_msgs,
  (SELECT COUNT(*) FROM contact) AS contacts,
  (SELECT COUNT(*) FROM media) AS media,
  (SELECT COUNT(*) FROM alarm) AS alarms,
  (SELECT COUNT(*) FROM alarm_occurrence) AS alarm_occurrences
```

## Step 3 — Write report

Write `diagnostics/YYYY/MM/DD/YYYY-MM-DD.md` via `shell`. Create the
directory with `mkdir -p diagnostics/YYYY/MM/DD`, then write the file.
After writing, update the symlink:
`ln -sf YYYY/MM/DD/YYYY-MM-DD.md diagnostics/latest.md`.
Lead with failures.

### Report structure

```markdown
# Diagnostic — YYYY-MM-DD

## Summary
all clear / N issues — one-line per FAIL/WARN

## Pruning
rows_deleted, cutoff, db_size_before → db_size_after (delta)

## Auth & Services
per-service: command, PASS/FAIL/SKIP, error detail if any
for install:true skills, note whether expected resources are healthy
group downstream failures under root cause (e.g. vault down)

## Error Log
24h: N warns, M errors
top messages (deduplicated with counts)
last 5 errors (truncated)
WARN if errors > 0, FAIL if errors > 10

## Alarms
dead, duplicate, or stale alarms with short IDs and goals

## Skill Outputs
per-directory: latest file, gap analysis
missing 1 day = WARN, missing 2+ days = FAIL
cross-reference: if alarm dead, that's the root cause

## Task Effectiveness
7-day completed/failed/cancelled counts
recent assessment scores and recurring themes
WARN if no assessment files exist (critic skill not running)
WARN if fail rate > 30%

## Data Quality
PRAGMA results, stale tasks

## Data Health
table row counts (activations, tasks, tool_calls, shell_sessions,
activation_rounds, messages by platform, contacts, media, alarms,
alarm_occurrences)

## Memories
snapshot count, latest snapshot date, latest.md row count and size
cursor value vs latest message — WARN if >24h behind

## Soul
latest.md size, snapshot count, latest snapshot date
WARN if latest snapshot is 2+ days old (dream cycle not evolving soul)

## Seeds
active seed count, oldest seed, cursor value
WARN if cursor >8h stale (extract not running)

## Briefings
topic count, topics.md last modified date
WARN if topics.md not updated in 7+ days

## Spending
30-day total, 24h vs 7-day avg, DB size

## Recommendations
concrete action for every FAIL
```

## Step 4 — Recap

After writing the report, send a concise WhatsApp recap to the owner
via `message_send`. One message, no fluff. Example:

```
Maintenance 2026-03-28
Pruned 1,204 rows (213MB → 189MB)
Auth 7/7 PASS · Alarms 20 ok
Errors 24h: 2 warn / 0 err ✓
Tasks 7d: 12 completed, 2 failed · Avg score 3.8/5
Memories 342 rows, cursor current
Soul evolved 03-27 · Seeds 3 active
Spend $12.40 24h / $15.20 7d avg
All clear ✓
```

Lead with FAILs if any. Keep it under 10 lines.

### Recommendation patterns

- Auth failure → re-run `setup` or rotate credentials
- Dead alarm → reschedule with `update_alarm`
- Duplicate alarms → cancel one with `cancel_alarm`
- Missing output + healthy alarm → load skill, check recent activations
- Missing output + dead alarm → fix alarm first
- Stale task → check `task_status`, cancel if stuck
- Integrity failure → back up DB immediately
- Spend > 2x average → check for loops or large contexts
- DB size > 500 MB after prune → investigate large tables
- Error count > 10 in 24h → investigate recurring error messages, check for loops or persistent failures
- Repeated identical error message → likely a single root cause, cite the message
- Error log missing → SKIP (nik may not have run)
- Memory cursor >24h behind → check memory extract alarm health
- Soul not updated in 2+ days → check dream cycle alarm and recent dream files
- Seed cursor >8h stale → check seed extract alarm
- Briefing topics stale 7+ days → topics may need refresh
- No assessment files → check critic skill alarm health
- Task fail rate > 30% → review recent assessments for recurring root causes
- Recurring tool/skill issues across assessments → flag for owner

### Escalation

The Step 4 recap always goes to the owner. If any CRITICAL finding or
3+ FAIL findings, call out the failures prominently in the recap.
