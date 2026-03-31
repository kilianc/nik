---
name: sql-hygiene
description: >-
  Enforce SQL query conventions for internal/queries/ and internal/db/. Use when
  creating, editing, or reviewing .sql files, embed.go, or Go query functions in
  the db package.
---

# SQL Query Hygiene

Checklist for working with `internal/queries/*.sql` and `internal/db/*.go`.

## SQL file rules

### One file per CRUD verb per entity

Each entity gets at most: one INSERT, one SELECT-single, one SELECT-list, one UPDATE, one DELETE `.sql` file. Use `COALESCE`/nullable params for field-level optionality. Only split when there is a real security or performance need (different WHERE-clause index patterns, fundamentally different return shapes).

Bad -- two UPDATE files for the same entity:

```sql
-- media_update_description.sql
UPDATE media SET describe_text = ?1 WHERE id = ?2;

-- media_update_transcript.sql
UPDATE media SET transcript_text = ?1 WHERE id = ?2;
```

Good -- single UPDATE with nullable params:

```sql
-- media_update.sql
UPDATE media
SET
  describe_text = COALESCE(?2, describe_text),
  described_at = COALESCE(NULLABLE_ISO8601_MS(?3), described_at),
  transcript_text = COALESCE(?4, transcript_text),
  transcribed_at = COALESCE(NULLABLE_ISO8601_MS(?5), transcribed_at),
  updated_at = NOW_ISO8601_MS()
WHERE id = ?1;
```

### Canonical prefixes

Filenames: `<entity>_<verb>[_qualifier].sql`. The entity name matches the table.

Good: `contact_get.sql`, `task_update.sql`, `alarm_occurrence_insert.sql`
Bad: `get_contact.sql`, `update_contact_field.sql`

### Formatting

- Two-space indentation everywhere (no 4-space alignment)
- One column per line in SELECT lists
- SET keyword on its own line, columns indented 2 spaces below
- Positional params: `?1`, `?2`, etc.
- `TEXT` not `VARCHAR`
- Singular table names

Correct UPDATE pattern:

```sql
UPDATE task
SET
  status = COALESCE(?2, status),
  activation_id = COALESCE(?3, activation_id),
  updated_at = NOW_ISO8601_MS()
WHERE id = ?1;
```

### Embedding

Every `.sql` file needs a corresponding `//go:embed` + exported var in `internal/queries/embed.go`. Var names are PascalCase derived from filenames. Group by entity with a comment header.

## Go function rules

### Naming

DB functions use entity prefix: `TaskGet`, `TaskList`, `TaskUpdate`, `ContactGet`, `ConversationParticipantList`. Avoid verb-first names like `GetContact` or `UpdateMedia`.

### Params structs

Any db function with 3+ domain params (excluding `ctx` and `db`) uses a Params struct: `TaskUpdateParams`, `MediaUpdateParams`. The struct lives in `models.go`.

### One function per entity operation

Never create `DoSomethingByX` / `DoSomethingByY` variants. Use a single function with a params struct, dispatch internally based on which fields are set.

## Checklist

When adding or editing queries:

1. Does a `.sql` file already exist for this entity + CRUD verb? Extend it instead of creating a new one.
2. Is the filename `<entity>_<verb>.sql`?
3. Two-space indentation, one column per line?
4. Corresponding `//go:embed` in `embed.go`?
5. Go function named `<Entity><Verb>`?
6. 3+ domain params? Use a Params struct.
7. Run `make lint && make test` after changes.
