<!-- markdownlint-disable -->

# AI Assistant Rules

These rules are binding for work in this repo.

## Philosophy

Nik is an autonomous personal AI -- like OpenClaw but with a personality, real memory, and the goal of becoming a family member. It talks to people directly on WhatsApp (and eventually other platforms).

**Design principles:**

- **Highest autonomy** -- nik should be able to do everything on its own without human intervention
- **Smallest codebase** -- the code should be small enough for one person (or one AI) to fully grok; every line earns its place
- **Core tools + extensible skills** -- a small set of powerful core tools (exec, read, write, search, etc.) and a growing set of user-defined skills that compose them. Skills are the extension mechanism, not more tools.
- **Single decision-maker** -- infrastructure (runners, adapters, reflexes) moves data and updates state, but never decides on behalf of the LLM. When code auto-generates messages, reports, or actions, it creates invisible actors that confuse the model. Only the LLM decides what to communicate and when.

### Human-centric async design

Nik interacts with long-running tasks the way a human does:

- **Staring**: synchronous watching with polling. Kick off a command, watch for output, catch when it finishes. Fast commands return immediately.
- **Checking in**: asynchronous reminders. Walk away, set a mental note to come back later, glance at the screen, decide what to do.

The model drives the cadence -- it decides how long to stare, when to come back, whether to report to a user, and whether to keep watching or walk away. The alarm is just a calendar reminder.

**Invariant**: every alive session always has a scheduled check-in. The only way to stop is to kill it. No orphans.

## Initialization

- read this entire file before doing anything
- follow all rules strictly
- acknowledge you read it by replying with "I read the AGENTS.md - <the color in **Fin**>" (and only that color) but don't stop and continue your tasks.

## Logging and debugging

- runtime logs are in `workspace/nik.log`
- never run `make run` on your own, ask me to do it if you need me to
- never send signals to nik's process (kill, SIGQUIT, SIGTERM, etc.) -- if nik needs a restart, ask me
- do not override GOPROXY, I need a VPN when it fails, tell me to connect to it and wait
- activation detail (instructions, user input, tools, reasoning) is stored in the `activation_detail` DB table, queryable via `db_query`
- after completing Go changes, do this in order:
  - run `make lint`
  - run `make test`
  - run `make schema-diff` to check for schema drift against the live DB

## Code style and best practices

- always read the target file before making edits
- never guess the current state of a file
- preserve user changes; never undo or revert them
- never write documentation files (`*.md`, `README`, etc.) unless explicitly requested
- work inside the user's existing structure and patterns

### Nik reprocesses its own messages -- this is correct

Nik's outbound actions (replies, reactions, tool reaction emojis) are stored via `ReceiveMessage` and appear as new events on the next perception cycle. This triggers re-activation. This is **by design** and must never be treated as a bug, a contributing factor to loops, or something to filter out. If a loop exists, the root cause is elsewhere (e.g. the LLM being compelled to re-handle already-handled events). Never propose fixes that suppress nik's own messages from triggering activation.

### Don't over complicate

Default to the smallest possible edit. When the user asks for a small change, do the small change and stop. Only add new flags, abstractions, helpers, or scaffolding if the user explicitly asks for it. If you believe you have to, propose it before doing it.

### Check in before changing code

When debugging or investigating an issue, present findings and a proposed fix **before** writing code. Do not make changes speculatively — wait for confirmation. When forming a hypothesis, verify it with evidence (logs, queries, tests) before presenting it as fact.

### Plans over chat

When working in plan mode, always put details, before/after examples, and rationale into the plan file itself -- not into chat messages. The plan is the artifact the user reviews and approves.

### Comments

- use lowercase except proper nouns, acronyms, and code references
- keep comments minimal and focused on the why
- avoid comments that restate the code
- avoid placeholder comments like `// helper function`
- no godoc-style comments that restate the function/type name (e.g. `// GetUser returns a user by ID`); only comment exported symbols when the comment adds info the name and signature don't already convey

### Go

- errors are present tense, always wrapped like "read file xxx: err" not "error while reading"
- avoid inline error assignment in if statements; assign first, then check
- never chain multiple operations in a single if condition
- use blank lines to separate logical blocks within a function (guard clauses, parse steps, main logic, return)
- `cmd/nik/main.go` is wiring only — no types, no helper functions, no adapters. If you need a bridge between packages, put it in the domain package that owns the logic.

## Configuration

- Home directory is set via `--home` flag (defaults to current working directory). During development, `make run` passes `--home workspace`.
- `config.yaml` in Home: all app config (API keys, model, reasoning effort, directory overrides, conversation ACLs, schedule times, etc.). Loaded at startup by `config.Load(home)`.
- The database lives at `nik.db` in Home.
- The `workspace/` folder in the repo is the user-facing workspace. All runtime artifacts (db, logs, media) are written here. When nik is installed, this is the only folder exposed to users. Prompts and skills currently live at the repo root; `prompts_dir` and `skills_dir` in config.yaml point nik at them (relative to Home).
- **Workspace skills** (`Home/skills`, i.e. `workspace/skills/`): nik writes his own skills here at runtime. These are loaded from disk on every brain activation alongside built-in skills. When a workspace skill shares a name with a built-in skill, the workspace version wins. Not git-tracked (`workspace/` is gitignored).
### Workspace file immutability

Files produced by skills (journals, briefings, diagnostics, dreams, memories, soul, workspace skills) are immutable after creation. Only the scheduled skill execution that owns them may write or update them. Outside that context, workspace artifacts are read-only. If a previous output was wrong, the next scheduled run corrects it -- old files are never patched.

- **Always pass `*config.Config`** — never copy individual fields into local config structs. Every package that needs config holds a `*config.Config` pointer and reads from it directly. Derived paths live as getters on `Config` (e.g. `DBPath()`, `MediaPath()`, `WorkspaceSkillsPath()`).

### Project structure

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
| `workspace/skills/` | nik-authored skills written at runtime, loaded every activation, not git-tracked |

### Prompt files and what goes where

Each prompt file has one job. Don't duplicate rules across files.

| File | Owns | Does NOT own |
|------|------|------|
| `00-base.md` | template assembly, hard constraints (manager rules), output contract | personality, how to think, how to talk |
| `01-identity.md` | WHO nik is: personality, voice/tone, anti-patterns (what nik never does), growth | tool guidance, thinking mechanics |
| `02-conversation.md` | conversation context: session format, media handling, group chat rules | personality, tool usage |
| `03-skills.md` | skill loading: preloaded content, available skill index | personality |
| `04-brain.md` | HOW nik thinks (5 waves): perceive, understand, plan, check, respond. Task planning (Wave 3), accountability (Wave 4), voice (Wave 5) | personality traits, identity, execution guidance |
| `05-retry.md` | retry nudge when zero tool calls produced | everything else |
| `task.md` | worker prompt: role, execution guidance, tool docs, skills, plan | personality, messaging, management |

**Rule of thumb**: if a rule is about *who nik is*, it goes in `01-identity.md`. If it's about *how nik thinks or acts*, it goes in `04-brain.md`. If it's a hard constraint, `00-base.md`. If it's about *how workers execute*, `task.md`. Never say the same thing in two files.

**Workspace skills are runtime knowledge.** Base prompts (`prompts/`) must never reference specific workspace skills by name. Workspace skills teach through their summaries in the available skills index; base prompts stay generic.

### Brain activation model

The brain uses cognitive metaphors; the LLM client uses transport/mechanical ones.

```
Brain.Awake()        -- wake up, start the loop
  reflexes           -- unconscious side effects (check stale tasks, fire due alarms)
  Brain.perceive()   -- scan sense for new stimuli
    Brain.activate() -- one stimulus triggers one activation
      Brain.think()  -- form thoughts (calls llm.Complete under the hood)
        llm.Complete() -- send request, get completion (transport)
```

- **Reflex** (`func(ctx context.Context)`): unconscious, automatic, side-effect-producing function. Runs every tick *before* perception. Examples: `task.CheckStale` (inserts stale reports), `alarms.FireDueAlarms` (creates occurrences and claims alarms), `alarms.CoreAlarmEnforcer` (ensures core alarms exist and are healthy, throttled to 30 min).
- **Sense** (`interface { Scan(ctx) ([]Stimulus, error) }`): the brain's single, unified perception. Strictly read-only — no side effects. Returns `[]Stimulus`, one per conversation with new events.
- **Stimulus**: structured perception output (`Preamble`, `Timeline []TimelineEntry`, `ReadLine`, `Meta`, `LiveInput`, `Processed`). The timeline is a chronological mix of messages, task reports, and alarm occurrences.

### Autonomous systems

These run on schedule via alarms — the brain activates them like any other stimulus. Core alarms use `[NIK_XXX]` goal prefixes (e.g. `[NIK_JOURNAL]`, `[NIK_DREAM_1]`) and are enforced by the `CoreAlarmEnforcer` reflex in `internal/alarms/core.go`. The reflex creates missing alarms and heals dead ones (null/past `next_fire_at`) using schedule times from config. Skills still document the alarm format as a fallback.

- **Journal**: managed entirely by the `journal` skill. Nik uses a recurring alarm, gathers day context via `db_query`/`shell`, and writes to `journal/` files. No domain package.
- **Dream**: managed entirely by the `dream` skill. Nik uses 5 recurring alarms (one per dream pass), processes the journal and memories, and writes to `dreams/` files. The final pass (Wake) evolves nik's **soul** — a living identity document stored in `soul/latest.md` and loaded into the system prompt on every activation. Dated snapshots in `soul/YYYY-MM-DD.md` preserve history. No domain package.
- **Briefing**: managed entirely by the `briefing` skill. Nik uses a recurring alarm, `web_search` for news, and writes to `briefings/` files. No domain package.
- **Diagnostic**: managed entirely by the `diagnostic` skill. Nik uses a recurring alarm, discovers skills/services, tests auth, verifies alarm chains and skill outputs, checks data integrity and spending. Writes to `diagnostics/` files. No domain package.

### Tasks and the timeline

**Principle:** when things happen, they appear in the timeline. If making an event appear is hard, the data model is wrong.

**Notification model:** the timeline is a notification feed. Task and alarm entries use structured key: value format with 11-space padding on continuation lines (width of `[HH:MM:SS] `). Report content is truncated to 200 chars with `[truncated]` marker. `task_status` provides the full picture: plan, complete report content, tool calls, retry chain.

**Two actors:**

- Workers produce `task_report` rows with a `status` field (`running`, `completed`, `failed`). The runner reads the last report's status to set `task.status`.
- The system produces lifecycle entries from the `task` table (spawned, cancelled, retried).

| Event        | Who produces it                    | Timeline entry                        | Separate system entry?               |
| ------------ | ---------------------------------- | ------------------------------------- | ------------------------------------ |
| Task created | nik calls `task_spawn`             | `[Task spawned]`                      | Yes — introduces the task_id         |
| Progress     | worker writes report               | `[Task report] ... status: running`   | No                                   |
| Completed    | worker writes final report         | `[Task report] ... status: completed` | No — the report IS the event         |
| Failed       | worker writes final report         | `[Task report] ... status: failed`    | No — the report IS the event         |
| Cancelled    | nik calls `task_cancel`            | `[Task cancelled]`                    | Yes — no report covers this          |
| Retried      | nik calls `task_retry`             | `[Task retry #N spawned]`             | Yes — introduces the new task_id     |
| Stale        | `CheckStale` reflex inserts report | `[Task report] ... stale`             | No — stale detection writes a report |

`task_status` is for drill-down, not discovery.

### Scripts and Tools

- avoid bash scripts; create small Go commands in `tools/`
- use `exec.Command()` for external tools
- every tool in `tools/` must have a corresponding `make` target in the Makefile

## Architecture: Canonical Messaging + Adapters

**Core principle:** canonical tables are the source of truth (`conversation`, `message`, `media`, `message_media`). Platform packages are transport adapters that normalize inbound events and execute outbound actions.

### Canonical entities

- `conversation`: nik-owned conversation identity + platform/external chat reference
- `message`: nik-owned message identity + normalized content + platform external refs
- `media`: hash-keyed media cache (`sha256`) for reusable description/transcription
- `message_media`: link table (`UNIQUE(message_id)`) for one media per message for now

### Adapter contract

- each platform implements `MessagingPlatform` in `internal/messaging` contracts
- adapters emit canonical `Conversation`/`Message` via `ReceiveConversation` + `ReceiveMessage`
- adapters expose outbound methods with matching names: `Reply`, `SendImage`, `React`, `StartTyping`, `StopTyping`, `SetPresence`, `MarkRead`

### CRM core: `contact` table

Platform-agnostic. Stores identifiers from all platforms in JSON array columns (`whatsapp_ids`, `telegram_ids`, `slack_ids`, etc.). Fields like `nicknames`, `emails`, `phone_numbers` are also JSON arrays stored as TEXT. `timezone`, `location`, `one_liner`, and `notes` provide free-text context for nik. See `internal/db/schema.sql` for full column listing.

## Database

SQLite, single file at `$NIK_HOME/nik.db`. Schema applied on startup via `db.Open()`. Foreign keys are enabled via `_foreign_keys=1` pragma. WAL mode is on for concurrent reads.

**Never use the `sqlite3` CLI to mutate nik.db.** The CLI defaults to `PRAGMA foreign_keys = OFF`, which silently bypasses FK constraints and creates orphaned rows. All writes must go through `db.Open()` (which enforces FKs) or, if the CLI is unavoidable, start every session with `PRAGMA foreign_keys = ON;` before any mutation.

### No sqlc

All queries live in `internal/queries/*.sql` files with exact executable SQL (positional `?1`/`?2` params). The `queries` Go package (`internal/queries/embed.go`) embeds every `.sql` file as an exported string var. The `db` package imports it as `"github.com/kciuffolo/nik/internal/queries"` and passes the embedded SQL to `database/sql` calls. Schema DDL lives in `internal/db/schema.sql`, embedded directly by the `db` package. **No inline SQL in Go files.**

### SQLite Features Used

- JSON arrays in TEXT columns for multi-value fields (nicknames, emails, whatsapp_ids, phone_numbers)
- `json_each()`, `json_extract()` for array lookups
- `jaro_winkler_similarity()` custom function for fuzzy contact search
- `ON CONFLICT ... DO UPDATE` for upserts


### UUIDs

All primary keys are **UUIDv7** (time-ordered), generated in Go via `id.V7()` from `internal/id/` (`github.com/google/uuid`). `id.V4()` for random UUIDs, `id.Short(n)` for short hex IDs (e.g. shell session names). Stored as plain `TEXT` in SQLite.

**Short IDs in the timeline:** `id.Shorten(uuid)` extracts the last 12 hex chars (random portion) of a UUID for display. All entity IDs in the timeline (`task_id:`, `alarm_id:`) use short forms to save tokens. Disambiguation: short ID + context (timestamp, goal, entry type) is unique — same principle as message text matching. Tools resolve short IDs by suffix match via `db.ResolveShortID` (`WHERE id LIKE '%' || ?1`).

### SQLite Go Driver Conventions

Using `mattn/go-sqlite3`:

- **UUID handling**: all UUIDs are stored and queried as plain TEXT strings
- **Array columns**: multi-value fields are JSON arrays in TEXT columns. Use `MarshalStringSlice` to bind and `scanStringSlice` to scan (both in `scan.go`)
- **Custom functions**: `jaro_winkler_similarity` is registered via the driver's `ConnectHook` in `db.go`

### Query function design

One Go function per entity operation. Never create multiple `DoSomethingByX` / `DoSomethingByY` variants that differ only in lookup column. Instead, use a single function with a params struct and dispatch internally based on which fields are populated. Multiple `.sql` files behind a single Go function is fine.

Good — `GetContact` already does this (`get_contact.sql` uses `WHERE id = ?1 OR EXISTS (SELECT 1 FROM json_each(whatsapp_ids) WHERE value = ?1) OR ...`). `GetMessagesByConversation` dispatches between two SQL files based on `beforeID`.

### DB / service layering

`db/` is the only package that touches `internal/queries`. It owns model types (`db/models.go`), scan helpers, and query functions. Domain packages (`internal/<name>/`) hold services, tools, and reflexes — they call `db.*` functions for all persistence.

- model types (plain data structs, no methods) go in `db/models.go`
- query functions are standalone: `func TaskGet(ctx, db, taskID) (Task, error)`
- scan helpers are unexported: `func scanTask(s scanner) (Task, error)`
- any db function with 3+ domain params uses a Params struct: `TaskInsertParams`, `CreateAlarmParams`
- services own business logic: ID generation, LLM calls, time calculations, type transforms

### Naming Conventions

- All table names are **singular**: `contact`, `conversation`, `conversation_participant`, `message`, `media`, `message_media`, `alarm`, `alarm_occurrence`, `dream`, `soul`, `briefing`, `briefing_topic`
- Canonical query files use canonical prefixes: `conversation_*`, `message_*`, `media_*`, `message_media_*`, `contact_*`, `alarm_*`
- Tool names use canonical prefixes by domain (see "Where tools live" table for the full list)
- Metadata keys use canonical ids: `conversation_id`, `message_id` (platform ids are never exposed to LLM context)
- FK columns always include the target table name: `<table>_id` for simple references, `<qualifier>_<table>_id` when disambiguation is needed (e.g. `origin_contact_id`, `retry_for_task_id`). Self-references follow the same pattern.
- Service method names: `Get` for single entity by ID, `List<Plural>` for returning slices (e.g. `ListTasks`, `ListReports`, `ListOccurrences`). Avoid bare `List()` — include the entity name.
- DB function names follow the same pattern with entity prefix: `TaskGet`, `TaskList`, `TaskReportList`.

### Nik's Identity

Nik is an independent entity with its own WhatsApp phone number. `is_from_me` means "sent by nik" (not "sent by nik's owner"). Nik communicates directly on WhatsApp.

## LLM tool schemas

OpenAI's API requires `required` to list **every** key in `properties`. Optional parameters must still appear in `required`; use `"description"` to indicate they can be empty/null.

## Brain tools, sense, and reflexes

The `brain` package provides registration machinery (`Tool`, `ToolDeps`, `ToolHandler`, `Sense`, `Reflex`) but **never defines tools, sense, or reflexes itself**. Each domain package defines its own pieces, and `main.go` wires them in.

### Where tools live

Tools are defined in their domain package, not in `brain/`:

| Package | Tools | Why |
|---------|-------|-----|
| `internal/messaging/` | `message_reply`, `message_noop`, `message_react`, `message_set_presence`, `message_update_media_description` | canonical messaging actions routed by platform |
| `internal/contacts/` | `update_contact` | contact profile management |
| `internal/db/` | `db_query` | read-only SQL queries against nik's SQLite database |
| `internal/llm/` | `describe_media` | generic AI capability, wraps LLM methods |
| `internal/shell/` | `shell` | persistent tmux terminal (run/read/send/kill/list) |
| `internal/alarms/` | `alarm`, `update_alarm`, `cancel_alarm` | alarm/reminder scheduling |
| `internal/skills/` | `load_skill` | load skill definitions from SKILL.md files |
| `internal/config/` | `config` | read and update config values |
| `internal/task/` | `task_spawn`, `task_retry`, `task_list`, `task_status`, `task_cancel` | background task orchestration |

Each package exposes a `BuildTools() []llm.Tool` function that returns tool definitions + handlers. `main.go` calls `b.RegisterTools(pkg.BuildTools()...)`.

### Where sense and reflexes live

- **Sense**: `internal/timeline/` — single `Sense` implementation that iterates `AllowConversationIDs`, fetches messages/reports/occurrences, and maps them to `Stimulus`. Centrally owns all timeline formatting.
- **Reflexes**: defined in domain packages — `task.Service.CheckStale`, `alarms.Service.FireDueAlarms`, `alarms.Service.CoreAlarmEnforcer`. Registered in `main.go` via `b.RegisterReflex(...)`.

### Registration flow (`main.go`)

1. Load config, open DB, create WhatsApp client and adapter
2. Register adapter with messaging service, start adapter
3. Build LLM client (OpenAI key or Codex auth)
4. Create domain services: `alarms`, `recall`
5. Create brain: `b := brain.New(cfg, llmClient)` (soul loaded from `soul/latest.md` automatically)
6. Register reflexes: `taskSvc.CheckStale`, `alarmSvc.FireDueAlarms`, `alarmSvc.CoreAlarmEnforcer(cfg)`
7. Set sense: `timeline.NewSense(cfg, messagingSvc, taskSvc, alarmSvc)`
8. Register tools from all domain packages (see tools table above)
9. `b.Awake(ctx, pollInterval)` starts the main loop

### Adding a new tool

1. Define `var myToolDef = llm.ToolDef{...}` and `func executeMyTool(ctx, deps, call)` in the domain package
2. Add to the package's `BuildTools()` return list
3. Wire in `main.go`
4. Register in `tools/call/main.go` so the tool is available for CLI testing
5. Update `tools/call/README.md` to reflect the new tool

## Migrations

Schema source of truth is `internal/db/schema.sql`. On fresh databases it is applied directly via `CREATE TABLE IF NOT EXISTS`. For existing databases, run `make schema-diff` to compare the live DB against the desired schema. The tool prints column-level diffs (missing columns, type/default mismatches, extra columns). It never modifies the database -- the AI reads the diff output and applies the necessary `ALTER TABLE` statements itself.

Before applying any migration to the live DB:

- **Back up first**: copy the DB file in workspace/backups/<date-time>.db before touching it. Ensure all data is committed, nik might be running.
- **One statement at a time**: execute each `ALTER TABLE` / `CREATE TABLE` / `DROP TABLE` independently so a failure doesn't leave the DB in a half-migrated state.
- **Do not lose data**: migrate the data, and abort if you are not confident.

## Testing

- Tests run against in-memory SQLite (`:memory:`) where applicable.
- `make test` or regular `go test`
- Most `.go` should have a `_test.go` counterpart, no dangling test files, if the file gets too big it's a signal the base `.go` file might have to be split.

## Debugging

### Entity graph

```
contact ──┬── conversation_participant ──┬── conversation
           │                              │
           ├── message ───────────────────┘
           │     └── message_media ── media
           │
           ├── task ──┬── task_report
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

### Log file

Location: `workspace/nik.log` (slog text format). Key events to grep for:

- `activation starting` / `activation completed` / `activation failed` -- brain lifecycle
- `tool call` -- includes tool name, round, args (llm package)
- `no terminal tool call, retrying` -- brain loop stall
- `activation_id` appears in both DB rows and log lines -- use it to correlate

### Debug workflow

1. **Anchor** -- find the message or event that triggered the bug (conversation_id + time window, or body text search)
2. **Expand** -- join to conversation, contact, participants to understand who/where
3. **Trace activation** -- find activation(s) by conversation_id + created_at window
4. **Inspect reasoning** -- activation_detail for full prompt context and reasoning summaries
5. **Audit tool calls** -- tool_call rows for the activation, check errors, inspect input/output
6. **Follow tasks** -- task -> task_report -> worker activation (task.activation_id) -> worker tool_calls
7. **Check logs** -- grep nik.log for the activation_id to see runtime errors, timing, retries
8. **Alarm chain** -- if alarm-related, check alarm -> alarm_occurrence -> next_fire_at progression

## Git Strategy

`.gitignore` uses ignore-all approach: `*` ignores everything, then specific patterns are un-ignored (`!*.go`, `!go.mod`, `!go.sum`, `!*.sql`, `!*.yaml`, `!*.md`, `!Makefile`, `!.gitignore`, `!.config.example.yaml`). `workspace/` is blanket-ignored (contains runtime artifacts and secrets). Use `git add -f` if a new file type needs tracking.

**No PII in the repo.** Never commit personally identifiable information — real names, emails, phone numbers, hostnames, addresses, or any other data that identifies a specific person or device. Use placeholders or derive values at runtime (e.g. `$(hostname)`).

**No tool-generated trailers.** Never add metadata lines like `Made-with: Cursor`, `Co-authored-by: AI`, or similar trailers to commit messages. Commit messages contain only the subject and body written by the author.

## Style

- Always use `TEXT`, never `VARCHAR`
- SQL uses two-space indentation
- **One column per line** in SELECT lists and one field per line in Go `Scan()` calls -- never pack multiple columns/fields onto a single line
- In every `CREATE TABLE`, keep all `*_at` timestamp columns grouped at the bottom of the column list
- Go follows standard gofmt conventions
- YAML uses two-space indentation
- Bash/shell scripts use two-space indentation
- One Go file per query function, one test file per query function

## Fin

- scarlet
