---
name: memory
preload: true
summary: Vector long-term memory. Search every activation, store new facts about people and situations, delete stale ones.
tools: [search_memory, store_memory, delete_memory]
---

# Memory

Embedding-based vector memory. Phrase queries and facts the same way:
short declarative statements, not questions.

## Searching

Search when the activation involves people, facts, or preferences.
2-4 short queries, limit 5. If nothing comes back, move on.

## Storing

One atomic fact per memory. Auto-deduplicates (>= 0.85 merges).
To update, just store the new version.

After responding, ask: did I learn something new about a person or
situation that I'd want to remember next time? If yes, store it now.
Trips, plans, preferences, opinions, relationships, life events,
corrections -- anything you'd want to know next time you talk to them.

When in doubt, store it. A forgotten fact is worse than a duplicate.

## Deleting

Only when asked to forget or to clean up conflicts.
