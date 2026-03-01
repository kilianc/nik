<!-- markdownlint-disable -->

# AI Assistant Rules

These rules are binding for work in this repo.

## Philosophy

Nik is an autonomous personal AI -- like OpenClaw but with a personality, real memory, and the goal of becoming a family member. It talks to people directly on WhatsApp (and eventually other platforms).

**Design principles:**

- **Highest autonomy** -- nik should be able to do everything on its own without human intervention
- **Smallest codebase** -- the code should be small enough for one person (or one AI) to fully grok; every line earns its place
- **Core tools + extensible skills** -- a small set of powerful core tools (exec, read, write, search, etc.) and a growing set of user-defined skills that compose them. Skills are the extension mechanism, not more tools.

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

### Go

- errors are present tense, always wrapped like "read file xxx: err" not "error while reading"
- avoid inline error assignment in if statements; assign first, then check
- never chain multiple operations in a single if condition
- use blank lines to separate logical blocks within a function (guard clauses, parse steps, main logic, return)

## Configuration

- Home directory is set via `--home` flag (defaults to current working directory). During development, `make run` passes `--home workspace`.
- `config.yaml` in Home: all app config (API keys, model, owner, poll intervals, etc.). Loaded at startup by `config.Load(home)`.
- The database lives at `nik.db` in Home.
- The `workspace/` folder in the repo is the user-facing workspace. All runtime artifacts (db, logs, media, debug) are written here. When nik is installed, this is the only folder exposed to users. Prompts and skills currently live at the repo root; `prompts_dir` and `skills_dir` in config.yaml point nik at them (relative to Home).
- **Always pass `*config.Config`** — never copy individual fields into local config structs. Every package that needs config holds a `*config.Config` pointer and reads from it directly. Derived paths live as getters on `Config` (e.g. `DBPath()`, `MediaPath()`, `DebugPath()`).

### Project structure

Entry point: `cmd/nik/main.go`

| Package | Purpose |
|---------|---------|
| `cmd/nik/` | binary entry point — config, DB, WhatsApp client wiring, signal handling |
| `internal/config/` | `Config` struct + `Load(home)` from `config.yaml` in home dir |
| `internal/db/` | SQLite open/schema, models, one Go file per query function |
| `internal/queries/` | embedded `.sql` files for canonical entities (`conversation_*`, `message_*`, `media_*`, etc.) |
| `internal/brain/` | main loop, data source + tool registration, prompt loading, debug output |
| `internal/llm/` | OpenAI client wrapper — `Complete`, `Transcribe`, `Describe`; generic brain tools |
| `internal/messaging/` | canonical messaging service, datasource, and tool handlers |
| `internal/whatsapp/` | WhatsApp platform adapter implementing messaging platform interface |
| `internal/contacts/` | contact resolution/upsert orchestration + contact update tools |
| `internal/search/` | search orchestration + read-only query/search tools |
| `internal/shell/` | tmux-backed persistent shell tool + data source |
| `internal/alarms/` | alarm/reminder scheduling service, tools, and data source |
| `internal/memory/` | long-term memory store with vector search (sqlite-vec) |
| `internal/websearch/` | web search tool via Brave Search API |
| `internal/skills/` | skill loader — reads SKILL.md files and registers tools dynamically |
| `migrations/` | legacy folder (not used in scratch-first development workflow) |
| `tools/` | codegen/build/debug tools invoked by `make` — no runtime code; each tool has its own README |
| `prompts/` | system prompt templates loaded at runtime |
| `skills/` | user-defined skill definitions (SKILL.md files) |
| `workspace/` | user-facing workspace — runtime artifacts (db, logs, media, debug, config) |

### Brain activation model

The brain uses cognitive metaphors; the LLM client uses transport/mechanical ones.

```
Brain.Awake()        -- wake up, start the loop
  Brain.perceive()   -- scan senses (data sources) for new stimuli
    Brain.activate() -- one stimulus triggers one activation
      Brain.think()  -- form thoughts (calls llm.Complete under the hood)
        llm.Complete() -- send request, get completion (transport)
```

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
- adapters expose outbound methods with matching names: `Reply`, `React`, `StartTyping`, `StopTyping`, `SetPresence`, `MarkRead`

### CRM core: `contact` table

Platform-agnostic. Stores identifiers from all platforms in JSON array columns (`whatsapp_ids`, etc.). Fields like `nicknames`, `emails`, `phone_numbers` are also JSON arrays stored as TEXT. `one_liner` and `notes` provide free-text context for nik. See `docs/schema.md` for full column listing.

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

All primary keys are **UUIDv4**, generated in Go via `db.NewID()` (`github.com/google/uuid`). Stored as plain `TEXT` in SQLite. No sequences needed, globally unique across tables.

### SQLite Go Driver Conventions

Using `mattn/go-sqlite3` with `asg017/sqlite-vec`:

- **UUID handling**: all UUIDs are stored and queried as plain TEXT strings
- **Array columns**: multi-value fields are JSON arrays in TEXT columns. Use `MarshalStringSlice` to bind and `scanStringSlice` to scan (both in `scan.go`)
- **Custom functions**: `jaro_winkler_similarity` is registered via the driver's `ConnectHook` in `db.go`

### Query function design

One Go function per entity operation. Never create multiple `DoSomethingByX` / `DoSomethingByY` variants that differ only in lookup column. Instead, use a single function with a params struct and dispatch internally based on which fields are populated. Multiple `.sql` files behind a single Go function is fine.

Good — `GetContact` already does this (`get_contact.sql` uses `WHERE id = ?1 OR EXISTS (SELECT 1 FROM json_each(whatsapp_ids) WHERE value = ?1) OR ...`). `GetMessagesByConversation` dispatches between two SQL files based on `beforeID`.

### Naming Conventions

- All table names are **singular**: `contact`, `conversation`, `message`, `media`, `message_media`, `alarm`
- Canonical query files use canonical prefixes: `conversation_*`, `message_*`, `media_*`, `message_media_*`, `contact_*`, `alarm_*`
- Tool names for messaging actions are canonical and prefixed: `message_reply`, `message_react`, `message_start_typing`, `message_stop_typing`, `message_set_presence`, `message_update_media_description`
- Metadata keys use canonical ids: `conversation_id`, `message_id` (platform ids are never exposed to LLM context)

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
| `internal/messaging/` | `message_reply`, `message_react`, `message_start_typing`, etc. | canonical messaging actions routed by platform |
| `internal/contacts/` | `update_contact` | contact profile management |
| `internal/search/` | `db_query`, `search_contacts` | read/search tooling |
| `internal/llm/` | `describe_media` | generic AI capability, wraps LLM methods |
| `internal/shell/` | `shell` | persistent tmux terminal (run/read/send/kill/list) |
| `internal/alarms/` | `create_alarm` | alarm/reminder scheduling |
| `internal/memory/` | `remember`, `recall`, `forget` | long-term memory with vector search |
| `internal/websearch/` | `web_search` | web search via Brave API |
| `internal/skills/` | `load_skill` | load skill definitions from SKILL.md files |
| `internal/config/` | `get_config` | read config values |

Each package exposes a `BuildTools() []llm.Tool` function that returns tool definitions + handlers. `main.go` calls `b.RegisterTools(pkg.BuildTools()...)`.

### Where data sources live

Data sources follow the same pattern. Each domain package that produces context for the brain exposes a `NewDataSource()` function. Currently registered: `messaging` (conversations), `alarms` (due alarms), `shell` (active sessions).

### Registration flow (`main.go`)

1. Create the brain: `b := brain.New(cfg, llmClient)`
2. Create messaging service and register platform adapters (`whatsapp.NewAdapter(...)`)
3. Start adapters with messaging receiver callbacks (`adapter.Start(ctx, messagingSvc)`)
4. Register data sources: `messaging`, `alarms`, `shell`
5. Register tools by domain (`messaging`, `config`, `contacts`, `search`, `alarms`, `shell`, `llm`, `memory`, `websearch`, `skills`)

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

`.gitignore` uses ignore-all approach: `*` ignores everything, then specific patterns are un-ignored (`!*.go`, `!*.sql`, `!*.yaml`, `!*.md`). `workspace/config.yaml` is explicitly re-ignored (contains secrets). Use `git add -f` if a new file type needs tracking.

## Style

- Always use `TEXT`, never `VARCHAR`
- SQL uses two-space indentation
- **One column per line** in SELECT lists and one field per line in Go `Scan()` calls -- never pack multiple columns/fields onto a single line
- In every `CREATE TABLE`, keep all `*_at` timestamp columns grouped at the bottom of the column list
- Go follows standard gofmt conventions
- One Go file per query function, one test file per query function

## Fin

- scarlet
