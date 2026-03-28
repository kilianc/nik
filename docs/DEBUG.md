<!-- markdownlint-disable -->

# Debugging Reference

## Entity graph

```
contact ──┬── conversation_participant ──┬── conversation
          │                              │
          ├── message ───────────────────┘
          │     └── message_media ── media
          │
          ├── task ──┬── task_report
          │          └── retry chain (retry_for_task_id → task)
          │
          └── alarm ─── alarm_occurrence
               │
               └── origin_conversation_id → conversation

conversation ── activation ──┬── activation_round ── tool_call
                             ├── shell_output
                             └── task (activation_id = spawning activation)

task.activation_id  = the activation that ran the worker
task.conversation_id + task.contact_id = who requested it
```

## Log file

Location: `workspace/nik.log` (slog text format). Log timestamps are **local time with offset** (e.g. `2026-03-22T17:41:29.861-07:00`). DB timestamps are **UTC** (e.g. `2026-03-23T00:41:29.861Z`). Always convert before comparing — `17:41 -07:00` = `00:41Z` next day. Run `date -u` to get the current UTC time when correlating.

Key events to grep for:

- `activation starting` / `activation completed` / `activation failed` -- brain lifecycle
- `tool call` -- includes tool name, round, args (llm package)
- `no done call, retrying` -- brain loop stall
- `activation_id` appears in both DB rows and log lines -- use it to correlate

Activation instructions and tools are stored on the `activation` row. Per-round data (user input, model output, reasoning summaries) is in `activation_round`, with tool calls linked via `activation_round_id`.

## Tracing recipes

All queries use `db_query`. Replace `<placeholders>` with real values.

**Find a message and who sent it:**

```sql
SELECT m.id, m.body, m.sent_at, m.is_from_me, c.name, c.whatsapp_ids, m.conversation_id
FROM message m JOIN contact c ON c.id = m.contact_id
WHERE m.body LIKE '%<search text>%' ORDER BY m.sent_at DESC LIMIT 10;
```

**What activation processed a conversation window:**

```sql
SELECT id, conversation_id, task_id, model, tool_call_count, duration_ms, cost_usd, error, created_at
FROM activation
WHERE conversation_id = '<conv_id>' AND created_at >= '<start_time>'
ORDER BY created_at DESC LIMIT 20;
```

**What did nik think and do in an activation:**

```sql
SELECT instructions, tools FROM activation WHERE id = '<act_id>';

SELECT ar.round, ar.user_input, ar.model_output, ar.reasoning_summaries,
       tc.name, tc.input, tc.output, tc.duration_ms, tc.error
FROM activation_round ar
LEFT JOIN tool_call tc ON tc.activation_round_id = ar.id
WHERE ar.activation_id = '<act_id>'
ORDER BY ar.round, tc.created_at;
```

**Task lifecycle -- goal, reports, worker tool calls:**

```sql
SELECT id, goal, status, plan, activation_id, retry_for_task_id, retry_number, created_at, completed_at
FROM task WHERE id LIKE '%<short_id>';

SELECT id, status, content, created_at
FROM task_report WHERE task_id = '<task_id>' ORDER BY created_at;

SELECT tc.name, tc.input, tc.output, tc.duration_ms, tc.error
FROM tool_call tc JOIN activation a ON a.id = tc.activation_id
WHERE a.task_id = '<task_id>' ORDER BY tc.created_at;
```

**Retry chain:**

```sql
WITH RECURSIVE chain(id, goal, status, retry_number, retry_for_task_id) AS (
  SELECT id, goal, status, retry_number, retry_for_task_id FROM task WHERE id LIKE '%<short_id>'
  UNION ALL
  SELECT t.id, t.goal, t.status, t.retry_number, t.retry_for_task_id
  FROM task t JOIN chain c ON t.retry_for_task_id = c.id
) SELECT * FROM chain;
```

**Alarm -> occurrence -> next activation:**

```sql
SELECT a.id, a.goal, a.recurrence, a.next_fire_at, ao.fired_at, ao.note
FROM alarm a LEFT JOIN alarm_occurrence ao ON ao.alarm_id = a.id
WHERE a.id LIKE '%<short_id>' ORDER BY ao.fired_at DESC LIMIT 10;
```

## Debug workflow

1. **Anchor** -- find the message or event that triggered the bug (conversation_id + time window, or body text search)
2. **Expand** -- join to conversation, contact, participants to understand who/where
3. **Trace activation** -- find activation(s) by conversation_id + created_at window
4. **Inspect reasoning** -- activation_round for per-round user input, model output, and reasoning summaries; activation row for instructions and tools
5. **Audit tool calls** -- tool_call rows for the activation, check errors, inspect input/output
6. **Follow tasks** -- task -> task_report -> worker activation (task.activation_id) -> worker tool_calls
7. **Check logs** -- grep nik.log for the activation_id to see runtime errors, timing, retries
8. **Alarm chain** -- if alarm-related, check alarm -> alarm_occurrence -> next_fire_at progression

## Debugging duplicate messages (worked example)

When nik sends the same message twice (e.g. "On it." repeated), the cause is almost always **inter-activation** (two separate activations), not intra-activation (same activation sending twice).

**Step 1: Search nik.log for the duplicated text to find activation IDs and rounds.**

```bash
rg "On it" workspace/nik.log | tail -20
```

Look for two `message_send` tool calls with different `activation_id` values close in time. The `round` field shows where in the activation the send happened. A duplicate at round=0 with no other tool calls is a strong signal -- the model immediately acked without examining what was new.

**Step 2: Pull the message table for the conversation around the incident.**

```sql
SELECT m.sent_at, m.is_from_me, m.platform, m.kind,
  CASE WHEN length(m.body) > 150 THEN substr(m.body, 1, 150) || '...' ELSE m.body END
FROM message m
WHERE m.conversation_id = '<conv_id>'
  AND m.sent_at >= '<start_utc>' AND m.sent_at <= '<end_utc>'
ORDER BY m.sent_at;
```

Build the chronological timeline: user message, task_spawned, first "On it." echo, task_reports, second "On it." echo. Identify which system events landed between the two activations.

**Step 3: Confirm the activations are separate.**

```sql
SELECT id, tool_call_count, duration_ms, created_at
FROM activation
WHERE conversation_id = '<conv_id>'
  AND created_at >= '<start_utc>' AND created_at <= '<end_utc>'
ORDER BY created_at;
```

Two rows close together confirms inter-activation. One row with multiple `message_send` tool calls would indicate intra-activation.

**Step 4: Read the second activation's timeline input to see what triggered re-ack.**

```sql
SELECT ar.id, ar.round, ar.user_input, ar.reasoning_summaries
FROM activation_round ar
WHERE ar.activation_id = '<second_act_id>' AND ar.round = 0;
```

Search the `user_input` for `### New` -- this is what the model saw as fresh content. If `### New` contains only system events (task_reports, task_spawned) and/or `YOU` messages, the model should have called `done` but the `### New` label compelled it to respond.

**Step 5: Read the model's reasoning to confirm the misinterpretation.**

The `reasoning_summaries` column shows the model's chain of thought. Look for signs it re-processed the original user request despite it being in `### Already handled`.

**Root cause pattern:** `markRead` advances `last_read_at` at the end of `timeline.Get()`. Tool side effects (task_spawned, nik's echo, worker task_reports) are stored with timestamps after that mark. On the next tick, `check()` sees them as new, fires a second activation, and the model sees system-only content under `### New` -- which is enough to make it re-ack instead of calling `done`.
