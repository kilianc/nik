---
name: critic
summary: >
  Batch post-task quality assessment. Finds completed/failed tasks without
  assessments and writes one per task to assessments/. Load when the
  critic reflex fires.
preload: false
tools: [db_query, write_file, read_file, shell]
reflex:
  - name: critic
    every: every day at 2am
---

# Critic

Batch assessment of finished tasks. When this skill loads, find all
completed or failed tasks that don't have an assessment file yet and
assess each one.

## Step 1 — Find un-assessed tasks

List existing assessment files:

```
shell action: "run", command: "ls assessments/*.md 2>/dev/null || echo 'none'"
```

Query recent terminal tasks:

```sql
SELECT id, goal, status, plan, activation_id, created_at, completed_at
FROM task
WHERE status IN ('completed', 'failed')
  AND completed_at >= DATETIME('now', '-48 hours')
ORDER BY completed_at DESC
```

Skip any task whose short ID already appears in an assessment filename.
If all tasks are assessed, stop — nothing to do.

## Step 2 — Gather context per task

For each un-assessed task, pull the tool-call trace and reports:

```sql
SELECT tc.name, COALESCE(ar.round, 0) AS round,
  tc.duration_ms, tc.error, tc.created_at
FROM tool_call tc
LEFT JOIN activation_round ar ON ar.id = tc.activation_round_id
WHERE tc.activation_id = '<activation_id>'
ORDER BY tc.created_at ASC
```

```sql
SELECT id, status, content, created_at
FROM task_report WHERE task_id = '<task_id>' ORDER BY created_at
```

## Step 3 — Assess each task

### 1. Effectiveness (1-5)

Did the outcome match the goal? Not effort, not difficulty -- results.

- 1 = total failure, goal unmet, no useful output
- 2 = attempted but largely failed
- 3 = partial success -- core ask addressed but significant gaps
- 4 = mostly succeeded, minor issues
- 5 = nailed it -- goal fully met, clean execution

A task that "completed" after 3 retries with errors is a 2-3.
Reserve 5 for clean first-try completions. Cite trace evidence.

### 2. Tool feedback

Per tool: helped / hindered / neutral. If it failed, classify:
- *transient* -- retry would fix
- *config* -- credentials/endpoint issue
- *misuse* -- wrong tool, bad args
- *gap* -- tool lacks needed capability

Were there tools that should have been used but weren't?

### 3. Skill feedback

Per skill loaded: was it useful? If loaded but unused, why?
Were there skills that should have been loaded but weren't?

### 4. Recommendations

Name the exact tool, skill, parameter, or behavior. What doesn't
exist yet? What needs a new parameter or better error message?
"None" is valid.

### 5. Duration

Estimate expected seconds, compare to observed. Flag any single
tool call consuming >30% of total as a bottleneck (avoidable vs
inherent).

## Step 4 — Write assessments

Use `write_file` to create `assessments/<YYYY-MM-DD>-<task-short-id>.md`:

```markdown
# Assessment — <task short id>

**Goal:** <goal>
**Status:** <status>
**Effectiveness:** <score>/5 — <1-2 sentence justification>
**Duration:** <observed>s (expected ~<estimate>s) — <explanation>

## Tool feedback
<per-tool verdicts>

## Skill feedback
<per-skill verdicts>

## Recommendations
<concrete improvements or "none">
```

## Rules

- Don't inflate. When in doubt, round down.
- Don't hedge. State what worked, what didn't, and why.
- Don't restate. Analyze, don't narrate.
- Classify, don't just describe. "shell failed: config -- expired token" drives action.
