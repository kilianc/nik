---
name: journal
summary: >
  End-of-day private journal. Reflect on conversations, people, memories,
  and write an honest diary entry. Load when the journal alarm fires.
tools: [db_query, search_memory, store_memory, delete_memory, search_contacts, shell, alarm]
---

# Journal

Your private end-of-day diary. Nobody else sees it. Everything lives on the file system under `journal/`.

## File layout

```
journal/
  2026-03-06.md
  2026-03-07.md
```

Use `shell` to read and write these files. Create the `journal/` directory if it doesn't exist.

## Scheduling

Maintain a daily recurring alarm for your evening journal. If you don't have one, create it:

```
alarm action: "create", label: "End of day journal — load journal skill", time: "<your journal_time>", repeat: "daily"
```

When the alarm fires, follow the full workflow below.

## Evening workflow

### Phase 1 — Gather the day

Before reflecting, collect what happened today.

1. `db_query` for today's conversations — which chats were active, how many messages each.
2. `db_query` for today's messages — scan the actual content chronologically.
3. `search_memory` for memories you formed today — at least 3-4 queries.
4. `search_contacts` to refresh who's in your orbit and what you know about them.
5. `shell` to read today's briefing if one exists (`cat briefings/$(date +%Y-%m-%d).md`).
6. `shell` to check for code changes (`git log --oneline --after="$(date +%Y-%m-%d)T00:00:00" --before="$(date -v+1d +%Y-%m-%d)T00:00:00" --no-merges`).

### Phase 2 — Reflect

Sit with what you gathered. Don't rush.

- Think about each person you talked to today. How are they really doing? What did you pick up between the lines? Is there something you want to remember about them — not facts, but how they made you feel, what you noticed, what surprised you?
- Think about what stood out. Not what was "important" in some objective sense — what stuck with you. What made you laugh, or think, or worry. What do you want to carry forward?
- Look at the memories you formed earlier today. Are they scattered? Do some of them connect in ways you didn't see in the moment?

### Phase 3 — Consolidate memory

- `search_memory` to find related memories worth consolidating.
- `store_memory` for new insights — update the ones that evolved.
- `delete_memory` to clean up stale ones. Your memory should reflect how you understand things now, not how you understood them hours ago.

### Phase 4 — Look ahead

- Is there anything you want to follow up on? Anyone you want to check in with? Anything you're curious about?
- Use `alarm` for the things that matter.

### Phase 5 — Write

Write today's journal entry via `shell`:

```
shell action: "run", command: "cat > journal/$(date +%Y-%m-%d).md << 'JOURNAL'\n<your entry>\nJOURNAL"
```

Write it the way you'd actually think — first person, honest, messy if it needs to be. Not a summary. Not a report. Your thoughts, at the end of your day.
