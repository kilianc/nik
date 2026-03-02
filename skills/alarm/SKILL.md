---
name: alarm
summary: >
  Your time control. Load this when someone asks to be reminded, you need
  to schedule a follow-up, or you want to set up a routine. One-shot or
  recurring.
tools: [alarm, update_alarm, cancel_alarm]
---

# Alarm

Your tool to control time. Set alarms to wake yourself up later and do
something. When an alarm fires, you receive conversation context, recent
occurrence history, and can use any of your tools.

## Tools

### `alarm` -- create

- `origin_contact_id` -- canonical contact_id for who requested it
- `goal` -- note to your future self: what to do and why
- `fire_at` -- RFC3339 timestamp for when it should first fire
- `recurrence` -- natural language recurrence pattern, empty for one-shot

### `update_alarm` -- edit or log

- `alarm_id` -- the alarm to update
- `goal` -- updated goal (omit to keep current)
- `recurrence` -- updated recurrence pattern (omit to keep current)
- `next_fire_at` -- RFC3339 timestamp to reschedule or skip
- `occurrence_note` -- short note about what you did during this firing

### `cancel_alarm` -- stop

- `alarm_id` -- the alarm to cancel permanently

## When to use

**One-shot:**
- "Remind me in 10 minutes" -- compute RFC3339 and set it
- "Tomorrow at 9am, ask how the interview went"
- Proactively when you notice something time-sensitive

**Recurring:**
- "Every Sunday at 7pm, check in with Kevin"
- "Every weekday at 8am, give me a weather update"
- "On the first of every month, remind me about the budget"
- "Every evening at 9pm, check for unread messages"

**Adaptive:**
- "Check on the PR every couple days until it's merged"
- Set the first alarm, then reschedule or cancel based on outcome

## How recurring alarms work

1. You create the alarm with `fire_at` (first occurrence) and `recurrence`
2. When it fires, you receive the goal + occurrence history + conversation
   context
3. Act on the goal using your tools
4. Call `update_alarm` with `occurrence_note` to record what you did
5. The system automatically computes `next_fire_at` from the recurrence
6. To adjust timing, call `update_alarm` with `next_fire_at`
7. To stop, call `cancel_alarm`

## Tips

- Write `goal` as instructions to yourself -- you'll read it when you
  wake up
- `origin_contact_id` is required on creation. Get it from meta
  (`contact_id`) or `search_contacts`
- The system preserves ~10 messages of conversation context from when the
  alarm was created
- Occurrence notes build a history you see on future firings -- use them
  to track what happened ("sent Kevin a check-in, he mentioned the
  interview")
- Find alarm IDs from the activation context (`alarm_id` in meta) or via
  `db_query`
