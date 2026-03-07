<!-- markdownlint-disable -->

# AI Assistant Rules

These rules are binding for work in this repo.

## Philosophy

Nik is an autonomous personal AI -- like OpenClaw but with a personality, real memory, and the goal of becoming a family member. It talks to people directly on WhatsApp (and eventually other platforms).

**Design principles:**

- **Highest autonomy** -- nik should be able to do everything on its own without human intervention
- **Smallest codebase** -- the code should be small enough for one person (or one AI) to fully grok; every line earns its place
- **Core tools + extensible skills** -- a small set of powerful core tools (exec, read, write, search, etc.) and a growing set of user-defined skills that compose them. Skills are the extension mechanism, not more tools.
- **Single decision-maker** -- infrastructure (runners, datasources, adapters) moves data and updates state, but never decides on behalf of the LLM. When code auto-generates messages, reports, or actions, it creates invisible actors that confuse the model. Only the LLM decides what to communicate and when.

### Human-centric async design

Nik interacts with long-running tasks the way a human does:

- **Staring**: synchronous watching with polling. Kick off a command, watch for output, catch when it finishes. Fast commands return immediately.
- **Checking in**: asynchronous reminders. Walk away, set a mental note to come back later, glance at the screen, decide what to do.

The model drives the cadence -- it decides how long to stare, when to come back, whether to report to a user, and whether to keep watching or walk away. The data source is just a calendar reminder.

**Invariant**: every alive session always has a scheduled check-in. The only way to stop is to kill it. No orphans.

## Initialization

- read this entire file before doing anything
- follow all rules strictly
- acknowledge you read it by replying with "I read the AGENTS.md - <the color in **Fin**>" (and only that color) but don't stop and continue your tasks.

## Logging and debugging

- runtime logs are in `workspace/nik.log`
- never run `make run` on your own, ask me to do it if you need me to
- do not override GOPROXY, I need a VPN when it fails, tell me to connect to it and wait
- there is a debug folder with all inputs and output to nik for each activation, you can use it to check what's happening
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
- The `workspace/` folder in the repo is the user-facing workspace. All runtime artifacts (db, logs, media, debug) are written here. When nik is installed, this is the only folder exposed to users. Prompts and skills currently live at the repo root; `prompts_dir` and `skills_dir` in config.yaml point nik at them (relative to Home).
- **Workspace skills** (`Home/skills`, i.e. `workspace/skills/`): nik writes his own skills here at runtime. These are loaded from disk on every brain activation alongside built-in skills. When a workspace skill shares a name with a built-in skill, the workspace version wins. Not git-tracked (`workspace/` is gitignored).
- **Always pass `*config.Config`** — never copy individual fields into local config structs. Every package that needs config holds a `*config.Config` pointer and reads from it directly. Derived paths live as getters on `Config` (e.g. `DBPath()`, `MediaPath()`, `DebugPath()`, `WorkspaceSkillsPath()`).

### Project structure

Entry point: `cmd/nik/main.go`

| Package | Purpose |
|---------|---------|
| `cmd/nik/` | binary entry point — config, DB, WhatsApp client wiring, signal handling |
| `internal/config/` | `Config` struct + `Load(home)` from `config.yaml` in home dir |
| `internal/db/` | SQLite open/schema, models, one Go file per query function |
| `internal/queries/` | embedded `.sql` files for canonical entities (`conversation_*`, `message_*`, `media_*`, etc.) |
| `internal/brain/` | main loop, data source + tool registration, prompt loading, debug output |
| `internal/codex/` | Codex auth for LLM client (login, token management) |
| `internal/dream/` | nightly dream passes for soul evolution, tools, and data source |
| `internal/id/` | UUID generation — `V4()`, `V7()`, `Short(n)` |
| `internal/journal/` | daily journal synthesis service, tools, and data source |
| `internal/llm/` | LLM client — `Complete`, `Embed`, `Transcribe`, `Describe`; supports OpenAI and Codex auth |
| `internal/messaging/` | canonical messaging service, datasource, and tool handlers |
| `internal/whatsapp/` | WhatsApp platform adapter implementing messaging platform interface |
| `internal/contacts/` | contact resolution/upsert orchestration + contact update tools |
| `internal/search/` | search orchestration + read-only query/search tools |
| `internal/shell/` | tmux-backed persistent shell tool + data source |
| `internal/alarms/` | alarm/reminder scheduling service, tools, and data source |
| `internal/memory/` | long-term memory store with vector search (sqlite-vec) |
| `internal/websearch/` | web search tool via Exa API |
| `internal/skills/` | skill loader — reads SKILL.md files and registers tools dynamically |
| `tools/` | codegen/build/debug tools invoked by `make` — no runtime code; each tool has its own README |
| `prompts/` | system prompt templates loaded at runtime |
| `skills/` | built-in skill definitions (SKILL.md files), git-tracked |
| `workspace/` | user-facing workspace — runtime artifacts (db, logs, media, debug, config) |
| `workspace/skills/` | nik-authored skills written at runtime, loaded every activation, not git-tracked |

### Prompt files and what goes where

Each prompt file has one job. Don't duplicate rules across files.

| File | Owns | Does NOT own |
|------|------|------|
| `00-base.md` | template assembly, hard constraints (manager rules), output contract | personality, how to think, how to talk |
| `01-identity.md` | WHO nik is: personality, family, team metaphor, voice/tone, anti-patterns (what nik never does), growth | how to delegate, tool guidance, thinking mechanics |
| `02-conversation.md` | conversation context: session format, media handling, group chat rules | personality, tool usage |
| `03-skills.md` | skill loading: preloaded content, available skill index | personality, delegation |
| `04-brain.md` | HOW nik thinks (9 waves), delegation mechanics (Wave 4), accountability (Wave 8), communication style (Wave 9) | personality traits, identity |
| `05-retry.md` | retry nudge when zero tool calls produced | everything else |
| `task.md` | worker prompt: role, resilience rules, tool docs, skills, plan | personality, messaging, delegation |

**Rule of thumb**: if a rule is about *who nik is*, it goes in `01-identity.md`. If it's about *how nik thinks or acts*, it goes in `04-brain.md`. If it's a hard constraint, `00-base.md`. If it's about *how workers execute*, `task.md`. Never say the same thing in two files.

### Brain activation model

The brain uses cognitive metaphors; the LLM client uses transport/mechanical ones.

```
Brain.Awake()        -- wake up, start the loop
  Brain.perceive()   -- scan senses (data sources) for new stimuli
    Brain.activate() -- one stimulus triggers one activation
      Brain.think()  -- form thoughts (calls llm.Complete under the hood)
        llm.Complete() -- send request, get completion (transport)
```

### Autonomous systems

These run on schedule via data sources — the brain activates them like any other stimulus.

- **Journal**: daily synthesis at `cfg.JournalAt()`. Summarizes the day's conversations, contacts, and memories into a `journal` table entry. Uses `journal.md` prompt template.
- **Dream**: nightly multi-pass process at `cfg.DreamAt()`. Five passes (Drift, Weave, Depths, Crystallize, Wake) that process the journal and memories, writing to the `dream` table. The final pass evolves nik's **soul** — a living identity document stored in the `soul` table and loaded into the system prompt on every activation via `dreamSvc.CurrentSoul`.
- **Briefing**: managed entirely by the `briefing` skill. Nik uses a recurring alarm, `web_search` for news, and writes to `briefings/` files. No domain package.

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

### No sqlc

All queries live in `internal/queries/*.sql` files with exact executable SQL (positional `?1`/`?2` params). The `queries` Go package (`internal/queries/embed.go`) embeds every `.sql` file as an exported string var. The `db` package imports it as `"github.com/kciuffolo/nik/internal/queries"` and passes the embedded SQL to `database/sql` calls. Schema DDL lives in `internal/db/schema.sql`, embedded directly by the `db` package. **No inline SQL in Go files.**

### SQLite Features Used

- JSON arrays in TEXT columns for multi-value fields (nicknames, emails, whatsapp_ids, phone_numbers)
- `json_each()`, `json_extract()` for array lookups
- `jaro_winkler_similarity()` custom function for fuzzy contact search
- `ON CONFLICT ... DO UPDATE` for upserts
- `sqlite-vec` extension for vector similarity search

### UUIDs

All primary keys are **UUIDv7** (time-ordered), generated in Go via `id.V7()` from `internal/id/` (`github.com/google/uuid`). `id.V4()` for random UUIDs, `id.Short(n)` for short hex IDs (e.g. shell session names). Stored as plain `TEXT` in SQLite.

### SQLite Go Driver Conventions

Using `mattn/go-sqlite3` with `asg017/sqlite-vec`:

- **UUID handling**: all UUIDs are stored and queried as plain TEXT strings
- **Array columns**: multi-value fields are JSON arrays in TEXT columns. Use `MarshalStringSlice` to bind and `scanStringSlice` to scan (both in `scan.go`)
- **Custom functions**: `jaro_winkler_similarity` is registered via the driver's `ConnectHook` in `db.go`

### Query function design

One Go function per entity operation. Never create multiple `DoSomethingByX` / `DoSomethingByY` variants that differ only in lookup column. Instead, use a single function with a params struct and dispatch internally based on which fields are populated. Multiple `.sql` files behind a single Go function is fine.

Good — `GetContact` already does this (`get_contact.sql` uses `WHERE id = ?1 OR EXISTS (SELECT 1 FROM json_each(whatsapp_ids) WHERE value = ?1) OR ...`). `GetMessagesByConversation` dispatches between two SQL files based on `beforeID`.

### DB / service layering

`db/` is the only package that touches `internal/queries`. It owns model types (`db/models.go`), scan helpers, and query functions. Domain packages (`internal/<name>/`) hold services, tools, and data sources — they call `db.*` functions for all persistence.

- model types (plain data structs, no methods) go in `db/models.go`
- query functions are standalone: `func TaskGet(ctx, db, taskID) (Task, error)`
- scan helpers are unexported: `func scanTask(s scanner) (Task, error)`
- any db function with 3+ domain params uses a Params struct: `TaskInsertParams`, `CreateAlarmParams`
- services own business logic: ID generation, LLM calls, time calculations, type transforms

### Naming Conventions

- All table names are **singular**: `contact`, `conversation`, `conversation_participant`, `message`, `media`, `message_media`, `alarm`, `alarm_occurrence`, `memory`, `vec_memory`, `journal`, `dream`, `soul`, `briefing`, `briefing_topic`
- Canonical query files use canonical prefixes: `conversation_*`, `message_*`, `media_*`, `message_media_*`, `contact_*`, `alarm_*`, `memory_*`, `journal_*`, `dream_*`, `soul_*`
- Tool names use canonical prefixes by domain (see "Where tools live" table for the full list)
- Metadata keys use canonical ids: `conversation_id`, `message_id` (platform ids are never exposed to LLM context)
- FK columns always include the target table name: `<table>_id` for simple references, `<qualifier>_<table>_id` when disambiguation is needed (e.g. `origin_contact_id`, `retry_for_task_id`). Self-references follow the same pattern.

### Nik's Identity

Nik is an independent entity with its own WhatsApp phone number. `is_from_me` means "sent by nik" (not "sent by nik's owner"). Nik communicates directly on WhatsApp.

## LLM tool schemas

OpenAI's API requires `required` to list **every** key in `properties`. Optional parameters must still appear in `required`; use `"description"` to indicate they can be empty/null.

## Brain tools and data sources

The `brain` package provides registration machinery (`Tool`, `ToolDeps`, `ToolHandler`, `DataSource`) but **never defines tools or data sources itself**. Each domain package defines its own tools and data sources, and `main.go` wires them in.

### Where tools live

Tools are defined in their domain package, not in `brain/`:

| Package | Tools | Why |
|---------|-------|-----|
| `internal/messaging/` | `message_reply`, `message_react`, `message_set_presence`, `message_update_media_description` | canonical messaging actions routed by platform |
| `internal/contacts/` | `update_contact` | contact profile management |
| `internal/search/` | `db_query`, `search_contacts` | read/search tooling |
| `internal/llm/` | `describe_media` | generic AI capability, wraps LLM methods |
| `internal/shell/` | `shell` | persistent tmux terminal (run/read/send/kill/list) |
| `internal/alarms/` | `alarm`, `update_alarm`, `cancel_alarm` | alarm/reminder scheduling |
| `internal/memory/` | `store_memory`, `search_memory`, `delete_memory` | long-term memory with vector search |
| `internal/websearch/` | `web_search` | web search via Exa API |
| `internal/skills/` | `load_skill` | load skill definitions from SKILL.md files |
| `internal/config/` | `update_config` | read and update config values |
| `internal/journal/` | `journal_write` | daily journal synthesis |
| `internal/dream/` | `dream_write`, `soul_evolve` | nightly dream passes and soul evolution |
| `internal/task/` | `task_spawn`, `task_retry`, `task_list`, `task_status`, `task_cancel` | background task orchestration |

Each package exposes a `BuildTools() []llm.Tool` function that returns tool definitions + handlers. `main.go` calls `b.RegisterTools(pkg.BuildTools()...)`.

### Where data sources live

Data sources follow the same pattern. Each domain package that produces context for the brain exposes a `NewDataSource()` function. Currently registered: `messaging` (unread conversations), `alarms` (due alarms), `shell` (active sessions), `journal` (daily journal), `dream` (nightly dream passes).

### Registration flow (`main.go`)

1. Load config, open DB, create WhatsApp client and adapter
2. Register adapter with messaging service, start adapter
3. Build LLM client (OpenAI key or Codex auth)
4. Create domain services: `alarms`, `search`, `memory`, `journal`, `dream`
5. Create brain: `b := brain.New(cfg, llmClient)`, set soul reader via `dreamSvc.CurrentSoul`
6. Register data sources: `messaging`, `alarms`, `shell`, `journal`, `dream`
7. Register tools from all domain packages (see tools table above)
8. `b.Awake(ctx, pollInterval)` starts the main loop

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

## Git Strategy

`.gitignore` uses ignore-all approach: `*` ignores everything, then specific patterns are un-ignored (`!*.go`, `!go.mod`, `!go.sum`, `!*.sql`, `!*.yaml`, `!*.md`, `!Makefile`, `!.gitignore`, `!.config.example.yaml`). `workspace/` is blanket-ignored (contains runtime artifacts and secrets). Use `git add -f` if a new file type needs tracking.

## Style

- Always use `TEXT`, never `VARCHAR`
- SQL uses two-space indentation
- **One column per line** in SELECT lists and one field per line in Go `Scan()` calls -- never pack multiple columns/fields onto a single line
- In every `CREATE TABLE`, keep all `*_at` timestamp columns grouped at the bottom of the column list
- Go follows standard gofmt conventions
- One Go file per query function, one test file per query function

## Fin

- scarlet
