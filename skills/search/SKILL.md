---
name: search
summary: >
  Read-only SQL against nik's database. Load this for ad-hoc lookups,
  stats, and debugging. Owner-only.
tools: [db_query]
---

# Search (db_query)

Run a read-only SQL query against nik's SQLite database. Only `SELECT`,
`WITH`, `SHOW`, `DESCRIBE`, and `PRAGMA` statements are allowed.

- `query` -- the SQL query string
- Returns up to 50 rows. If truncated, the response includes
  `"truncated": true`.

## SQLite features available

- `jaro_winkler_similarity(a, b)` -- custom function for fuzzy string
  matching (returns 0-1 similarity score)
- JSON functions (`json_extract`, `json_each`, etc.) for array columns
  stored as JSON TEXT
- Standard SQL CTEs, window functions, aggregations

## Key tables

| Table | Purpose |
|---|---|
| `contact` | people nik knows (nicknames, emails, phone_numbers, whatsapp_ids as JSON arrays; one_liner, notes, timezone, location) |
| `conversation` | nik-owned chat identity + platform reference |
| `message` | normalized messages (body, contact_id, conversation_id, is_from_me, sent_at) |
| `media` | media cache (describe_text, transcript_text, local_path) |
| `message_media` | link table (message_id, media_id) |
| `alarm` | scheduled alarms (goal, fire_at, origin_contact_id, origin_conversation_id) |
| `memory` | stored memories (content, metadata) with vec_memory for embeddings |

All primary keys are UUIDv7 stored as TEXT.

## Tips

- This tool is **privileged** (owner-only).
- Array columns (nicknames, emails, etc.) are JSON arrays in TEXT
  columns. Use `json_each()` to unnest them.
- Use `PRAGMA table_info(<table>)` to inspect schema when unsure.
- Combine with `search_contacts` for people lookup -- `db_query` is
  better for complex joins and aggregations.
