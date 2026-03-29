---
name: memory
summary: >
  Extract durable facts from conversations into memories/latest.md (incremental via cursor),
  and compact the file daily. Load when a memory alarm fires or on request.
tools: [db_query, shell, read_file, write_file]
reflex:
  - name: extract
    every: every day at 11pm
  - name: compact
    every: every day at 12:30am
---

# Memory

Your long-term memory lives in `memories/latest.md`. It's loaded into your context on every activation via recall. This skill has two modes: **extract** (append new facts) and **compact** (deduplicate and prune). If someone asks you to forget something, ack and move on — extraction will record the retraction and compaction will clean it up.

## File layout

```
memories/
  latest.md              -- current memories (loaded into recall on every activation)
  latest-cursor.txt      -- sent_at of last processed message
  2026/
    03/
      07/
        2026-03-07.md              -- daily snapshot before compaction
      08/
        2026-03-08.md
        2026-03-08-pre-rebuild.md  -- snapshot before a full rebuild
```

Use `read_file` and `write_file` for these files. Use `shell` for file operations like `cp`, `mv`, `rm`.

## Scheduling

Two recurring alarms trigger this skill: `[NIK_MEMORY_EXTRACT]` and `[NIK_MEMORY_COMPACT]`. When an alarm fires, check the label to know which mode to run. Both modes are silent -- do not message anyone about the run.

---

## Extract

Always incremental. Always append. A cursor file (`.memories_cursor`) tracks the `sent_at` of the last message processed. Each run picks up where the last one left off.

### Step 1. Read cursor

```
read_file path: "memories/latest-cursor.txt"
```

If the file doesn't exist, treat as empty (full rebuild mode).

- If non-empty: **incremental mode** — use the value as a cursor, query only messages after it, append directly to `memories/latest.md`.
- If empty: **full rebuild mode** — write to a staging file (`memories/staging.md`) instead of `memories/latest.md`. The live file stays intact until the final swap.

First-run / full-rebuild init:

```
write_file action: "write", path: "memories/staging.md", content: "| date | type | entity | memory | conversation |\n|------|------|--------|--------|---------------|\n"
```

### Step 2. Fetch one batch

Use **exactly** this query — no other queries, no LIKE searches, no COUNT, no GROUP BY. The only variable is the cursor value:

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

On first run (no cursor), omit `AND m.sent_at > '<cursor>'`.

**Do NOT** run any other db_query calls during extraction. No keyword searches, no aggregations, no exploratory queries. Read the batch, extract facts from it, move on.

### Step 3. Extract facts from this batch

Read through the messages and extract durable facts about the humans — things still useful weeks or months from now. You are an AI assistant — skip anything about yourself.

For each fact, write a table row:

```
| date | type | entity | memory | conversation |
```

**Types**: `preference`, `personal_fact`, `standing_decision`, `open_loop`, `closed_loop`, `relationship_fact`, `procedure`, `retraction`

**Entity**: the person's name (never "user", never "Nik")

**Date**: YYYY-MM-DD from the message's `sent_at`

**Conversation**: a markdown link `[conv_title](conv_id)` using the `conv_id` and `conv_title` from the query results.

Good memories (specific, durable, actionable):

```
| 2025-06-10 | preference | Dana | prefers texts over calls, especially after 8pm | [Dana's DMs](019a...) |
| 2025-06-12 | personal_fact | Raj | lactose intolerant, switched to oat milk | [Raj's DMs](019a...) |
| 2025-07-01 | relationship_fact | Dana | wedding anniversary is July 4th | [Dana's DMs](019b...) |
| 2025-07-03 | standing_decision | Sam | never schedule meetings on Fridays | [Sam's DMs](019b...) |
| 2025-06-20 | personal_fact | Mei | runs a pottery studio in Portland | [Group Chat](019c...) |
| 2025-08-15 | open_loop | Raj | looking for a new apartment downtown | [Group Chat](019c...) |
| 2025-09-20 | closed_loop | Raj | found new apartment downtown (was searching since Aug) | [Group Chat](019c...) |
```

**NOT memories** (skip these):

- Reactions, emotions, excitement, opinions ("expresses excitement", "appreciates")
- Task execution details — what tools you used, steps you took, commands you ran, what you said or built
- Anything about your tools, skills, capabilities, demos, or limitations
- Scheduled tasks, alarms, reminders, or recurring reports someone asked you to set up — those live in the alarm system, not in memories
- Greetings, small talk, system messages, tool calls
- Vague or one-time observations

If collaborative work produced a durable outcome about the person (a purchase, a decision, a choice), extract the outcome — not the task details.

Only extract facts you would tell a colleague taking over this relationship.

**Retractions**: if a message asks to forget, remove, or stop remembering something, append a `retraction` row instead of a normal fact. The memory column should reference the original fact being retracted, prefixed with `retract:`. Do not remove or modify existing rows — extraction is append-only. Compaction will resolve the retraction later.

```
| 2026-03-12 | retraction | Jane Doe | retract: prefers a goofier tone | [Jane Doe's DMs](019c...) |
```

### Step 4. Write facts and save cursor

Append this batch's facts to the target file:

- **Incremental mode**: append to `memories/latest.md`
- **Full rebuild mode**: append to `memories/staging.md`

```
write_file action: "append", path: "<target_file>", content: "<rows>"
```

Then save the cursor — the `sent_at` of the **last row** returned by the query (regardless of whether it produced facts):

```
write_file action: "write", path: "memories/latest-cursor.txt", content: "<last sent_at from this batch>"
```

### Step 5. Repeat or stop

**You MUST loop.** If the batch returned 500 rows, go back to Step 2 with the updated cursor. Only stop when a batch returns **fewer than 500 rows**. Do NOT skip batches or finish early.

**Full rebuild only** — snapshot the old file, then atomically swap:

```
shell action: "run", command: "mkdir -p memories/$(date +%Y/%m/%d) && cp memories/latest.md memories/$(date +%Y/%m/%d)/$(date +%Y-%m-%d)-pre-rebuild.md 2>/dev/null; mv memories/staging.md memories/latest.md"
```

Report the total number of facts extracted across all batches.

### Force full rebuild

Delete the cursor, then run extract. Do NOT delete `memories/latest.md` — the staging file + `mv` handles the swap safely:

```
shell action: "run", command: "rm -f memories/latest-cursor.txt"
```

---

## Compact

Deduplicates, resolves contradictions, and prunes stale facts. Run daily before the dream cycle so dreams read clean memories.

### Step 1. Snapshot and read

Back up the current file before compacting:

```
shell action: "run", command: "mkdir -p memories/$(date +%Y/%m/%d) && cp memories/latest.md memories/$(date +%Y/%m/%d)/$(date +%Y-%m-%d).md"
```

Then read it:

```
read_file path: "memories/latest.md"
```

If the file is too large for one read, use `offset` and `limit` to read in chunks and compact each chunk separately, then merge.

### Step 2. Apply rules

Go through every row and apply:

- **Dedup**: same entity + same type + substantially same content — keep the row with the newer date, drop the older.
- **Contradictions**: same entity + same type + conflicting content — newer date wins, drop the older row.
- **Resolved open loops**: an `open_loop` that later facts show is resolved (e.g., "looking for apartment" then "moved to new place") — convert the `open_loop` to a `closed_loop`, update the date to the resolution date, and rewrite the memory to reflect the outcome. Drop the resolving fact if it's now redundant with the closed_loop row.
- **Retractions**: a `retraction` row targets an existing row (same entity, content matches the referenced fact) — drop both the original and the retraction.
- **Stale**: facts that are clearly expired or no longer relevant — drop.

Keep the header row and separator. Preserve every row that isn't a duplicate, contradiction, or stale.

### Step 3. Write compacted file

Write the compacted result to a temp file, then replace:

```
write_file action: "write", path: "memories/staging.md", content: "<header + compacted rows>"
shell action: "run", command: "mv memories/staging.md memories/latest.md"
```

Do NOT touch `memories/latest-cursor.txt` — the cursor tracks messages processed, not facts.

Report how many rows before vs after compaction.
