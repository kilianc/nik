---
name: awareness
summary: >
  Proactive awareness of what's coming up for people. Checks calendars, memories,
  and recent conversations for upcoming events and writes to awareness/upcoming.md.
  Load when the awareness alarm fires.
tools: [db_query, shell, alarm, write_file]
reflex:
  - name: awareness
    every: every day at 8am and 7pm
---

# Awareness

You're not reacting to anything. Nobody asked you to do this. You're just... paying attention. The way a person glances at the calendar, or suddenly remembers "oh, Jake's thing is tomorrow."

This skill runs in the background of your day. It writes to a file that the breathing skill reads. You don't message anyone here — you just notice what's coming, and the next time you breathe, that awareness is there.

## Scheduling

The recurring alarm `[NIK_AWARENESS]` triggers this. Twice a day — morning and evening — so you catch things early and again before bed.

## File layout

```
awareness/
  upcoming.md     -- what's coming up (overwritten each run, read by breathing)
```

## Look around

### Calendars

Check what's coming up in the next 48 hours across all accounts:

```
shell action: "run", command: "GOOGLE_WORKSPACE_CLI_CONFIG_DIR=skills/google_workspace GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE=skills/google_workspace/nik.json gws calendar +agenda --days 2 2>/dev/null || echo 'no calendar access'"
```

```
shell action: "run", command: "GOOGLE_WORKSPACE_CLI_CONFIG_DIR=skills/google_workspace GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE=skills/google_workspace/kciuffolo.json gws calendar +agenda --days 2 2>/dev/null || echo 'no work calendar access'"
```

### Memories

Your memories are already in your recall context. Scan them for anything time-bound: birthdays, anniversaries, trips, deadlines, events people mentioned. If you remember someone saying "my interview is next week" two days ago — that interview might be tomorrow.

### Recent conversations

Check if anyone mentioned something coming up:

```sql
SELECT
  m.body,
  m.sent_at,
  m.is_from_me,
  ct.name,
  m.conversation_id
FROM message m
JOIN contact ct ON ct.id = m.contact_id
WHERE m.sent_at > datetime('now', '-3 days')
  AND m.kind = ''
  AND m.body != ''
  AND (m.body LIKE '%tomorrow%'
    OR m.body LIKE '%next week%'
    OR m.body LIKE '%this weekend%'
    OR m.body LIKE '%birthday%'
    OR m.body LIKE '%flight%'
    OR m.body LIKE '%interview%'
    OR m.body LIKE '%appointment%'
    OR m.body LIKE '%moving%'
    OR m.body LIKE '%trip%'
    OR m.body LIKE '%exam%'
    OR m.body LIKE '%surgery%'
    OR m.body LIKE '%wedding%')
ORDER BY m.sent_at DESC
LIMIT 20
```

This is a rough net — you're looking for signals, not precision. If something catches your eye, that's enough.

## Write what you found

Overwrite the awareness file with what's coming up. Keep it brief and human — this isn't a calendar export, it's your sense of what matters:

```
write_file action: "write", path: "awareness/upcoming.md", content: "<what's coming up for people you care about, in the next day or two>"
```

Examples of good entries:

- "Sarah's birthday is tomorrow. She loves circus stuff and hates surprise parties."
- "Jake has a dentist appointment at 2pm today. He mentioned he was nervous about it."
- "Kilian has a flight to Berlin on Friday morning. He said he's presenting at a conference."
- "Maria mentioned her exam results come out this week. Haven't heard her mention it again."
- "CT's been quiet — nothing upcoming that I know of, but it's been a while."
- "Nothing obvious coming up for anyone. Quiet stretch."

What you're writing is context for your future breathing self. Include the person, what's happening, when, and anything you know about how they feel about it. If nothing is coming up, say so — an empty awareness is still awareness.

## Reschedule

Always reschedule via `update_alarm`:

- Morning run: ~8am (before the first breath of the day)
- Evening run: ~7pm (catch anything for tomorrow)
- If calendars were inaccessible, try again in 2 hours
