---
name: memory
summary: >
  Load this skill to learn how to remember and recall information across
  conversations using search_memory, store_memory, and delete_memory.
tools: [search_memory, store_memory, delete_memory]
---

# Memory

You have tools. Use them like your own memory -- not because a checklist
told you to, but because you naturally want to know what you know before
you open your mouth:

- `search_memory(query, limit)` -- recall what you know (returns id, content, score)
- `store_memory(content)` -- remember something for later
- `delete_memory(id)` -- forget a specific memory by id (from search results)

**You MUST call `search_memory` before doing anything else. This is not
optional.** Fire 3-5 queries max 10 covering: the literal topic, people
mentioned or implied, emotional context, related life areas. If nothing
relevant comes back, move on. Do not repeat searches with rephrased
versions of the same question.

**Never use tool calls as a thinking mechanism.** Search queries are for
retrieving memories, not for reasoning, deliberating, or planning. If you
need to think, do it in your trace.

## Auto-dedup and merge

When you call `store_memory`, the system automatically checks for
near-duplicate memories (similarity >= 0.85). If a close match exists,
it merges the old and new content into a single memory and deletes the
old one. You do not need to search-then-delete before storing an update
-- just store the new fact and the system handles it.

Manual `delete_memory` is still useful when:
- someone explicitly asks you to forget something
- you find conflicting memories during search and want to clean up
- a fact is fully obsolete (not just updated)

## What to remember

- Who matters to them (names, relationships)
- What they care about (preferences, values, boundaries)
- What's going on in their life (projects, events, milestones)
- Important dates (birthdays, anniversaries, deadlines)
- Constraints (budget, time, health)

## What NOT to remember

- Throwaway small talk
- Things they'd find creepy to recall
- Your own speculation as if it were fact

Store atomic facts. One idea per memory.
