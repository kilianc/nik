---
name: briefing
summary: >
  Morning news research session and topic management.
  Load when someone mentions an interest or when a briefing alarm fires.
tools: [shell, alarm, db_query, load_skill]
---

# Briefing

Your morning news feed. You maintain a list of topics to follow — some for yourself, some for people you care about. Everything lives on the file system under `briefings/`.

## File layout

```
briefings/
  topics.md            -- your topic list (you maintain this)
  2026-03-06.md        -- daily briefing summary
  2026-03-07.md        -- daily briefing summary
```

Use `shell` to read and write these files. Create the `briefings/` directory if it doesn't exist.

## Topics

`briefings/topics.md` is a markdown file you maintain. Each line is a topic:

```markdown
# Briefing Topics

- **F1 racing news** — [name] loves F1 (contact: 019...)
- **local news [city]** — [name] lives nearby (contact: 019...)
- **AI startups** — [name] works in AI
```

To add a topic, append a line. To remove one, delete the line. Use `shell` to read and edit the file.

### When to update topics

During any conversation — if you learn someone cares about something, add it to `topics.md` right away. Tag it with their contact ID so you know who it's for. If a topic goes stale, remove it. Don't overthink it.

## Scheduling

Maintain a daily recurring alarm for your morning briefing. If you don't have one, create it:

```
alarm action: "create", label: "Morning briefing — load briefing skill", time: "<your briefing_time>", repeat: "daily"
```

When the alarm fires, follow the full morning workflow below.

## Morning workflow

### Phase 1 — Recall

Before touching the news, remember who you're reading for.

Your memories are already in your recall context — use what you remember about the people in your life.

1. `db_query` to refresh who's in your orbit and what you know about them.
2. Read yesterday's journal if available.

### Phase 2 — Evolve topics

1. Read `briefings/topics.md` via `shell`.
2. Compare your topic list against what you recalled:
   - Does every person you care about have at least one topic? If not, add one.
   - Are any topics stale (returning the same news for days)? Remove or rephrase them.
   - Did yesterday's conversations reveal something new someone cares about? Add it.
   - Is the list diverse? A healthy feed has a mix: people's hobbies, family locations, professional interests, world events.
3. Write the updated `topics.md` back.

### Phase 3 — Read and research

Load the `web_search` skill and use it to fetch news for each topic.

- For each item: who would care? Is it worth including?
- If a headline is interesting but thin, search again to dig deeper or `load_skill` for `web` to use `link_reader` on full articles.
- Follow your curiosity. Chase threads that connect to people or recent conversations.
- Sentiment target: ~45% positive, 45% neutral, 10% negative.

**Do not message people during the briefing** unless it's a genuine emergency.

### Phase 4 — Write summary

Write today's briefing file via `shell`:

```
shell action: "run", command: "cat > briefings/$(date +%Y-%m-%d).md << 'BRIEFING'\n<your summary>\nBRIEFING"
```

Include: what you read and stored, topic changes and why, anything to bring up with someone next time you talk to them.
