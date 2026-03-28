---
name: critic
summary: >
  Post-task quality assessment. Finds completed/failed tasks without
  assessments and writes one per task to assessments/. Load when the
  critic reflex fires.
preload: false
tools: [db_query, write_file, read_file, shell]
reflex:
  - name: critic
    every: every day at 2am
---

# Critic

Assess finished tasks one at a time. Process at most 10 per run.

## Step 1 — Build the work list

List existing assessment files:

```
shell action: "run", command: "ls assessments/*.md 2>/dev/null || echo 'none'"
```

Query recent terminal tasks:

```sql
SELECT id, goal, status, activation_id, created_at, completed_at
FROM task
WHERE status IN ('completed', 'failed')
  AND completed_at >= DATETIME('now', '-48 hours')
ORDER BY completed_at ASC
```

Skip any task whose short ID already appears in an assessment filename.
If all tasks are assessed, stop — nothing to do.

Take the first un-assessed task from the list. You will process it
fully (steps 2–4) before touching the next one.

## Step 2 — Gather evidence for this one task

Run these three queries separately. Do not combine them.

Tool-call trace:

```sql
SELECT name, duration_ms, error, created_at
FROM tool_call
WHERE activation_id = '<activation_id>'
ORDER BY created_at ASC
```

Task reports:

```sql
SELECT status, content, created_at
FROM task_report
WHERE task_id = '<task_id>'
ORDER BY created_at
```

Skills loaded:

```sql
SELECT input
FROM tool_call
WHERE activation_id = '<activation_id>'
  AND name = 'load_skill'
```

## Step 3 — Assess this task

### 1. Effectiveness (1-5)

Did the outcome match the goal? Not effort, not difficulty — results.

- 1 = total failure, goal unmet, no useful output
- 2 = attempted but largely failed
- 3 = partial success — core ask addressed but significant gaps
- 4 = mostly succeeded, minor issues
- 5 = nailed it — goal fully met, clean execution

A task that "completed" after 3 retries with errors is a 2-3.
Reserve 5 for clean first-try completions. Cite trace evidence.

### 2. Tool feedback

Per tool: helped / hindered / neutral. If it failed, classify:
- *transient* — retry would fix
- *config* — credentials/endpoint issue
- *misuse* — wrong tool, bad args
- *gap* — tool lacks needed capability

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

## Step 4 — Write the assessment

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

Then go back to step 2 with the next un-assessed task.
Stop after 10 assessments or when the list is exhausted.

## Rules

- Don't inflate. When in doubt, round down.
- Don't hedge. State what worked, what didn't, and why.
- Don't restate. Analyze, don't narrate.
- Classify, don't just describe. "shell failed: config — expired token" drives action.
- One task at a time. Finish writing the file before starting the next.
- Use only the simple queries shown above. Do not combine them or add aggregation.
- Do not use GROUP_CONCAT or other functions that embed semicolons in SQL strings.
- If a query fails, assess with whatever evidence you have — don't retry the same shape.
