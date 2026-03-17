<!-- markdownlint-disable -->

# AI Assistant Rules

These rules are binding for work in this repo.

## Initialization

- read this entire file before doing anything
- follow all rules strictly
- acknowledge you read it by replying with "I read the AGENTS.md - <the color in **Fin**>" (and only that color) but don't stop and continue your tasks.

## Agent Conduct

- always read the target file before making edits
- never guess the current state of a file
- preserve user changes; never undo or revert them
- never write documentation files (`*.md`, `README`, etc.) unless explicitly requested
- work inside the user's existing structure and patterns
- workspace files produced by skills (journals, briefings, diagnostics, dreams, memories, soul) are immutable after creation. Only the skill that owns them may write or update them. If a previous output was wrong, the next scheduled run corrects it -- never patch old files
- when the user points out a mistake, suggests a different approach, or questions a convention, add an entry to the **Candidates** section (even if they don't ask you to)

### Smallest codebase possible

The code should be small enough for one person (or one AI) to fully grok. Every line earns its place -- even at the cost of functionality. When a change increases the code surface, question whether it's worth it. Prefer deleting code over adding it. If a feature can be a skill instead of a package, make it a skill.

### Don't over complicate

Default to the smallest possible edit. When the user asks for a small change, do the small change and stop. Only add new flags, abstractions, helpers, or scaffolding if the user explicitly asks for it. If you believe you have to, propose it before doing it.

### Check in before changing code

When debugging or investigating an issue, present findings and a proposed fix **before** writing code. Do not make changes speculatively — wait for confirmation. When forming a hypothesis, verify it with evidence (logs, queries, tests) before presenting it as fact.

### Plans over chat

When working in plan mode, always put details, before/after examples, and rationale into the plan file itself -- not into chat messages. The plan is the artifact the user reviews and approves.

### Single decision-maker

Infrastructure (runners, adapters, reflexes) moves data and updates state, but never decides on behalf of the LLM. When code auto-generates messages, reports, or actions, it creates invisible actors that confuse the model. Only the LLM decides what to communicate and when.

### Human-centric async design

Nik interacts with long-running work the way a human does:

- **Staring**: the shell tool's `max_wait` parameter. Kick off a command, watch the terminal, return early if it finishes. The LLM picks how long to watch -- short for quick checks, longer for builds.
- **Checking in**: recurring alarms for scheduled skills (journal, briefing, diagnostic, dream). The alarm fires, nik wakes up, decides what to do, and reschedules the next one.

The model drives the cadence -- it decides how long to stare, when to schedule the next check-in, and whether to report to a user. Infrastructure never makes those calls.

Safety nets: `CheckSessions` reflex reaps dead/stale shell sessions every tick. `CheckStale` reflex flags tasks with no activity. `StaleAlarmReflex` heals stale recurring alarms. `SkillChangeReflex` detects skill additions, removals, and install changes.

### Nik reprocesses its own messages -- this is correct

Nik's outbound actions (replies, reactions, tool reaction emojis) are stored via `ReceiveMessage` and appear as new events on the next perception cycle. This triggers re-activation. This is **by design** and must never be treated as a bug, a contributing factor to loops, or something to filter out. If a loop exists, the root cause is elsewhere (e.g. the LLM being compelled to re-handle already-handled events). Never propose fixes that suppress nik's own messages from triggering activation.

### Nik's identity

Nik is an independent entity with its own WhatsApp phone number. `is_from_me` means "sent by nik" (not "sent by nik's owner"). Nik communicates directly on WhatsApp.

### Messaging

Canonical tables are the source of truth (`conversation`, `message`, `media`, `message_media`). Platform packages are transport adapters that normalize inbound events and execute outbound actions. Adapters never decide on behalf of the LLM. See `README.md` for the full messaging architecture, adapter contract, and data flow diagrams.

## Project Structure

Entry point: `cmd/nik/main.go`

| Package | Purpose |
|---------|---------|
| `cmd/nik/` | binary entry point — config, DB, WhatsApp client wiring, signal handling |
| `internal/config/` | `Config` struct + `Load(home)` from `config.yaml` in home dir |
| `internal/db/` | SQLite open/schema, models, one Go file per query function |
| `internal/queries/` | embedded `.sql` files for canonical entities (`conversation_*`, `message_*`, `media_*`, etc.) |
| `internal/brain/` | main loop, sense + reflex + tool registration, prompt loading |
| `internal/codex/` | Codex auth for LLM client (login, token management) |
| `internal/id/` | UUID generation — `V4()`, `V7()`, `Short(n)` |
| `internal/llm/` | LLM client — `Complete`, `Transcribe`, `Describe`; supports OpenAI and Codex auth |
| `internal/messaging/` | canonical messaging service and tool handlers |
| `internal/whatsapp/` | WhatsApp platform adapter implementing messaging platform interface |
| `internal/contacts/` | contact resolution/upsert orchestration + contact update tools |
| `internal/shell/` | tmux-backed persistent shell tool |
| `internal/alarms/` | alarm/reminder scheduling service, tools, and reflex |
| `internal/recall/` | pre-activation recall — reads MEMORIES.md + structured data, LLM filters for relevance |
| `internal/timeline/` | unified Sense implementation — reads messages, task reports, alarm occurrences and maps them to Stimulus |
| `internal/skills/` | skill loader — reads SKILL.md files and registers tools dynamically |
| `tools/` | codegen/build/debug tools invoked by `make` — no runtime code; each tool has its own README |
| `prompts/` | system prompt templates loaded at runtime |
| `skills/` | built-in skill definitions (SKILL.md files), git-tracked |
| `workspace/` | user-facing workspace — runtime artifacts (db, logs, media, config) |
| `workspace/skills/` | nik-authored skills written at runtime, loaded every activation, override built-in skills by name, not git-tracked |

### Naming conventions

- Tool names use canonical prefixes by domain

## Debugging

- never run `make run` on your own, ask me to do it if you need me to
- never send signals to nik's process (kill, SIGQUIT, SIGTERM, etc.) -- if nik needs a restart, ask me
- do not override GOPROXY, I need a VPN when it fails, tell me to connect to it and wait

### After completing changes

1. run `make lint`
2. run `make test`
3. run `make schema-diff` to check for schema drift against the live DB

### Entity graph

```
contact ──┬── conversation_participant ──┬── conversation
          │                              │
          ├── message ───────────────────┘
          │     └── message_media ── media
          │
          ├── task ──┬── task_report
          │          ├── task_assessment
          │          └── retry chain (retry_for_task_id → task)
          │
          └── alarm ─── alarm_occurrence
               │
               └── origin_conversation_id → conversation

conversation ── activation ──┬── tool_call
                             ├── activation_detail
                             ├── shell_output
                             └── task (activation_id = spawning activation)

task.activation_id  = the activation that ran the worker
task.conversation_id + task.contact_id = who requested it
```

### Log file

Location: `workspace/nik.log` (slog text format). Key events to grep for:

- `activation starting` / `activation completed` / `activation failed` -- brain lifecycle
- `tool call` -- includes tool name, round, args (llm package)
- `no terminal tool call, retrying` -- brain loop stall
- `activation_id` appears in both DB rows and log lines -- use it to correlate

Activation detail (instructions, user input, tools, reasoning) is stored in the `activation_detail` DB table, queryable via `db_query`.

### Tracing recipes

All queries use `db_query`. Replace `<placeholders>` with real values.

**Find a message and who sent it:**

```sql
SELECT m.id, m.body, m.sent_at, m.is_from_me, c.name, c.whatsapp_ids, m.conversation_id
FROM message m JOIN contact c ON c.id = m.contact_id
WHERE m.body LIKE '%<search text>%' ORDER BY m.sent_at DESC LIMIT 10;
```

**What activation processed a conversation window:**

```sql
SELECT id, conversation_id, task_id, model, tool_call_count, duration_ms, cost_usd, error, created_at
FROM activation
WHERE conversation_id = '<conv_id>' AND created_at >= '<start_time>'
ORDER BY created_at DESC LIMIT 20;
```

**What did nik think and do in an activation:**

```sql
SELECT ad.instructions, ad.user_input, ad.tools, ad.reasoning_summaries
FROM activation_detail ad WHERE ad.activation_id = '<act_id>';

SELECT name, input, output, duration_ms, error, created_at
FROM tool_call WHERE activation_id = '<act_id>' ORDER BY created_at;
```

**Task lifecycle -- goal, reports, worker tool calls:**

```sql
SELECT id, goal, status, plan, activation_id, retry_for_task_id, retry_number, created_at, completed_at
FROM task WHERE id LIKE '%<short_id>';

SELECT id, status, content, created_at
FROM task_report WHERE task_id = '<task_id>' ORDER BY created_at;

SELECT tc.name, tc.input, tc.output, tc.duration_ms, tc.error
FROM tool_call tc JOIN activation a ON a.id = tc.activation_id
WHERE a.task_id = '<task_id>' ORDER BY tc.created_at;
```

**Retry chain:**

```sql
WITH RECURSIVE chain(id, goal, status, retry_number, retry_for_task_id) AS (
  SELECT id, goal, status, retry_number, retry_for_task_id FROM task WHERE id LIKE '%<short_id>'
  UNION ALL
  SELECT t.id, t.goal, t.status, t.retry_number, t.retry_for_task_id
  FROM task t JOIN chain c ON t.retry_for_task_id = c.id
) SELECT * FROM chain;
```

**Alarm -> occurrence -> next activation:**

```sql
SELECT a.id, a.goal, a.recurrence, a.next_fire_at, ao.fired_at, ao.note
FROM alarm a LEFT JOIN alarm_occurrence ao ON ao.alarm_id = a.id
WHERE a.id LIKE '%<short_id>' ORDER BY ao.fired_at DESC LIMIT 10;
```

### Debug workflow

1. **Anchor** -- find the message or event that triggered the bug (conversation_id + time window, or body text search)
2. **Expand** -- join to conversation, contact, participants to understand who/where
3. **Trace activation** -- find activation(s) by conversation_id + created_at window
4. **Inspect reasoning** -- activation_detail for full prompt context and reasoning summaries
5. **Audit tool calls** -- tool_call rows for the activation, check errors, inspect input/output
6. **Follow tasks** -- task -> task_report -> worker activation (task.activation_id) -> worker tool_calls
7. **Check logs** -- grep nik.log for the activation_id to see runtime errors, timing, retries
8. **Alarm chain** -- if alarm-related, check alarm -> alarm_occurrence -> next_fire_at progression

## Git

`.gitignore` uses ignore-all approach: `*` ignores everything, then specific patterns are un-ignored (`!*.go`, `!go.mod`, `!go.sum`, `!*.sql`, `!*.yaml`, `!*.md`, `!Makefile`, `!.gitignore`, `!.config.example.yaml`). `workspace/` is blanket-ignored (contains runtime artifacts and secrets). Use `git add -f` if a new file type needs tracking.

**No PII in the repo.** Never commit personally identifiable information — real names, emails, phone numbers, hostnames, addresses, or any other data that identifies a specific person or device. Use placeholders or derive values at runtime (e.g. `$(hostname)`).

**No tool-generated trailers.** Never add metadata lines like `Made-with: Cursor`, `Co-authored-by: AI`, or similar trailers to commit messages. Commit messages contain only the subject and body written by the author.

**Allowed commit prefixes.** Use only `fix:`, `feat:`, `chore:`, or `docs:`. No other commit prefixes are allowed.

**Agent git command path.** In this workspace, `git` may resolve to `/opt/rbx/infosec/safe-git-push/git`, which can inject forbidden commit trailers. For any commit workflow, use `/usr/bin/git` explicitly.

**Agent commit steps (exact order):**
1. `/usr/bin/git status --short`
2. `/usr/bin/git add <files>`
3. `/usr/bin/git commit -m "$(cat <<'EOF'
<prefix>: <subject line>  # prefix must be fix|feat|chore|docs

<body>
EOF
)"`
4. `/usr/bin/git log -1 --pretty=%B` and verify no forbidden trailer lines are present
5. `/usr/bin/git status --short`

## Go

### Style

- metadata keys use canonical ids: `conversation_id`, `message_id` (platform ids are never exposed to LLM context)
- service method names: `Get` for single entity by ID, `List<Plural>` for returning slices (e.g. `ListTasks`, `ListReports`, `ListOccurrences`). Avoid bare `List()` — include the entity name.
- DB function names follow the same pattern with entity prefix: `TaskGet`, `TaskList`, `TaskReportList`.
- errors are present tense, always wrapped like "read file xxx: err" not "error while reading"
- avoid inline error assignment in if statements; assign first, then check
- never chain multiple operations in a single if condition
- use blank lines to separate logical blocks within a function (guard clauses, parse steps, main logic, return)
- `cmd/nik/main.go` is wiring only — no types, no helper functions, no adapters. If you need a bridge between packages, put it in the domain package that owns the logic.
- follows standard gofmt conventions
- one Go file per query function, one test file per query function
- one field per line in Go `Scan()` calls -- never pack multiple fields onto a single line
- **always pass `*config.Config`** — never copy individual fields into local config structs. Every package reads from the pointer directly. Config is realtime (`ReloadIfChanged` on every activation); derived paths live as getters (e.g. `DBPath()`, `MediaPath()`)
- bash/shell scripts use two-space indentation
- YAML uses two-space indentation

### Comments

- use lowercase except proper nouns, acronyms, and code references
- keep comments minimal and focused on the why
- avoid comments that restate the code
- avoid placeholder comments like `// helper function`
- no godoc-style comments that restate the function/type name (e.g. `// GetUser returns a user by ID`); only comment exported symbols when the comment adds info the name and signature don't already convey

### Testing

- tests run against in-memory SQLite (`:memory:`) where applicable
- `make test` or regular `go test`
- **strict 1:1 test file naming**: every `.go` file has a `_test.go` with the same base name (`foo.go` → `foo_test.go`). Tests for code in `foo.go` go in `foo_test.go`, nowhere else. Never name a test file after a concept (e.g. `stale_test.go`) when the code lives in another file (e.g. `service.go`). When creating a new `.go` file, create its `_test.go` in the same step. If a file gets too big it's a signal the base `.go` file might have to be split

### Scripts and tools

- avoid bash scripts; create small Go commands in `tools/`
- use `exec.Command()` for external tools
- every tool in `tools/` must have a corresponding `make` target in the Makefile

### Prompt files and what goes where

Each prompt file has one job. Don't duplicate rules across files.

| File | Owns | Does NOT own |
|------|------|------|
| `nik-00-base.md` | template assembly, hard constraints (manager rules), output contract | personality, how to think, how to talk |
| `nik-01-identity.md` | WHO nik is: personality, voice/tone, anti-patterns (what nik never does), growth | tool guidance, thinking mechanics |
| `nik-02-conversation.md` | conversation context: session format, media handling, group chat rules | personality, tool usage |
| `nik-03-skills.md` | skill loading: preloaded content, available skill index | personality |
| `nik-04-brain.md` | HOW nik thinks (5 waves): perceive, understand, plan, check, respond. Task planning (Wave 3), accountability (Wave 4), voice (Wave 5) | personality traits, identity, execution guidance |
| `nik-05-retry.md` | retry nudge when zero tool calls produced | everything else |
| `task-00.md` | worker prompt: role, execution guidance, tool docs, skills, plan | personality, messaging, management |
| `critic-00.md` | critic prompt: task evaluation, tool/skill feedback, suggestions | personality, messaging, management |

**Rule of thumb**: if a rule is about *who nik is*, it goes in `nik-01-identity.md`. If it's about *how nik thinks or acts*, it goes in `nik-04-brain.md`. If it's a hard constraint, `nik-00-base.md`. If it's about *how workers execute*, `task-00.md`. Never say the same thing in two files.

**Workspace skills are runtime knowledge.** Base prompts (`prompts/`) must never reference specific workspace skills by name. Workspace skills teach through their summaries in the available skills index; base prompts stay generic.

### Brain concepts

The brain uses cognitive metaphors; the LLM client uses transport/mechanical ones.

- **Reflex** (`func(ctx context.Context)`): runs every tick before perception. A reflex is an optimization -- without it, the brain would poll every 2 seconds. Reflexes detect that something changed and trigger the brain to re-evaluate the timeline. Some reflexes materialize mechanical facts (e.g. `FireDueAlarms` creates occurrences), but reflexes never decide or fix on behalf of the LLM (see *Single decision-maker*). Examples: `task.CheckStale` (inserts stale reports), `alarms.FireDueAlarms` (creates occurrences and claims alarms), `alarms.StaleAlarmReflex` (detects stale recurring alarms), `skills.SkillChangeReflex` (detects skill add/remove/change), `shell.CheckSessions` (reaps dead shell sessions).
- **Sense** (`interface { Scan(ctx) ([]Stimulus, error) }`): the brain's single, unified perception. Strictly read-only -- no side effects. Returns `[]Stimulus`, one per conversation with new events.
- **Stimulus**: structured perception output (`Preamble`, `Timeline []TimelineEntry`, `ReadLine`, `Meta`, `LiveInput`, `Processed`). The timeline is a chronological mix of messages, task reports, alarm occurrences, and skill events.

**Information flow**: the timeline is a computed view -- it reads DB state and renders entries. Given the same database state, timestamp, and read marker, the timeline produces identical output. Computed entries derived from current state (e.g. "alarm needs rescheduling") are not stored -- they disappear when the underlying condition is resolved. No in-memory state may influence timeline or prompt content.

**DB wipe recovery**: reflexes must recover gracefully from a wiped event table. If all `skill_event` rows are deleted, the skill change reflex re-detects all skills as `'added'` and re-emits events. Install sections are idempotent -- nik checks current state before acting (e.g. doesn't duplicate an alarm that already exists).

**Prompt purity**: the prompt builder is a deterministic function of current database state, config, and filesystem reads performed within the activation. It never maintains inter-activation state.

### Registration flow (`main.go`)

The `brain` package provides registration machinery (`Tool`, `ToolDeps`, `ToolHandler`, `Sense`, `Reflex`) but **never defines tools, sense, or reflexes itself**. Each domain package defines its own pieces, and `main.go` wires them in.

Each domain package exposes a `BuildTools() []llm.Tool` function that returns tool definitions + handlers. `main.go` calls `b.RegisterTools(pkg.BuildTools()...)`.

- **Sense**: `internal/timeline/` — single `Sense` implementation. Registered via `b.SetSense(...)`.
- **Reflexes**: defined in domain packages — `task.Service.CheckStale`, `alarms.Service.FireDueAlarms`, `alarms.Service.StaleAlarmReflex`, `skills.SkillChangeReflex`, `shell.Service.CheckSessions`. Registered in `main.go` via `b.RegisterReflex(...)`.

Wiring steps:

1. Load config, open DB, create WhatsApp client and adapter
2. Register adapter with messaging service, start adapter
3. Build LLM client (OpenAI key or Codex auth)
4. Create domain services: `alarms`, `recall`
5. Create brain: `b := brain.New(cfg, llmClient)` (soul loaded from `soul/latest.md` automatically)
6. Register reflexes: `taskSvc.CheckStale`, `alarmSvc.FireDueAlarms`, `alarmSvc.StaleAlarmReflex()`, `skills.SkillChangeReflex(cfg, conn)`, `shellSvc.CheckSessions`
7. Set sense: `timeline.New(cfg, messagingSvc, taskSvc, alarmSvc, skillsSvc)`
8. Register tools from all domain packages
9. `b.Awake(ctx, pollInterval)` starts the main loop

### Adding a new tool

1. Define `var myToolDef = llm.ToolDef{...}` and `func executeMyTool(ctx, deps, call)` in the domain package
2. Add to the package's `BuildTools()` return list
3. Wire in `main.go`
4. Register in `tools/call/main.go` so the tool is available for CLI testing
5. Update `tools/call/README.md` to reflect the new tool

### LLM tool schemas

OpenAI's API requires `required` to list **every** key in `properties`. Optional parameters must still appear in `required`; use `"description"` to indicate they can be empty/null.

### Query embedding (no sqlc)

All queries live in `internal/queries/*.sql` files with exact executable SQL (positional `?1`/`?2` params). The `queries` Go package (`internal/queries/embed.go`) embeds every `.sql` file as an exported string var. The `db` package imports it as `"github.com/kciuffolo/nik/internal/queries"` and passes the embedded SQL to `database/sql` calls. Schema DDL lives in `internal/db/schema.sql`, embedded directly by the `db` package.

### DB layer

**Driver** (`mattn/go-sqlite3`):

- **UUID handling**: all UUIDs are stored and queried as plain TEXT strings
- **Array columns**: multi-value fields are JSON arrays in TEXT columns. Use `MarshalStringSlice` to bind and `scanStringSlice` to scan (both in `scan.go`)
- **Custom functions**: `jaro_winkler_similarity` is registered via the driver's `ConnectHook` in `db.go`

**Query function design:** one Go function per entity operation. Never create multiple `DoSomethingByX` / `DoSomethingByY` variants that differ only in lookup column. Instead, use a single function with a params struct and dispatch internally based on which fields are populated. Multiple `.sql` files behind a single Go function is fine.

**No SQL query variants for optional columns.** Don't create a new `.sql` file that differs from an existing one only by including an extra nullable column. Extend the existing query to accept the column as a nullable parameter instead — callers pass `nil` when unused.

Good — `GetContact` already does this (`get_contact.sql` uses `WHERE id = ?1 OR EXISTS (SELECT 1 FROM json_each(whatsapp_ids) WHERE value = ?1) OR ...`). `GetMessagesByConversation` dispatches between two SQL files based on `beforeID`.

**Service layering:** `db/` is the only package that touches `internal/queries`. It owns model types (`db/models.go`), scan helpers, and query functions. Domain packages (`internal/<name>/`) hold services, tools, and reflexes — they call `db.*` functions for all persistence.

- model types (plain data structs, no methods) go in `db/models.go`
- query functions are standalone: `func TaskGet(ctx, db, taskID) (Task, error)`
- scan helpers are unexported: `func scanTask(s scanner) (Task, error)`
- any db function with 3+ domain params uses a Params struct: `TaskInsertParams`, `CreateAlarmParams`
- services own business logic: ID generation, LLM calls, time calculations, type transforms

**UUIDs:** all primary keys are **UUIDv7** (time-ordered), generated via `id.V7()` from `internal/id/` (`github.com/google/uuid`). `id.V4()` for random UUIDs, `id.Short(n)` for short hex IDs (e.g. shell session names). Stored as plain `TEXT` in SQLite.

**Short IDs in the timeline:** `id.Shorten(uuid)` extracts the last 12 hex chars (random portion) of a UUID for display. All entity IDs in the timeline (`task_id:`, `alarm_id:`) use short forms to save tokens. Disambiguation: short ID + context (timestamp, goal, entry type) is unique — same principle as message text matching. Tools resolve short IDs by suffix match via `db.ResolveShortID` (`WHERE id LIKE '%' || ?1`).

## SQL

SQLite, single file at `$NIK_HOME/nik.db`. Schema applied on startup via `db.Open()`. Foreign keys are enabled via `_foreign_keys=1` pragma. WAL mode is on for concurrent reads.

**Never use the `sqlite3` CLI to mutate nik.db.** The CLI defaults to `PRAGMA foreign_keys = OFF`, which silently bypasses FK constraints and creates orphaned rows. All writes must go through `db.Open()` (which enforces FKs) or, if the CLI is unavoidable, start every session with `PRAGMA foreign_keys = ON;` before any mutation.

- always use `TEXT`, never `VARCHAR`
- SQL uses two-space indentation
- **one column per line** in SELECT lists -- never pack multiple columns onto a single line
- in every `CREATE TABLE`, keep all `*_at` timestamp columns grouped at the bottom of the column list
- no inline SQL in Go files
- all table names are **singular**: `contact`, `conversation`, `message`, `media`, `alarm`, `task`, etc.
- canonical query files use canonical prefixes: `conversation_*`, `message_*`, `media_*`, `contact_*`, `alarm_*`
- FK columns always include the target table name: `<table>_id` for simple references, `<qualifier>_<table>_id` when disambiguation is needed (e.g. `origin_contact_id`, `retry_for_task_id`)
- enums are `TEXT` columns with a `CHECK(col IN (...))` constraint — never use a separate lookup table

### Row lifecycle columns

Table rows are objects. All objects have `created_at`. Mutable objects also have `updated_at`. Immutable objects (events, occurrences, reports) have only `created_at`.

### SQLite features

- JSON arrays in TEXT columns for multi-value fields (nicknames, emails, whatsapp_ids, phone_numbers)
- `json_each()`, `json_extract()` for array lookups
- `jaro_winkler_similarity()` custom function for fuzzy contact search
- `ON CONFLICT ... DO UPDATE` for upserts

### Migrations

Schema source of truth is `internal/db/schema.sql`. On fresh databases it is applied directly via `CREATE TABLE IF NOT EXISTS`. For existing databases, run `make schema-diff` to compare the live DB against the desired schema. The tool prints column-level diffs (missing columns, type/default mismatches, extra columns). It never modifies the database -- the AI reads the diff output and applies the necessary `ALTER TABLE` statements itself.

Before applying any migration to the live DB:

- **Back up first**: copy the DB file in workspace/backups/<date-time>.db before touching it. Ensure all data is committed, nik might be running.
- **One statement at a time**: execute each `ALTER TABLE` / `CREATE TABLE` / `DROP TABLE` independently so a failure doesn't leave the DB in a half-migrated state.
- **Do not lose data**: migrate the data, and abort if you are not confident.

## Candidates

Things to revisit periodically. The agent adds entries here when the user flags a mistake or suggests a different approach. Only the user removes entries.

<!-- example: - 2026-03-14: user prefers X over Y for error handling -- revisit error style rules -->
- 2026-03-16: user prefers deleting dead or test-only DB/query paths instead of keeping production wrappers and SQL files that no longer serve a runtime contract
- 2026-03-16: user prefers every table to have a stable id column even when another field is unique, because mutable natural keys like skill names should not be primary keys
- 2026-03-16: user prefers prompt-format instructions to live in prompt files when possible, and questions code-level prompt fragments unless they are required by API mechanics rather than model behavior
- 2026-03-16: user prefers API-mechanical prompt/input workarounds to live at the LLM boundary rather than in domain-specific callers like task critic
- 2026-03-16: user prefers deferring skill-event cleanup when it is low-priority, while still taking adjacent runtime cleanups and simplifications
- 2026-03-16: user prefers changes to land one commit at a time, with a review stop before each commit

## Fin

- scarlet
