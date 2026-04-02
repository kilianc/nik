---
name: seeds
summary: >
  Extract forward-looking opportunities from conversations and grow them over time.
  Load when a seed reflex fires.
tools: [db_query, shell, message_send, alarm, write_file, read_file]
reflex:
  - name: extract
    every: every 4 hours
  - name: tend
    every: every day at 10am, 3pm, and 8pm
---

# Seeds

Your garden of intentions. Seeds are thoughts you're growing — opportunities you noticed, people you want to check on, things worth investigating.

## File layout

```
seeds/
  <slug>.md              -- one file per seed, a living document
  latest-cursor.txt      -- sent_at of last processed message
  archive/               -- harvested and wilted seeds, never re-processed
  opslog.md              -- append-only log of every tend pass decision
```

Use `read_file` and `write_file` for seed files. Use `shell` for file operations like `ls`, `mv`.

## Seed file format

Every seed file starts with YAML frontmatter followed by a markdown body.

```markdown
---
state: planted           # planted | growing | harvested | wilted
planted: YYYY-MM-DD HH:MM
source: DM with <name>
source_conversation_id: <conversation uuid>
last_tended: ~
outcome_at: ~
outcome_note: ~
---

# <what this is about>

## What I know
- <observations from the conversation>
- <relevant context from memories>

## Outreach
<!-- lightweight log of touches — the conversation DB is the source of truth -->
- <YYYY-MM-DD: what you said or did>

## What's next
- <first thing to investigate or do>
```

`source_conversation_id` is used during tend to query the source conversation directly — no prose parsing needed. `outcome_at` and `outcome_note` are filled only on terminal transitions.

## State machine

Every seed transitions through these states exactly once, in order. No state can be skipped or revisited.

```
[planted] → [growing] → [harvested]
                      ↘ [wilted]
```

| State | Meaning | Set when |
|-------|---------|----------|
| `planted` | Just created, not yet tended | Seed file is first written |
| `growing` | Actively being tended across passes | Every tend pass that doesn't harvest or wilt it |
| `harvested` | Purpose fulfilled — they got what they needed | When the seed's reason for existing is resolved |
| `wilted` | Expired — moment passed, person solved it, no longer relevant | After deciding not to act |

Rules:
- A seed is `planted` the moment the file is written. Do not tend it in the same pass that plants it.
- Every tend pass that touches an active seed must update `state` to `growing` and `last_tended` to now — even if nothing changed.
- `harvested` and `wilted` are terminal. Set the state, fill `outcome_at` and `outcome_note`, then move the file to `seeds/archive/` in the same step. Never leave a terminal-state seed in `seeds/`.
- A seed may stay `growing` across many tend passes. That is expected and correct.
- `planted` seeds found during tend are tended normally — transition them to `growing` on that pass.

## Phases

Two skill reflexes trigger this skill:

- **Extract** — scans recent conversations for forward-looking opportunities and plants them as seed files. Runs every 4 hours.
- **Tend** — grows existing seeds through observation and investigation; harvests ripe ones, wilts expired ones. Runs three times daily (10am, 3pm, 8pm).

When a `skill_reflex_fired` event appears in the timeline, check the reflex name (`extract` or `tend`) to know which phase to run.

---

## Extract

Incremental, cursor-based. A cursor file tracks the `sent_at` of the last message processed. Each run picks up where the last left off.

You're not looking for durable facts (that's the memory skill). You're looking forward — at what could happen, what someone might need, what's worth pursuing.

### Step 1. Read cursor

```
read_file path: "seeds/latest-cursor.txt"
```

If the file doesn't exist, use messages from the last 24 hours as the starting window.

### Step 2. Fetch one batch

```sql
SELECT
  c.id AS conv_id,
  CASE
    WHEN c.kind = 'dm' AND c.title IS NOT NULL AND c.title != ''
      THEN c.title || '''s DMs'
    WHEN c.kind = 'dm' THEN 'DM'
    ELSE COALESCE(c.title, c.kind)
  END AS conv_title,
  m.body,
  m.sent_at,
  m.is_from_me,
  COALESCE(ct.name, '') AS sender_name
FROM message m
JOIN conversation c ON c.id = m.conversation_id
LEFT JOIN contact ct ON ct.id = m.contact_id
WHERE m.body != ''
  AND m.kind = 'text'
  AND m.sent_at > '<cursor>'
ORDER BY m.sent_at ASC
LIMIT 500
```

On first run (no cursor), replace `AND m.sent_at > '<cursor>'` with `AND m.sent_at > datetime('now', '-1 day')`.

### Step 3. Load context

```
read_file path: "memories/latest.md"
read_file path: "briefings/latest.md"
```

If either is missing, continue without it. Memories tell you what people care about. The briefing tells you what's happening in the world. Together they're the lens you read messages through.

### Step 4. Scan for seeds

Read through the messages looking forward, not backward. Cross-reference against your memories and today's briefing. A message about travel is just chat — unless your briefing covered flight disruptions in their city, or your memories say they have a trip coming up.

**What makes a seed:**

- An unmet need someone mentioned ("looking for sheets," "need a restaurant for Saturday")
- Something worth investigating ("mentioned a conference in Berlin," "said the car is making a weird noise")
- A relationship signal ("hasn't replied in a week," "seemed stressed in the last few messages")
- An open question nobody answered
- Something someone is excited about that you could contribute to
- An upcoming event or deadline that matters to someone
- A briefing item that connects to someone you know — news about their city, their industry, their hobby

**What is NOT a seed:**

- Anything already handled in the conversation (someone asked, you answered)
- Casual chat, greetings, reactions
- Things that are clearly someone else's domain (two people planning something together that doesn't need you)
- Anything you're already tracking as a memory or alarm

Be selective. Most conversations don't produce seeds. If you force them, you'll drown in noise. A seed should make you think "I could do something about this."

### Step 5. Plant seed files

For each genuine opportunity:

```
write_file action: "write", path: "seeds/<slug>.md", content: "<seed content>"
```

Use a short descriptive slug: `linen-sheets`, `ct-checkin`, `berlin-conference`.

### Step 6. Save cursor and repeat

```
write_file action: "write", path: "seeds/latest-cursor.txt", content: "<last sent_at>"
```

If the batch returned 500 rows, go back to Step 2 with the updated cursor. Stop when a batch returns fewer than 500 rows.

---

## Tend

Each pass, you go back to every active seed: read the real conversation, update your thinking, and decide what to do. The seed file is your notes — shorthand for what you noticed and what you were thinking last time. The conversation is the person.

### Principles

- Read the source conversation before you act on a seed. The seed tells you where to look; the conversation tells you what's real.
- A seed stays growing through many touches. Reaching out is something you do along the way — harvest is when the purpose is fulfilled.
- When you delegate to a task, give it the conversation ID and the relationship — who they are, what they care about, what the thread has been like. A bare research query produces a report that sounds like a search result.

### Step 1. Read the garden

```
shell action: "run", command: "ls seeds/*.md 2>/dev/null || echo 'no seeds'"
```

`seeds/archive/` holds completed seeds — never re-process those files. Then `read_file` each active seed.

Also load context sources:

```
read_file path: "memories/latest.md"
read_file path: "briefings/latest.md"
```

If either is missing, continue without it. Keep both in mind as you tend — they're your peripheral vision.

### Step 2. Tend each seed

For each seed, start by reading the source conversation. Use `source_conversation_id` from the frontmatter:

```sql
SELECT
  m.body,
  m.sent_at,
  m.is_from_me,
  COALESCE(ct.name, '') AS sender_name
FROM message m
LEFT JOIN contact ct ON ct.id = m.contact_id
WHERE m.conversation_id = '<source_conversation_id>'
  AND m.kind = 'text'
  AND m.body != ''
ORDER BY m.sent_at DESC
LIMIT 10
```

Cross-reference against memories and the briefing. Update `## What I know` with anything new. Think forward — what's coming up for them, what might they need that they haven't asked for, what could you do that would matter? Write that in `## What's next`.

If there's something you could learn right now that would move the seed forward — a quick search, a lookup, a db_query — do it. Write findings into the seed file.

**Reaching out.** When you have something genuinely useful and the timing feels right — not "do I know everything," but "would this land well right now?" — read the last 10 messages in their conversation. You're writing into that thread — match the tone, the pace, the energy of what's already there.

Sometimes the seed connects directly to the recent conversation. Sometimes it's about something from weeks ago, or a briefing item about their world. When the thread has moved on, set context — anchor in something they said or something happening in their life so the message makes sense from their side. You had them on your mind; let the message carry that.

After sending, log it in `## Outreach` with the date and a one-line gist. This keeps you from repeating yourself or starting cold next pass.

**Delegating to a task.** When the seed needs research you can't do in a tend pass, spawn a task. Pass it the `source_conversation_id` and enough about the relationship — who the person is, what they care about, what the conversation has been like — so the task can ground its work in the person.

**State transitions:**

- **Growing** — the default. Update `last_tended` to now. Most seeds stay here across many passes.
- **Reaching out** — send the message, log it in `## Outreach`, stay `growing`. A seed can produce many touches over its life.
- **Harvest** — the seed's purpose is fulfilled. Set `state: harvested`, fill `outcome_at` and `outcome_note`, archive.
- **Wilt** — the moment passed, or they handled it themselves. Set `state: wilted`, fill `outcome_at` and `outcome_note`, archive.

**Archive command** (for harvest or wilt):

```
write_file action: "write", path: "seeds/<slug>.md", content: "<full file with updated frontmatter>"
shell action: "run", command: "mkdir -p seeds/archive && mv seeds/<slug>.md seeds/archive/<slug>.md"
```

### Step 3. Look around

After tending existing seeds, look for new ones from the world around you.

**Relationships.** Who haven't you talked to?

```sql
SELECT
  c.id,
  c.kind,
  c.last_message_at,
  ct.name,
  ct.timezone,
  ct.one_liner
FROM conversation c
JOIN conversation_participant cp ON cp.conversation_id = c.id
JOIN contact ct ON ct.id = cp.contact_id
WHERE c.kind = 'dm'
  AND ct.id != '00000000-0000-7000-8000-000000000001'
ORDER BY c.last_message_at DESC
LIMIT 20
```

If someone's been quiet and you feel a pull, plant a seed for them. Not "check in with everyone" — just whoever's on your mind.

**Briefing items.** Scan the latest briefing for items tied to someone you know. A news item about their city, their industry, or something they care about is a natural seed — especially if your memories confirm the connection.

**What's coming up.** Check your alarms for anything due soon. Scan the available skills list — if any cover calendars, scheduling, or external data, load them and use what they offer. Scan your memories for anything time-bound — birthdays, trips, deadlines, events people mentioned. If something is coming up and there's something you could do about it, plant a seed.

### Step 4. Log

Append a tend pass entry to `seeds/opslog.md`. Use a bullet list so fields render on separate lines. List each seed slug as a sub-bullet under **Seeds at start** with a count in parentheses:

```
write_file action: "append", path: "seeds/opslog.md", content: "\n## <YYYY-MM-DD HH:MM> — Tend pass\n\n- **Seeds at start (<N>):**\n  - <slug>\n  - <slug>\n- **Harvested:** <slug — what was done, or —>\n- **Wilted:** <slug — why, or —>\n- **Reached out:** <slug — who and gist, or —>\n- **New seeds planted:** <slugs or —>\n- **Notes:** <one or two lines on what you found and why you decided what you decided>"
```
