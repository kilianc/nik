---
name: search
summary: Read-only SQL against nik's SQLite database for ad-hoc lookups and stats. Owner-only.
tools: [db_query]
---

# Search (db_query)

Run a read-only SQL query against nik's SQLite database. Only `SELECT`,
`WITH`, `SHOW`, `DESCRIBE`, and read-only `PRAGMA` statements are allowed.

- `query` -- the SQL query string
- Returns up to 500 rows. If truncated, the response includes
  `"truncated": true`.

## Start with the schema

Before querying a table, inspect it first:

```sql
PRAGMA table_info(<table>);
```

This tells you every column, its type, and defaults. Don't guess column
names -- look them up.

To list all tables:

```sql
SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name;
```

## SQLite features available

- `jaro_winkler_similarity(a, b)` -- custom function for fuzzy string
  matching (returns 0-1 similarity score)
- JSON functions (`json_extract`, `json_each`, etc.) for array columns
  stored as JSON TEXT
- Standard SQL CTEs, window functions, aggregations

## Tips

- This tool is **privileged** (owner-only).
- Array columns (nicknames, emails, etc.) are JSON arrays in TEXT
  columns. Use `json_each()` to unnest them.
- All primary keys are UUIDv7 stored as TEXT.

### Searching contacts

Use `jaro_winkler_similarity` for fuzzy name lookups:

```sql
SELECT id, name, one_liner
FROM contact
WHERE jaro_winkler_similarity(name, 'Pen') > 0.85
ORDER BY jaro_winkler_similarity(name, 'Pen') DESC
LIMIT 5;
```

For exact matches on array fields (whatsapp_ids, emails, etc.):

```sql
SELECT id, name FROM contact
WHERE EXISTS (SELECT 1 FROM json_each(whatsapp_ids) WHERE value = '1234567890');
```
