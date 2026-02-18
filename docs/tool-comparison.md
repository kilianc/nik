# Tool Comparison: Codex CLI vs OpenCode vs OpenClaw vs Nik

## Complete Tool Inventory (Unfiltered)

Every tool registered in each product, grouped by category. Sources:

- **Codex CLI**: [handlers/](https://github.com/openai/codex/tree/main/codex-rs/core/src/tools/handlers) + [spec.rs](https://github.com/openai/codex/blob/main/codex-rs/core/src/tools/spec.rs)
- **OpenCode**: [tool/](https://github.com/anomalyco/opencode/tree/dev/packages/opencode/src/tool)
- **OpenClaw**: [docs/tools](https://docs.openclaw.ai/tools)

### File System

| Capability                 | Codex CLI     | OpenCode      | OpenClaw                 | Nik |
| -------------------------- | ------------- | ------------- | ------------------------ | --- |
| Read file                  | `read_file`   | `read`        | `read`                   | --  |
| Write file                 | --            | `write`       | `write`                  | --  |
| Edit file (string replace) | --            | `edit`        | `edit`                   | --  |
| Multi-file edit            | --            | `multiedit`   | --                       | --  |
| Apply patch/diff           | `apply_patch` | `apply_patch` | `apply_patch` (optional) | --  |
| List directory             | `list_dir`    | `ls`          | `ls`                     | --  |
| Find files by pattern      | --            | `glob`        | `find`                   | --  |
| Search file contents       | `grep_files`  | `grep`        | `grep`                   | --  |
| Semantic code search       | --            | `codesearch`  | --                       | --  |

### Shell / Execution

| Capability                   | Codex CLI                   | OpenCode | OpenClaw                                             | Nik          |
| ---------------------------- | --------------------------- | -------- | ---------------------------------------------------- | ------------ |
| Shell command (basic)        | `shell`                     | `bash`   | `bash`                                               | -- (planned) |
| Shell command (user's shell) | `shell_command`             | --       | --                                                   | --           |
| PTY/interactive exec         | `exec_command`              | --       | `exec` (pty flag)                                    | -- (planned) |
| Write to stdin               | `write_stdin`               | --       | `process` write action                               | -- (planned) |
| Background process mgmt      | --                          | --       | `process` (poll/log/write/kill/list/clear/send-keys) | -- (planned) |
| Container exec               | `container.exec`            | --       | --                                                   | --           |
| Local shell                  | `local_shell`               | --       | --                                                   | --           |
| JavaScript REPL              | `js_repl` + `js_repl_reset` | --       | --                                                   | --           |

### Web

| Capability        | Codex CLI                   | OpenCode             | OpenClaw                               | Nik |
| ----------------- | --------------------------- | -------------------- | -------------------------------------- | --- |
| Web search        | `web_search` (API built-in) | `websearch` (Exa AI) | `web_search` (Brave/Perplexity/Gemini) | --  |
| Fetch URL content | --                          | `webfetch`           | `web_fetch`                            | --  |

### Memory / Persistent State

| Capability              | Codex CLI                       | OpenCode                 | OpenClaw                         | Nik |
| ----------------------- | ------------------------------- | ------------------------ | -------------------------------- | --- |
| Todo / task tracking    | `update_plan` (doubles as todo) | `todoread` / `todowrite` | --                               | --  |
| Long-term memory read   | --                              | --                       | `memory_get`                     | --  |
| Long-term memory search | --                              | --                       | `memory_search` (vector-indexed) | --  |

### Scheduling / Automation

| Capability        | Codex CLI | OpenCode | OpenClaw                                     | Nik     |
| ----------------- | --------- | -------- | -------------------------------------------- | ------- |
| One-shot alarm    | --        | --       | `cron` (at: one-shot ISO timestamp)          | `alarm` |
| Recurring cron    | --        | --       | `cron` (cron: 5-field expr, every: interval) | --      |
| Gateway / restart | --        | --       | `gateway` (restart/config.get/config.patch)  | --      |

### Multi-Agent / Sessions

| Capability           | Codex CLI                                         | OpenCode | OpenClaw                        | Nik |
| -------------------- | ------------------------------------------------- | -------- | ------------------------------- | --- |
| Spawn sub-agent      | `spawn_agent`                                     | `task`   | `sessions_spawn`                | --  |
| Send input to agent  | `send_input`                                      | --       | `sessions_send`                 | --  |
| Resume agent         | `resume_agent`                                    | --       | --                              | --  |
| Wait for agent       | `wait`                                            | --       | --                              | --  |
| Close agent          | `close_agent`                                     | --       | --                              | --  |
| List sessions/agents | --                                                | --       | `sessions_list` / `agents_list` | --  |
| Session history      | --                                                | --       | `sessions_history`              | --  |
| Session status       | --                                                | --       | `session_status`                | --  |
| Batch jobs from CSV  | `spawn_agents_on_csv` + `report_agent_job_result` | `batch`  | --                              | --  |

### User Interaction / Planning

| Capability                   | Codex CLI                        | OpenCode                   | OpenClaw             | Nik               |
| ---------------------------- | -------------------------------- | -------------------------- | -------------------- | ----------------- |
| Ask user structured question | `request_user_input`             | `question`                 | -- (via messaging)   | -- (via WhatsApp) |
| Plan mode toggle             | -- (separate collaboration mode) | `plan-enter` / `plan-exit` | --                   | --                |
| Update plan/todo             | `update_plan`                    | `todowrite`                | --                   | --                |
| Skills management            | --                               | `skill`                    | -- (via skills dirs) | --                |

### Messaging

| Capability           | Codex CLI | OpenCode | OpenClaw                                         | Nik                                                              |
| -------------------- | --------- | -------- | ------------------------------------------------ | ---------------------------------------------------------------- |
| Send messages        | --        | --       | `message` send (multi-platform)                  | `wapp_reply`                                                     |
| Reactions            | --        | --       | `message` react                                  | `wapp_react`                                                     |
| Presence / typing    | --        | --       | --                                               | `wapp_set_presence` / `wapp_send_composing` / `wapp_send_paused` |
| Search messages      | --        | --       | `message` search                                 | -- (in TODO.md)                                                  |
| Edit/delete messages | --        | --       | `message` edit/delete                            | --                                                               |
| Threads              | --        | --       | `message` thread-create/thread-reply/thread-list | --                                                               |
| Pins                 | --        | --       | `message` pin/unpin/list-pins                    | --                                                               |
| Polls                | --        | --       | `message` poll                                   | --                                                               |
| Roles/permissions    | --        | --       | `message` role-add/role-remove/permissions       | --                                                               |
| Emoji/sticker upload | --        | --       | `message` emoji-upload/sticker-upload            | --                                                               |
| Channel info         | --        | --       | `message` channel-info/channel-list              | --                                                               |
| Events               | --        | --       | `message` event-list/event-create                | --                                                               |
| Moderation           | --        | --       | `message` timeout/kick/ban                       | --                                                               |

### Media / Vision

| Capability              | Codex CLI    | OpenCode | OpenClaw              | Nik                             |
| ----------------------- | ------------ | -------- | --------------------- | ------------------------------- |
| View/analyze image      | `view_image` | --       | `image`               | `describe_media`                |
| Audio transcription     | --           | --       | --                    | `describe_media`                |
| Media description store | --           | --       | --                    | `wapp_update_media_description` |
| Camera snap             | --           | --       | `nodes` camera_snap   | --                              |
| Screen record           | --           | --       | `nodes` screen_record | --                              |

### Browser / UI

| Capability         | Codex CLI | OpenCode | OpenClaw                                                     | Nik |
| ------------------ | --------- | -------- | ------------------------------------------------------------ | --- |
| Browser automation | --        | --       | `browser` (navigate/screenshot/snapshot/act/tabs/start/stop) | --  |
| Canvas / A2UI      | --        | --       | `canvas` (present/eval/snapshot/a2ui_push)                   | --  |

### Code Intelligence

| Capability                         | Codex CLI          | OpenCode             | OpenClaw | Nik |
| ---------------------------------- | ------------------ | -------------------- | -------- | --- |
| LSP (go-to-def, references, hover) | --                 | `lsp` (experimental) | --       | --  |
| BM25 app/connector search          | `search_tool_bm25` | --                   | --       | --  |

### Nodes / Infrastructure (OpenClaw-only)

| Capability                  | Codex CLI | OpenCode | OpenClaw                       | Nik |
| --------------------------- | --------- | -------- | ------------------------------ | --- |
| Node discovery/status       | --        | --       | `nodes` status/describe        | --  |
| Run command on node         | --        | --       | `nodes` run                    | --  |
| Push notification to device | --        | --       | `nodes` notify                 | --  |
| Device location             | --        | --       | `nodes` location_get           | --  |
| Node pairing                | --        | --       | `nodes` pending/approve/reject | --  |

### MCP / Extensibility

| Capability                  | Codex CLI                     | OpenCode             | OpenClaw | Nik |
| --------------------------- | ----------------------------- | -------------------- | -------- | --- |
| List MCP resources          | `list_mcp_resources`          | --                   | --       | --  |
| List MCP resource templates | `list_mcp_resource_templates` | --                   | --       | --  |
| Read MCP resource           | `read_mcp_resource`           | --                   | --       | --  |
| Dynamic/external tools      | `dynamic` handler             | `external-directory` | plugins  | --  |

### Data / Config (nik-specific)

| Capability         | Codex CLI | OpenCode | OpenClaw         | Nik                          |
| ------------------ | --------- | -------- | ---------------- | ---------------------------- |
| Database queries   | --        | --       | --               | `db_query` (privileged)      |
| Contact management | --        | --       | --               | `update_contact`             |
| Config management  | --        | --       | `gateway` config | `update_config` (privileged) |

---

## Tool Count Summary

| Product       | Total unique tools                                                        |
| ------------- | ------------------------------------------------------------------------- |
| **Codex CLI** | 27 registered handlers (some are aliases/variants of the same capability) |
| **OpenCode**  | 19 tools (each .ts file = one tool)                                       |
| **OpenClaw**  | 24+ tools (across 10 tool groups, `message` alone has 20+ actions)        |
| **Nik**       | 10 tools                                                                  |

---

## Deep Dive: Tool Descriptions from Each Product

For each of the six tools we're considering, here's the actual prompt/description each product uses.

---

### 1. Ask a Question

**Codex CLI** -- `request_user_input` ([source](https://github.com/openai/codex/blob/main/codex-rs/core/src/tools/handlers/request_user_input.rs))

> "Request user input for one to three short questions and wait for the response. This tool is only available in Plan mode."

Parameters: `questions` (array of structured question objects, each with `options` -- non-empty options required, plus auto-added `is_other` freeform option).

**OpenCode** -- `question` ([source](https://github.com/anomalyco/opencode/blob/dev/packages/opencode/src/tool/question.txt))

> "Use this tool when you need to ask the user questions during execution. This allows you to: 1. Gather user preferences or requirements 2. Clarify ambiguous instructions 3. Get decisions on implementation choices as you work 4. Offer choices to the user about what direction to take."
>
> Usage notes: "When `custom` is enabled (default), a 'Type your own answer' option is added automatically; don't include 'Other' or catch-all options. Answers are returned as arrays of labels; set `multiple: true` to allow selecting more than one. If you recommend a specific option, make that the first option in the list and add '(Recommended)' at the end of the label."

Parameters: `question` (string), `options` (array of labels), `multiple` (bool), `custom` (bool).

**OpenClaw** -- No dedicated tool. User interaction happens through the messaging platform (WhatsApp, Discord, etc.) and exec approval prompts (`ask` parameter on `exec`).

---

### 2. Plan Mode

**Codex CLI** -- `update_plan` ([source](https://github.com/openai/codex/blob/main/codex-rs/core/src/tools/handlers/plan.rs))

> "Updates the task plan. Provide an optional explanation and a list of plan items, each with a step and status. At most one step can be in_progress at a time."

Parameters: `explanation` (string, optional), `plan` (array of `{step: string, status: "pending"|"in_progress"|"completed"}`).

Note: This is a TODO/checklist tool. It's *not* allowed in Plan mode (which is a separate collaboration mode). The model uses it to record a structured plan that clients render. The code comment says: "This function doesn't do anything useful. However, it gives the model a structured way to record its plan that clients can read and render."

**OpenCode** -- `plan-enter` / `plan-exit` ([enter](https://github.com/anomalyco/opencode/blob/dev/packages/opencode/src/tool/plan-enter.txt), [exit](https://github.com/anomalyco/opencode/blob/dev/packages/opencode/src/tool/plan-exit.txt))

Enter:

> "Use this tool to suggest switching to plan agent when the user's request would benefit from planning before implementation. If they explicitly mention wanting to create a plan ALWAYS call this tool first. Call this tool when: the user's request is complex and would benefit from planning first; you want to research and design before making changes; the task involves multiple files or significant architectural decisions."

Exit:

> "Use this tool when you have completed the planning phase and are ready to exit plan agent. This tool will ask the user if they want to switch to build agent to start implementing the plan."

Parameters: none. These are mode-switching signals, not structured data tools.

**OpenClaw** -- No dedicated plan tool. Planning is handled via skills and session prompts.

---

### 3. Web Search

**Codex CLI** -- `web_search` ([source](https://github.com/openai/codex/blob/main/codex-rs/core/src/tools/spec.rs#L1793-L1804))

This is an OpenAI Responses API built-in tool (`ToolSpec::WebSearch`), not a function-call tool with parameters. The model invokes it natively. Configuration: `WebSearchMode::Live` (real-time) or `WebSearchMode::Cached` (cached results). No user-facing parameters -- the model just decides to search.

**OpenCode** -- `websearch` ([source](https://github.com/anomalyco/opencode/blob/dev/packages/opencode/src/tool/websearch.txt))

> "Search the web using Exa AI -- performs real-time web searches and can scrape content from specific URLs. Provides up-to-date information for current events and recent data. Supports configurable result counts and returns the content from the most relevant websites. Use this tool for accessing information beyond knowledge cutoff. Searches are performed automatically within a single API call."
>
> Usage notes: "Supports live crawling modes: 'fallback' (backup if cached unavailable) or 'preferred' (prioritize live crawling). Search types: 'auto' (balanced), 'fast' (quick results), 'deep' (comprehensive search). Configurable context length for optimal LLM integration. Domain filtering and advanced search options available."

Parameters: `query` (required), `count`, `crawl_mode`, `search_type`, `context_length`, domain filters.

**OpenClaw** -- `web_search` ([source](https://docs.openclaw.ai/tools/web))

> "Search the web using Brave Search API."

Parameters: `query` (required), `count` (1-10, default from config), `country` (2-letter code), `search_lang` (ISO), `ui_lang` (ISO), `freshness` (pd/pw/pm/py or date range).

Notes: Supports Brave (default), Perplexity Sonar, or Gemini with Google Search grounding. Results cached 15 min. Returns structured results (Brave) or AI-synthesized answers with citations (Perplexity/Gemini).

---

### 4. Web Fetch

**Codex CLI** -- No dedicated web_fetch tool. Web access is via the built-in `web_search` or MCP resources.

**OpenCode** -- `webfetch` ([source](https://github.com/anomalyco/opencode/blob/dev/packages/opencode/src/tool/webfetch.txt))

> "Fetches content from a specified URL. Takes a URL and optional format as input. Fetches the URL content, converts to requested format (markdown by default). Returns the content in the specified format. Use this tool when you need to retrieve and analyze web content."
>
> Usage notes: "IMPORTANT: if another tool is present that offers better web fetching capabilities, is more targeted to the task, or has fewer restrictions, prefer using that tool instead. The URL must be a fully-formed valid URL. HTTP URLs will be automatically upgraded to HTTPS. Format options: 'markdown' (default), 'text', or 'html'. This tool is read-only and does not modify any files. Results may be summarized if the content is very large."

Parameters: `url` (required), `format` ("markdown" | "text" | "html").

**OpenClaw** -- `web_fetch` ([source](https://docs.openclaw.ai/tools/web))

> "Fetch and extract readable content from a URL (HTML -> markdown/text)."

Parameters: `url` (required, http/https only), `extractMode` ("markdown" | "text"), `maxChars` (truncate long pages, clamped by config cap).

Notes: Plain HTTP GET + Readability extraction. Firecrawl anti-bot fallback optional. Cached 15 min. Blocks private/internal hostnames. SSRF protection with redirect checks.

---

### 5. Shell / Exec

**Codex CLI** -- three variants ([spec source](https://github.com/openai/codex/blob/main/codex-rs/core/src/tools/spec.rs), [handler source](https://github.com/openai/codex/blob/main/codex-rs/core/src/tools/handlers/shell.rs)):

`shell` (execvp-based):

> "Runs a shell command and returns its output. The arguments to `shell` will be passed to execvp(). Most terminal commands should be prefixed with ['bash', '-lc']. Always set the `workdir` param. Do not use `cd` unless absolutely necessary."

Parameters: `command` (string array), `workdir`, `timeout_ms`, sandbox permissions.

`shell_command` (user's default shell):

> "Runs a shell command and returns its output. Always set the `workdir` param. Do not use `cd` unless absolutely necessary."

Parameters: `command` (string), `workdir`, `timeout_ms`, `login` (bool), sandbox permissions.

`exec_command` (PTY-based):

> "Runs a command in a PTY, returning output or a session ID for ongoing interaction."

Parameters: `cmd`, `workdir`, `shell`, `tty` (bool), `yield_time_ms`, `max_output_tokens`, `login` (bool), sandbox permissions.

**OpenCode** -- `bash` ([source](https://github.com/anomalyco/opencode/blob/dev/packages/opencode/src/tool/bash.txt))

> "Executes a given bash command in a persistent shell session with optional timeout, ensuring proper handling and security measures. All commands run in ${directory} by default. Use the `workdir` parameter if you need to run a command in a different directory. AVOID using `cd <directory> && <command>` patterns -- use `workdir` instead."
>
> "IMPORTANT: This tool is for terminal operations like git, npm, docker, etc. DO NOT use it for file operations (reading, writing, editing, searching, finding files) -- use the specialized tools for this instead."

Parameters: `command` (required), `workdir`, `timeout` (ms, default 120000), `description` (5-10 word summary).

Output capped at configurable max lines/bytes; overflow written to file.

**OpenClaw** -- `exec` ([source](https://docs.openclaw.ai/tools/exec))

> "Run shell commands in the workspace. Supports foreground + background execution via `process`. If `process` is disallowed, `exec` runs synchronously and ignores `yieldMs`/`background`."

Parameters: `command` (required), `workdir`, `env`, `timeout` (seconds, default 1800), `background` (bool), `yieldMs` (default 10000, auto-background after delay), `pty` (bool), `host` (sandbox/gateway/node), `elevated` (bool), `security` (deny/allowlist/full), `ask` (off/on-miss/always), `node` (id/name).

`process` tool actions: `list`, `poll`, `log` (with offset/limit), `write`, `kill`, `clear`, `remove`, `send-keys`, `submit`, `paste`.

---

### 6. Todo / Task Tracking

**Codex CLI** -- `update_plan` (doubles as todo tracker) ([source](https://github.com/openai/codex/blob/main/codex-rs/core/src/tools/handlers/plan.rs))

> "Updates the task plan. Provide an optional explanation and a list of plan items, each with a step and status. At most one step can be in_progress at a time."

Parameters: `explanation` (optional), `plan` (array of `{step, status}`). Statuses: pending, in_progress, completed.

**OpenCode** -- `todowrite` ([source](https://github.com/anomalyco/opencode/blob/dev/packages/opencode/src/tool/todowrite.txt))

> "Use this tool to create and manage a structured task list for your current coding session. This helps you track progress, organize complex tasks, and demonstrate thoroughness to the user."
>
> When to use: "Complex multistep tasks (3+ steps), non-trivial tasks requiring planning, user explicitly requests it, user provides multiple tasks, after receiving new instructions, after completing a task, when starting new work."
>
> When NOT to use: "Single straightforward task, trivial task, less than 3 trivial steps, purely conversational."

Parameters: `todos` (array of `{id, content, status}`). Statuses: pending, in_progress, completed, cancelled. Rule: only one in_progress at a time.

**OpenClaw** -- No dedicated todo tool. Task tracking happens through:

- `cron` tool for scheduled recurring tasks
- Session management (`sessions_spawn`) for delegating subtasks
- `MEMORY.md` for persistent decisions/plans

---

### 7. Cron / Recurring Schedule

**Codex CLI** -- No cron tool.

**OpenCode** -- No cron tool.

**OpenClaw** -- `cron` ([docs](https://docs.openclaw.ai/cron-jobs), [cli](https://docs.openclaw.ai/cli/cron))

> "Manage Gateway cron jobs and wakeups."

Actions: `add`, `update`, `remove`, `run` (execute immediately), `runs` (execution history), `status`, `list`, `wake` (enqueue system event + optional immediate heartbeat).

Schedule types:

- `cron`: 5-field cron expression with optional IANA timezone
- `at`: One-shot reminder at specific ISO timestamp (deletes after run by default)
- `every`: Recurring interval-based schedule

Execution modes: `isolated` (dedicated agent turn) or `main` (enqueue on next heartbeat). Delivery: `announce`, `webhook`, or `none`. Jobs persist at `~/.openclaw/cron/jobs.json` across restarts. Recurring jobs use exponential retry backoff on consecutive errors.

---

## Nik's Current Toolset (10 tools)

- **WhatsApp**: `wapp_reply`, `wapp_react`, `wapp_set_presence`, `wapp_send_composing`, `wapp_send_paused`, `wapp_update_media_description`
- **AI**: `describe_media`
- **Schedule**: `alarm`
- **Data (privileged)**: `db_query`, `update_config`
- **CRM**: `update_contact`

## Already Planned

- **`exec` + `process`** -- full plan in [docs/plans/03-exec-process.md](03-exec-process.md)

## Candidate Tools for Nik

The six tools we're evaluating, with effort/impact notes:

| Tool               | Who has it                                                           | Effort                                                  | Impact for nik                                                               |
| ------------------ | -------------------------------------------------------------------- | ------------------------------------------------------- | ---------------------------------------------------------------------------- |
| `exec` + `process` | Codex, OpenCode, OpenClaw                                            | Medium (plan exists in `docs/plans/03-exec-process.md`) | High -- unlocks running commands on your machine                             |
| `web_search`       | Codex (built-in), OpenCode (Exa), OpenClaw (Brave/Perplexity/Gemini) | Low-Medium (API wrapper + config)                       | High -- real-world knowledge, lookups, news                                  |
| `web_fetch`        | OpenCode, OpenClaw                                                   | Low (HTTP GET + readability extraction)                 | Medium-High -- pairs with web_search, read articles/docs                     |
| `ask_question`     | Codex, OpenCode                                                      | Low (structured question + options via WhatsApp)        | Medium -- nik already talks via WhatsApp, but structured choices are cleaner |
| `todo` / `plan`    | Codex (update_plan), OpenCode (todowrite + plan-enter/exit)          | Low (state tracking, no external deps)                  | Medium -- helps nik break down complex requests                              |
| `cron`             | OpenClaw only                                                        | Medium (extends alarm infra, needs persistence)         | Medium-High -- "remind me every Monday", "check X daily"                     |

## Decision Point

Which tool(s) should we build next? Pick one or prioritize the order.
