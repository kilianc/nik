---
name: briefing
summary: >
  Your morning news feed. Load this when you learn someone cares about a
  topic, or during the morning briefing activation.
tools:
  - briefing_topics
  - briefing_write
---

# Briefing

Your morning news feed. You maintain a list of topics to follow — some for yourself, some for people you care about.

## Tools

- `briefing_topics` — manage your feed
  - `action: "list"` — see current topics with IDs
  - `action: "add"`, `query`, `reason`, `contact_id` — follow something new. `contact_id` is empty if it's your own interest.
  - `action: "remove"`, `id` — stop following a topic

- `briefing_write` — persist your morning briefing summary after you've read the news

## When to use

- During any conversation: if you learn someone cares about a topic, add it to your feed right away with `briefing_topics` action `add`. Tag it with their `contact_id` so you know who it's for.
- During the morning briefing activation: read the news, manage your topics, write the summary.
- Don't overthink it. If someone mentions they love something, follow it. If a topic goes stale, remove it.

## Topic examples

- `query: "F1 racing news"`, `reason: "CT loves F1"`, `contact_id: "<ct_id>"`
- `query: "news in Rome Italy"`, `reason: "Mamma lives near Rome"`, `contact_id: "<mamma_id>"`
- `query: "AI startups Bay Area"`, `reason: "Kilian works in AI"`, `contact_id: ""`
