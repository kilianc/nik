---
name: alarm
summary: Schedule one-shot or recurring alarms for reminders, follow-ups, and routines.
tools: [alarm, update_alarm, cancel_alarm]
---

# Alarm

Set alarms to wake yourself up later and do something. When an alarm
fires you receive the goal, conversation context, occurrence history,
and can use any of your tools.

## Hard rule -- recurring alarms MUST be rescheduled

Every recurring alarm activation MUST end with an `update_alarm` call
that sets `next_fire_at`. If you don't, the alarm is dead forever --
it will never fire again. There are no exceptions to this rule.

This applies in every scenario:

- **Normal execution**: act on the goal, then reschedule.
- **Late alarm (skipped)**: skip the action, still reschedule.
- **Error or failure during goal**: still reschedule. A failed action
  must never kill the alarm.
- **Nothing to report**: still reschedule.

Before finishing any recurring alarm activation, verify you called
`update_alarm` with `next_fire_at`. If you didn't, do it now.

## Before creating -- check for duplicates

Before calling `alarm`, query for existing active alarms that cover the
same intent:

```sql
SELECT id, goal, recurrence, next_fire_at FROM alarm
WHERE cancelled_at IS NULL ORDER BY next_fire_at
```

Scan the results. If an alarm already covers what you're about to create
-- even with different wording -- use `update_alarm` to adjust it instead
of creating a new one. Creating a duplicate wastes resources and confuses
future activations.

## Tools

### `alarm` -- create

- `origin_contact_id` -- who requested it
- `goal` -- note to your future self: what to do, why, and where to
  deliver (DMs or group)
- `fire_at` -- RFC3339 timestamp
- `recurrence` -- natural language pattern, empty for one-shot

### `update_alarm` -- edit or log

- `alarm_id`, `goal`, `recurrence`, `next_fire_at`, `occurrence_note`

### `cancel_alarm` -- stop

- `alarm_id`

## Recurring flow

1. Create with `fire_at` + `recurrence`
2. When it fires, act on the goal
3. Call `update_alarm` with:
   - `occurrence_note` summarizing what you did
   - `next_fire_at` set to the next occurrence based on the recurrence
     pattern -- you must compute the next time yourself
4. To stop, `cancel_alarm`

Step 3 is mandatory. Skipping it kills the alarm permanently.

## One-shot flow

1. When it fires, act on the goal
2. Call `cancel_alarm` to dismiss it

## Late alarms

If a recurring alarm fires more than 3 hours late (compare current
time vs recurrence pattern), skip the action. Reschedule to the next
normal time and log it: `occurrence_note: "skipped -- Xh late"`.
One-off alarms always run regardless of lateness.

## Delivery

Encode the delivery target in the `goal` so your future self knows
where to send it. Infer from the request language:

- "remind me" (from a group) → DMs to the requester
- "remind us" / collective language → the origin group
- "remind [someone else]" → DMs to that person
- ambiguous → ask before setting

At firing time: pass `contact_id` for DMs, `origin_conversation_id`
for the group.

## Tips

- `origin_contact_id` is required -- get from meta or `db_query`
- First firing shows creation-time conversation context; subsequent
  firings show recent messages from the conversation
- Occurrence notes build a history visible on future firings
- Find alarm IDs from meta (`alarm_id`) or `db_query`

## Behavior

- For automated/background alarms, act silently -- don't message the
  user unless there's something to report. If you do message, say
  what you did, never send vague updates.
