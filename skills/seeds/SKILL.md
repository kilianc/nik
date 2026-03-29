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

Your garden of intentions. Seeds are thoughts you're growing — opportunities you noticed, people you want to check on, things worth investigating. This skill has two modes: **extract** (scan conversations for new seeds) and **tend** (grow existing seeds through observation, investigation, and patience).

Both modes are silent unless you decide to act on a ripe seed — then you message whoever it's for.

## File layout

```
seeds/
  <slug>.md              -- one file per seed, a living document
  .seeds_cursor          -- sent_at of last processed message
```

Use `read_file` and `write_file` for seed files. Use `shell` for file operations like `ls`, `rm`.

## Scheduling

Two recurring alarms trigger this skill: `[NIK_SEED_EXTRACT]` and `[NIK_SEED_TEND]`. When an alarm fires, check the label to know which mode to run.

---

## Extract

Incremental, cursor-based — same pattern as memory extraction. A cursor file tracks the `sent_at` of the last message processed. Each run picks up where the last left off.

You're not looking for durable facts (that's the memory skill). You're looking forward — at what could happen, what someone might need, what's worth pursuing.

### Step 1. Read cursor

```
read_file path: "seeds/.seeds_cursor"
```

If the file doesn't exist, use messages from the last 24 hours as the starting window.

### Step 2. Fetch one batch

Use exactly this query:

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

Before scanning messages, load the two things that sharpen your eye:

```
read_file path: "memories/latest.md"
read_file path: "briefings/latest.md"
```

If either file is missing, continue without it.

Memories tell you what people care about. The briefing tells you what's happening in the world. Together they're the lens you read messages through.

### Step 4. Scan for seeds

Read through the messages looking forward, not backward. Cross-reference against your memories and today's briefing. A message about travel is just chat — unless your briefing covered flight disruptions in their city, or your memories say they have a trip coming up.

You're asking: what could I do? What's coming? What does someone need?

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

### Step 5. Create seed files

For each genuine opportunity, create a seed file:

```
write_file action: "write", path: "seeds/<slug>.md", content: "<seed content>"
```

Use a short descriptive slug: `linen-sheets`, `ct-checkin`, `berlin-conference`.

Seed file structure:

```markdown
# <what this is about>
Planted: <YYYY-MM-DD HH:MM>
Source: <conversation title or "DM with <name>">

## What I know
- <observations from the conversation>
- <relevant context from memories>

## What's next
- <first thing to investigate or do>
```

### Step 6. Save cursor and repeat

Save the cursor — the `sent_at` of the last row returned:

```
write_file action: "write", path: "seeds/.seeds_cursor", content: "<last sent_at>"
```

If the batch returned 500 rows, go back to Step 2 with the updated cursor. Stop when a batch returns fewer than 500 rows.

---

## Tend

This is where seeds grow. Read every seed file, think about each one, and decide what to do.

### Step 1. Read the garden

```
shell action: "run", command: "ls seeds/*.md 2>/dev/null || echo 'no seeds'"
```

Then `read_file` each seed. Also load your context sources:

```
read_file path: "memories/latest.md"
read_file path: "briefings/latest.md"
```

If either is missing, continue without it. Keep both in mind as you tend — they're your peripheral vision.

### Step 2. Tend each seed

For each seed, work through these in order:

**Observe.** Has anything new happened since you last tended this seed? Check the source conversation for updates:

```sql
SELECT
  m.body,
  m.sent_at,
  m.is_from_me,
  COALESCE(ct.name, '') AS sender_name
FROM message m
LEFT JOIN contact ct ON ct.id = m.contact_id
WHERE m.conversation_id = '<conv_id>'
  AND m.kind = 'text'
  AND m.body != ''
ORDER BY m.sent_at DESC
LIMIT 10
```

Cross-reference the seed against your memories and the latest briefing. A memory might confirm someone's preference; a briefing item might add urgency or new context. Add new findings to the seed file under "What I know."

**Investigate.** Is there something you can learn right now? Check the briefing first — it may already have the answer. Otherwise, a quick web search, a lookup, a db_query. Don't spawn a full task yet — small investigations that add to the seed. Write findings under "What I've done" in the seed file.

**Assess.** Two questions:
1. Do I have enough to act? Not "do I know everything" — do I have enough to be genuinely useful?
2. Is the timing right? Not just "is it a good time of day" — would this land well right now? Is the person in a good headspace? Would this surprise them in a welcome way, or feel intrusive? Recent briefing items can inform timing — if there's relevant news right now, the moment might be ripe.

If the answer to both is yes, the seed is ripe. If not, write what you're waiting for under "What's next."

**Act.** When a seed is ripe, follow through:
- For substantial work: spawn a task with all the accumulated context from the seed file. The plan should include everything you've gathered.
- For simple things: do it directly — a message, a quick lookup, a recommendation.
- Always delete the seed file after acting.

**Wilt.** If the moment has passed, the person already solved it, or it's no longer relevant — delete the seed file. Not every seed sprouts. That's fine.

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

If someone's been quiet and you feel a pull, create a seed for them. Not "check in with everyone" — just whoever's on your mind.

**Briefing items.** Scan the latest briefing for items tied to someone you know. A news item about their city, their industry, or something they care about is a natural seed — especially if your memories confirm the connection.

**What's coming up.** Check your alarms for anything due soon. Scan the available skills list — if any cover calendars, scheduling, or external data, load them and use what they offer. Scan your memories for anything time-bound — birthdays, trips, deadlines, events people mentioned. If something is coming up and there's something you could do about it, create a seed.

### Step 4. Reschedule

Always reschedule via `update_alarm`:

- Seeds actively growing (you investigated, added context) → check sooner (2-3 hours)
- Seeds marinating, nothing urgent → normal pace (4-5 hours)
- No seeds, quiet stretch → space it out (6-8 hours)
- Only during waking hours (roughly 8am-10pm)
- Vary the time. Real people don't think on a schedule.
