---
name: sql-hygiene
description: >-
  Enforce SQL query conventions for internal/queries/ and internal/db/. Use when
  creating, editing, or reviewing .sql files, embed.go, or Go query functions in
  the db package.
---

# SQL Query Hygiene

Pre-flight workflow for `internal/queries/*.sql` and `internal/db/*.go`. Rules live in AGENTS.md under **Query embedding**, **DB layer**, and **SQL** sections -- read those first, this skill enforces them.

## Workflow

1. Read AGENTS.md SQL and DB layer sections for the current conventions.
2. Run the checklist below against every file you're adding or editing.
3. Run `make lint && make test` after changes.

## Checklist

When adding or editing queries:

1. Does a `.sql` file already exist for this entity + CRUD verb? Extend it instead of creating a new one.
2. Is the filename `<entity>_<verb>.sql`?
3. Two-space indentation, one column per line?
4. Corresponding `//go:embed` in `embed.go`?
5. Go function named `<Entity><Verb>`?
6. 3+ domain params? Use a Params struct in `models.go`.
7. Run `make lint && make test` after changes.
