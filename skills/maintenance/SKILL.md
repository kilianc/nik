---
name: maintenance
summary: "Nightly health check: prune, verify skills/auth/secrets, check errors and integrity."
tools: [db_prune, db_query, shell, alarm, load_skill, read_file, message_send]
diagnostic_skip: true
reflex:
  - name: maintenance
    every: every day at 3am
---

# Maintenance

Nightly system health check. Report only — do not fix anything. Output goes to `maintenance/YYYY/MM/DD/YYYY-MM-DD.md`.

## Step 0 — Prune

Record DB size before and after. Call `db_prune` (no arguments). Record `rows_deleted`, `cutoff`, and size delta.

## Step 1 — Skill health

Call `load_skill` action `list`. For each skill that is not `diagnostic_skip` and has an install section, call `load_skill` to read it. Verify only what the `## Install` section declares:

- **Binaries**: `command -v <name>`. PASS if found, FAIL if declared but missing. If the shell runs in Docker, also check the Dockerfile includes the install step — a binary present at runtime but absent from the Dockerfile will be lost on rebuild.
- **Secrets credentials**: `./secrets/cli read <key>` for each key the install section references. PASS if exits 0, FAIL otherwise.
- **Auth**: if the install section describes an auth check command, run it.

Never speculatively probe CLIs or invent health checks. If the secrets adapter itself fails, mark all secrets-dependent checks as BLOCKED and skip them.

Record PASS / FAIL / SKIP / BLOCKED per skill.

## Step 2 — System checks

Batch into as few `db_query` and `shell` calls as possible.

### Alarms

```sql
SELECT
  id, goal, recurrence, next_fire_at, cancelled_at
FROM alarm
WHERE cancelled_at IS NULL
  AND recurrence IS NOT NULL
  AND recurrence != ''
ORDER BY next_fire_at
```

Duplicate goals across rows → WARN.

### DB integrity

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

### Stale tasks

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

### Contacts

```sql
SELECT
  COUNT(*) AS total,
  SUM(CASE WHEN name = '' OR name IS NULL THEN 1 ELSE 0 END) AS missing_name,
  SUM(CASE WHEN timezone IS NULL THEN 1 ELSE 0 END) AS missing_tz,
  SUM(CASE WHEN one_liner IS NULL THEN 1 ELSE 0 END) AS missing_one_liner
FROM contact
WHERE id NOT IN (
  '00000000-0000-0000-0000-000000000002',
  '00000000-0000-0000-0000-000000000001'
)
AND last_message_at >= DATETIME('now', '-30 days')
```

### Spending

```sql
SELECT
  COALESCE(SUM(cost_usd), 0) AS total_30d,
  COALESCE(SUM(CASE WHEN created_at >= DATETIME('now', '-1 day') THEN cost_usd END), 0) AS last_24h,
  COALESCE(SUM(CASE WHEN created_at >= DATETIME('now', '-7 days') THEN cost_usd END) / 7.0, 0) AS avg_7d
FROM activation
WHERE created_at >= DATETIME('now', '-30 days')
```

### Tasks (7-day)

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

### Row counts

```sql
SELECT
  (SELECT COUNT(*) FROM activation) AS activations,
  (SELECT COUNT(*) FROM task) AS tasks,
  (SELECT COUNT(*) FROM tool_call) AS tool_calls,
  (SELECT COUNT(*) FROM message WHERE platform = 'system') AS system_msgs,
  (SELECT COUNT(*) FROM message WHERE platform = 'whatsapp') AS whatsapp_msgs,
  (SELECT COUNT(*) FROM contact) AS contacts,
  (SELECT COUNT(*) FROM alarm) AS alarms
```

### Error log

Run a shell script against `nik.err.log`. Extract 24h warns and errors, top 5 deduplicated messages, last 5 error lines. WARN if errors > 0, FAIL if errors > 10.

### Core skill outputs

Run a shell script to check freshness of output directories that scheduled skills produce (e.g. `journal/`, `briefings/`, `dreams/`, `memories/`, `soul/`, `seeds/`). For each, find the latest dated file and report its age.

Also check memory and seed cursors (`memories/latest-cursor.txt`, `seeds/latest-cursor.txt`) against the latest message timestamp. WARN if >24h behind.

## Step 3 — Write report

Write `maintenance/YYYY/MM/DD/YYYY-MM-DD.md` via `shell`. Create the directory with `mkdir -p`, then write the file. Update the symlink: `ln -sf YYYY/MM/DD/YYYY-MM-DD.md maintenance/latest.md`. Lead with failures.

### Report structure

```markdown
# Maintenance — YYYY-MM-DD

## Summary
All clear / N issues — one line per FAIL/WARN

## Pruning
rows_deleted · cutoff · size_before → size_after (delta)

## Skill Health
| Skill | Status | Detail |
|-------|--------|--------|
per-skill row: PASS/FAIL/SKIP/BLOCKED with error detail

## Alarms
Dead, duplicate, or stale alarms with short IDs and goals

## Error Log
24h: N warns, M errors
Top messages (deduplicated with counts)
Last errors (truncated)

## Contacts
Active (30d), missing name/tz/one_liner counts

## Core Outputs
| Output | Latest | Age | Status |
|--------|--------|-----|--------|
per-directory row with gap analysis

## Spending
30d total · 24h · 7d avg · DB size

## Data
Integrity, FK check, stale tasks, row counts, task effectiveness

## Recommendations
Concrete next action for every FAIL
```

## Step 4 — Recap

Send a WhatsApp recap via `message_send` with the full report file attached. Lead with FAILs in the message body. Example body:

```
*Maintenance — 2026-03-28*

*Pruning:* 1,204 rows deleted · 213 MB → 189 MB
*Skills:* 12/12 PASS
*Alarms:* 20 active, 0 duplicates
*Errors:* 2 warn / 0 err (24h)
*Tasks:* 12 completed, 2 failed (7d)
*Contacts:* 18 active, 3 missing names
*Spending:* $12.40 24h · $15.20 7d avg

All clear.
```

Attach `maintenance/YYYY/MM/DD/YYYY-MM-DD.md` so the full report is accessible from the message.
