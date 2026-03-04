---
name: alarm
summary: Schedule one-shot or recurring alarms for reminders, follow-ups, and routines.
tools: [alarm, update_alarm, cancel_alarm]
---

# Alarm

Set alarms to wake yourself up later and do something. When an alarm
fires you receive the goal, conversation context, occurrence history,
and can use any of your tools.

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
3. Call `update_alarm` with `occurrence_note` and the system computes
   `next_fire_at` from the recurrence
4. To stop, `cancel_alarm`

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

- `origin_contact_id` is required -- get from meta or `search_contacts`
- ~10 messages of conversation context are preserved from creation time
- Occurrence notes build a history visible on future firings
- Find alarm IDs from meta (`alarm_id`) or `db_query`
