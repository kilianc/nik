# Nik

Nik (Noetic Intelligence Kernel) is an autonomous personal AI that lives on WhatsApp. Not an assistant -- a family member. It has its own phone number, its own personality, and genuine relationships with the people it talks to. Built in Go, backed by SQLite, powered by LLMs.

## Philosophy

- **Highest autonomy** -- nik runs on its own. No human in the loop, no babysitting.
- **Smallest codebase** -- small enough for one person (or one AI) to fully grok. Every line earns its place.
- **Core tools + extensible skills** -- a small set of powerful built-in tools and a growing set of user-defined skills that compose them.

## The Brain Loop

The brain is the core of the system. It's a polling loop that checks for things to do and handles them.

```
Awake(ctx, 2s)
│
├─ perceive
│   ├─ for each DataSource → Check(ctx)
│   │   └─ returns []DataSourceOutput (lines of context + metadata)
│   │
│   └─ for each output
│       ├─ skip if conversation already active (dedup)
│       └─ go activate(ctx, output)
│           ├─ Processing callback (e.g. mark read)
│           ├─ think
│           │   ├─ loadInstructions (prompt + skills + soul + time)
│           │   ├─ join input lines
│           │   └─ llm.Complete (loop: completion → tool calls → execute → repeat)
│           └─ Processed callback (e.g. mark alarm fired)
│
└─ (repeat every 2s until ctx cancelled)
```

Every 2 seconds the brain calls `Check()` on each registered data source. Each source returns zero or more outputs -- chunks of context with metadata like `conversation_id`. For each output, the brain spawns a goroutine (an activation): it loads the system prompt (personality + preloaded skills + soul), concatenates the input, and thinks -- which under the hood means calling `llm.Complete`. The LLM can make tool calls (reply, set alarms, run shell commands), and each result feeds back into the completion loop until the model is done.

One activation = one shot. There are no follow-up turns. When the model returns, it's over. This constraint forces the model to do all its work -- searching, thinking, replying -- in a single burst.

```mermaid
flowchart TD
    Awake["Awake (polling loop)"] --> Tick

    subgraph perceiveStep [Perceive]
        CheckSources["Check each DataSource"] --> Outputs["Collect DataSourceOutputs"]
        Outputs --> Dedup{"Already\nactive?"}
        Dedup -- yes --> Skip[Skip]
        Dedup -- no --> Activate["Activate"]
    end

    Activate --> Processing["Processing callback"]
    Processing --> LoadPrompt["Load instructions\n(prompt + skills + soul)"]
    LoadPrompt --> ThinkStep["think"]

    subgraph completeLoop [LLM Complete Loop]
        ThinkStep --> LLM["LLM completion"]
        LLM --> ToolCalls{"Tool calls?"}
        ToolCalls -- yes --> Exec["Execute tools"]
        Exec --> ThinkStep
        ToolCalls -- no --> Done["Return output"]
    end

    Done --> Processed["Processed callback"]
```

## Data Sources

A data source is anything that can produce work for the brain. Each one implements a single method: `Check(ctx) ([]DataSourceOutput, error)`. The brain doesn't know or care what generates the inputs -- it just polls.

| Source | Trigger | What it produces |
|--------|---------|------------------|
| **messaging** | unread conversations in the allow list | conversation history + session context + new messages |
| **alarms** | a reminder's `fire_at` has passed | the alarm goal + conversation context |
| **shell** | a tmux session exited or hit its check-in time | session output for review |
| **journal** | end of day (configurable time) | the day's conversations and memories for reflection |
| **dream** | nightly passes (1-4 hourly after journal) | prior dreams + memories for subconscious processing |
| **briefing** | morning after wake | daily briefing context |

Each output carries metadata (`conversation_id`, `message_id`, etc.) and optional lifecycle callbacks (`Processing` runs before the LLM, `Processed` runs after).

## Messaging: Adapters and the Canonical Model

Messaging is split into two layers:

**Canonical layer** -- platform-agnostic tables (`conversation`, `message`, `media`, `contact`) are the source of truth. Every message nik sends or receives lives here with a UUIDv7 primary key, regardless of where it came from.

**Adapter layer** -- each platform implements `MessagingPlatform`: normalize inbound events into canonical models, execute outbound actions (reply, react, typing indicators, read receipts). Currently there's one adapter: WhatsApp via whatsmeow.

The two interfaces that connect them:

```go
// inbound -- adapters push events through this
type MessageReceiver interface {
    ReceiveConversation(ctx, conv)
    ReceiveMessage(ctx, msg)
    OnHistorySyncComplete(ctx, platform)
}

// outbound -- brain tools call these via the service
type MessagingPlatform interface {
    Reply(ctx, externalConversationID, body)
    React(ctx, externalConversationID, externalMessageID, emoji)
    StartTyping / StopTyping / SetPresence / MarkRead
}
```

The full flow of a message through the system:

```mermaid
flowchart LR
    subgraph whatsapp [WhatsApp]
        WA_In(("incoming\nmessage"))
        WA_Out(("outgoing\nmessage"))
    end

    subgraph adapter [Adapter]
        Normalize["Normalize to\ncanonical model"]
        Execute["Execute platform\naction"]
    end

    subgraph service [Messaging Service]
        Receive["ReceiveMessage\n(upsert conv, contact,\nmessage, media)"]
        Reply["Reply\n(typing → delay → send\n→ ReceiveMessage)"]
    end

    subgraph db [SQLite]
        Tables[("conversation\nmessage\nmedia\ncontact")]
    end

    subgraph datasource [Data Source]
        Poll["PollUnreadConversationIDs\n→ build context"]
    end

    subgraph brainBox [Brain]
        BrainLoop["activate → think"]
    end

    subgraph llmBox [LLM]
        LLM_Complete["Complete + tool calls"]
    end

    WA_In --> Normalize --> Receive --> Tables
    Tables --> Poll --> BrainLoop --> LLM_Complete
    LLM_Complete -- "message_reply" --> Reply --> Execute --> WA_Out
    Execute --> Receive
```

When a message arrives: the WhatsApp adapter normalizes it and calls `ReceiveMessage`, which upserts the conversation, resolves/creates the contact, and inserts the message. On the next perceive cycle, the messaging data source polls for unread conversations, builds the context (history + session info + participant profiles), and hands it to the brain. The brain activates, thinks, and calls tools -- most commonly `message_reply`, which goes back through the service to the adapter and out to WhatsApp. The outbound message is also fed back through `ReceiveMessage` so it appears in the canonical history.

## Autonomous Systems

These run on schedule via alarms — the brain activates them like any other stimulus. Core alarms use `[NIK_XXX]` goal prefixes (e.g. `[NIK_JOURNAL]`, `[NIK_DREAM_1]`) and are enforced by the `CoreAlarmEnforcer` reflex in `internal/alarms/core.go`. The reflex creates missing alarms and heals dead ones (null/past `next_fire_at`) using schedule times from config. Skills still document the alarm format as a fallback.

- **Journal**: managed entirely by the `journal` skill. Nik uses a recurring alarm, gathers day context via `db_query`/`shell`, and writes to `journal/` files. No domain package.
- **Dream**: managed entirely by the `dream` skill. Nik uses 5 recurring alarms (one per dream pass), processes the journal and memories, and writes to `dreams/` files. The final pass (Wake) evolves nik's **soul** — a living identity document stored in `soul/latest.md` and loaded into the system prompt on every activation. Dated snapshots in `soul/YYYY-MM-DD.md` preserve history. No domain package.
- **Briefing**: managed entirely by the `briefing` skill. Nik uses a recurring alarm, `web_search` for news, and writes to `briefings/` files. No domain package.
- **Diagnostic**: managed entirely by the `diagnostic` skill. Nik uses a recurring alarm, discovers skills/services, tests auth, verifies alarm chains and skill outputs, checks data integrity and spending. Writes to `diagnostics/` files. No domain package.

## Tasks and the Timeline

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

## Tools

Domain packages define tools via `BuildTools()` and register them at startup. The brain makes them available to the LLM during activations. Some tools are privileged (owner-only).

**messaging** -- `message_reply`, `message_react`, `message_set_presence`

**shell** -- `shell` (tmux: run, read, send, kill, list)

**alarms** -- `alarm`

**contacts** -- `update_contact`

**search** -- `db_query`

**llm** -- `describe_media`

**websearch** -- `web_search`

**skills** -- `load_skill`

**config** -- `config`

**journal** -- `journal_write`

**dream** -- `dream_write`, `soul_evolve`

**briefing** -- `briefing_write`
