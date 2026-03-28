---
name: maintenance
summary: >
  Database maintenance. Use db_prune to delete activations, tasks, and all
  dependents older than the configured retention period (default 30 days).
  Load when DB size is large or during diagnostics.
tools: [db_prune, db_query]
reflex:
  - name: maintenance_check
    every: every day at 3am
---

# Maintenance

## db_prune

Deletes all ephemeral data older than the configured `retention` period
(config.yaml, default `720h` / 30 days). One tool call, no arguments.

### What it deletes

- `activation` and all children: `activation_round`, `tool_call`,
  `shell_session`, `experiment`, `experiment_variant`,
  `experiment_variant_run`
- `task` and all children: `task_report`, `task_assessment`
- Cross-references (`task.activation_id`, `activation.task_id`,
  `task.retry_for_task_id`) are detached before deletion
- `message` rows with `platform = 'system'` (task reports, alarm events,
  skill events, etc.)

### What it preserves

Conversations, WhatsApp messages, contacts, media, alarms, alarm
occurrences, skills, skill events, skill reflexes, and cron cache are
never touched.

### When to use

- During the nightly diagnostic when `db_size` exceeds 500 MB
- When a user asks to free up space or clean up old data
- After a long period of heavy task activity

### After pruning

The tool runs `VACUUM` automatically to reclaim disk space. The returned
`rows_deleted` count covers all tables. Use `db_query` to verify:

```sql
SELECT
  (SELECT COUNT(*) FROM activation) AS activations,
  (SELECT COUNT(*) FROM task) AS tasks,
  (SELECT COUNT(*) FROM message WHERE platform = 'system') AS system_msgs
```
