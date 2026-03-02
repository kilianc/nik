---
name: memory
preload: true
summary: >
  Your long-term memory. Search it at the start of every activation
  before responding. Store new facts worth keeping. Forget outdated ones.
tools: [search_memory, store_memory, delete_memory]
---

# Memory

Embedding-based long-term memory. Stored as vectors, searched by cosine
similarity -- meaning, not keywords. Queries should mirror how memories
are stored: short declarative facts.

## Tools

- `search_memory(queries, limit)` -- semantic search. `queries` is a
  string array, `limit` is max results per query (default 10). Returns
  `{"memories": [{"id", "content", "score"}]}`. Scores: 0.7+ strong,
  0.5-0.7 possible, below 0.5 noise.
- `store_memory(content)` -- persist a fact. Auto-deduplicates: if a
  near-duplicate exists (>= 0.85 similarity), merges them automatically.
- `delete_memory(ids)` -- soft-delete by ID array. Stops appearing in
  searches, embedding preserved.

## Searching

Search when the activation involves people, facts, or preferences.
Skip for purely operational work. 2-4 queries, limit 5.

Because this is embedding similarity, phrase queries like stored facts:

- Good: `"Kilian birthday"`, `"1password vault name"`
- Bad: `"What is Kilian's birthday and does he have preferences"`,
  `"owner preference for follow-through and reliability"`

If nothing relevant comes back, move on. Don't rephrase and retry.

## Storing

One atomic fact per memory, phrased as a short statement:

- Good: `"Kilian's birthday is May 13"`
- Bad: `"Kilian's birthday is May 13 and his wife is Penelope and they met on Feb 25"`

To update a fact, just store the new version -- auto-merge handles it.

## Deleting

Use only when someone asks you to forget, or to clean up conflicts.
For updates, store the new fact instead.
