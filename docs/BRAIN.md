# Brain

How the brain works, why it works that way, and the gotchas that matter.

This document covers nik's main loop — the conversation-handling brain. Task
workers are separate: each task gets its own activation with a single LLM call
(no continuous steering, no timeline). Workers run in parallel with the brain
and with each other. Their design is simpler and not covered here.

## The brain's single job

The brain wakes up, looks at the timeline for each conversation, and acts on
whatever is new. The timeline is split into two sections:

- **Already handled** — messages the brain has seen before.
- **New** — messages the brain hasn't seen yet.

The brain's job is to resolve everything in "New" — reply, spawn work, or
call `done` — then go back to sleep. When something new appears, it wakes
up again.

```
  ┌──────────────────────────────────────────────────┐
  │                    every 2s                      │
  │                                                  │
  │   tick                                           │
  │    ├── reflexes                                  │
  │    │    materialize internal state as timeline   │
  │    │    entries (fire alarms, flag stale tasks,  │
  │    │    emit skill events, reap shells)          │
  │    │                                             │
  │    └── check each conversation                   │
  │         anything new? ─── no ──── sleep          │
  │              │                                   │
  │             yes                                  │
  │              │                                   │
  │         activate                                 │
  │          ├── consume timeline (marks as read)    │
  │          ├── recall relevant memories            │
  │          ├── build prompt                        │
  │          └── LLM loop (continuous steering)      │
  │               ├── tool calls → execute → next    │
  │               └── done + empty round → exit      │
  │                                                  │
  └──────────────────────────────────────────────────┘
```

**Files:** `internal/brain/brain.go`

## Everything is a message

The timeline is a flat, chronological list of messages. There are only two
sources:

1. **Real messages** — someone (including nik) sent a message in a
   conversation.
2. **System messages** — a reflex or tool side effect wrote an event into the
   conversation (`task_spawned`, `alarm_fired`, `skill_added`, etc.).

Both are stored in the same `message` table with the same shape. The brain
doesn't distinguish between them when checking for new activity — anything
after `last_read_at` triggers an activation. This means a single mechanism
drives all of nik's behavior: a human texting, an alarm firing, a task
completing, and a skill changing all look the same to the brain. They're
messages in a timeline.

Not all system messages deserve the brain's attention — see
[System messages](#system-messages-and-what-should-trigger-activation) for the
activatable vs passive distinction.

## The consumable timeline

The timeline is consumable: reading it marks it as read. Delivery is
at-most-once — messages appear as "New" exactly once. The moment the brain
reads the timeline for a conversation, the read marker advances to now, and
those messages become "Already handled" for all future reads.

This has a critical consequence: **anything that needs the same view of the
timeline must cache and reuse the first read.** Subsequent calls to get the
timeline will return a different snapshot because the read marker has advanced.
The brain prefetches the timeline once and shares that cached value between
recall (which needs it to select relevant memories) and the first LLM round
(which needs it as context). After that first use the cache is consumed, and
later rounds get fresh reads.

**`last_read_at`** is the single timestamp on each conversation row. Everything
before it is "Already handled". Everything strictly after it is "New".

- `check()` is read-only — it peeks at whether anything is after the read
  marker without advancing it.
- `Get()` reads AND advances the marker. Both use the same marker; the
  difference is mutation.

**`markRead` also sends platform read receipts** (WhatsApp blue ticks) for
inbound messages up to the new read time.

**Files:** `internal/timeline/sense.go`, `internal/messaging/service.go`

## Continuous steering (zero accumulation)

The brain does not accumulate messages between rounds. After each tool
execution, the provider is **reset** and a fresh timeline is read from the
database. Each LLM round is a self-contained, single-turn API call whose sole
user message is the full timeline.

Tool calls are stored as system messages in the `message` table
(`platform: "system"`, `kind: "tool_call"`). When the timeline is re-read next
round, those tool call messages appear naturally — interleaved chronologically
with user messages, sent-message echoes, and other system events. The model
sees cause before effect: a tool call entry sits at its real timestamp, between
the input that triggered it and the messages it produced.

```
  round 0:  reset → Read() → timeline (no tool calls yet)
            model calls message_send → execute → insert tool_call system message

  round 1:  reset → Read() → timeline includes [tool call] message_send + echoed YOU messages
            model calls done → execute → insert tool_call system message → exit
```

Each round's API call contains exactly one user message (the timeline). There
is no accumulated tool call/result pair history — the timeline IS the history.
This eliminates temporal inversion (effects appearing before causes) and makes
the context idempotent: same DB state produces the same timeline.

Tool calls are rendered as `YOU:` messages in the timeline (not `system:`),
since they represent nik's own actions.

**Files:** `internal/brain/brain.go` (`think`), `internal/brain/tools.go`
(`insertToolCallMessages`), `internal/timeline/system.go` (`renderToolCall`)

## The `done` tool and the end of an activation

Every activation ends with the model calling `done`. The `done` tool takes a
`reason` parameter (for debug logging) and signals that the model has finished
all its work for this activation. When the brain detects `done` among the
round's tool calls, it saves the done tool call as a system message (same as
every other tool call) and exits immediately. There is no trace round —
`done.reason` and the round's reasoning summaries are the debugging artifacts.

**Done self-reactivation prevention.** Done is saved to DB like every other
tool call (available for debugging via SQL), but invisible at two layers:

1. **Query excludes done.** The `message_list.sql` query filters out done tool
   calls: `AND NOT (m.kind = 'tool_call' AND json_extract(m.body, '$.name') = 'done')`.
   Done messages never leave the database. Both `Check()` and the timeline
   renderer use the same query, so done is invisible everywhere.

If the model produces zero tool calls without calling `done`, the brain injects
`nik-05-retry.md` as a follow-up user message (one shot). This nudge warns
about common failure modes and insists on at least one tool call. If the nudge
also produces no tool calls, the activation fails.

**Worker nudge:** task workers have an analogous mechanism. If a worker produces
text with no tool calls (meaning it finished without calling `task_report`), it
checks whether a final report already exists. If not, `task-01-nudge.md` is
injected once. If the worker still produces no tool calls, the task is resolved
based on the last report status (or fails if no report exists).

**Files:** `internal/brain/brain.go` (`think`),
`internal/brain/tools.go` (`doneToolDef`, `doneHandler`),
`internal/task/runner.go` (`runLoop`),
`prompts/nik-05-retry.md`, `prompts/task-01-nudge.md`

## Self-reactivation: continuity across activations

When nik sends a message via `message_send`, the outbound message is stored in
the database with a `sent_at` timestamp after the current `last_read_at`. On
the next tick, `check()` sees it as new and triggers another activation. The
model sees its own previous reply as a `YOU` message in "Already handled".

This is by design. The brain is built for high-frequency conversations and
group chats where nik's reply is rarely the last message. Marking nik's own
messages as read preemptively would create race conditions — new messages from
others could land between the reply and the mark, and get silently buried. The
complexity to solve that isn't worth it. So nik always sees its own messages as
"New" on the next tick, re-evaluates, and decides whether to act or call `done`.

This wastes tokens when nik is the only entry in the "New" section, but it also
tests the model's self-restraint — it must recognize there's nothing to do and
call `done` instead of talking to itself.

The same applies to all tool side effects: `task_spawned`, `task_report`,
`alarm_updated`, etc. are stored as system messages. They land after
`last_read_at` and trigger re-activation so the brain can react to its own work
completing.

**If this creates a loop, the bug is in the model's decision-making**
(re-handling something it already handled), not in the reactivation mechanism.
Never suppress self-reactivation as a fix.

## Reflexes: constructing the timeline

Reflexes run before every `check()`. They are not the brain's core logic —
they're triggers that mutate DB state so the timeline has something new to show:

- `FireDueAlarms` — creates alarm_occurrence rows and emits `alarm_fired`
  system messages when alarms are due.
- `StaleAlarmReflex` — detects recurring alarms with no upcoming fire time and
  emits `alarm_stale` events.
- `CheckStale` — flags tasks with no activity and inserts stale task reports.
- `SkillChangeReflex` — detects skill file additions, removals, and changes;
  emits `skill_added`/`skill_removed`/`skill_changed` events.
- `SkillCheckReflex` — runs skill-declared check commands (declared in SKILL.md
  frontmatter via `reflex:` block). Pipes the previous record via stdin, checks
  stdout. Non-empty + different = new record, stored in `skill_reflex` table and
  emits `skill_reflex_fired` system message.
- `CheckSessions` — reaps dead/stale shell sessions.

Without reflexes, the brain would still work — it would just wait passively for
external events (inbound messages). Reflexes make the brain self-aware of
internal state changes by materializing them as timeline entries.

**DB wipe recovery:** reflexes recover gracefully from a wiped event table. If
all `skill_event` rows are deleted, the skill change reflex re-detects all
skills as `added` and re-emits events. Install sections are idempotent.

## System messages and what should trigger activation

As described in [Everything is a message](#everything-is-a-message), system
messages are stored alongside real messages and the brain treats them
identically when checking for new activity. The brain doesn't have to act on
all of them — most will result in `done`. We never decide for the model; it sees
everything and chooses. This is a deliberate test of model performance. We
manage it by tweaking prompts and adding simple code fallbacks only when
necessary.

**Typically actionable** (nik should usually do something):

- Any non-system message — user messages, nik's own echoes
- `alarm_fired`, `alarm_stale` — require model action
- `skill_added`, `skill_removed`, `skill_changed` — may require install steps
- `trigger` — explicit activation request
- `skill_reflex_fired` — skill check command detected something new
- `task_report` (completed/failed) — task finished, model should review

**Typically `done`** (nik should usually ignore):

- `task_report` (running) — progress updates (though a nonsensical report
  might warrant stopping the task)
- `task_spawned`, `task_retry`, `task_cancelled` — echoes of model's own actions
- `alarm_created`, `alarm_updated` — confirmations
- `media_processed` — bookkeeping

**Known risk:** `check()` does not distinguish between these categories. Any
entry after `last_read_at` triggers activation, so passive system messages
cause activations where the correct answer is `done`. When nik does something
unexpected, we analyze the timeline to understand why and improve the prompt or
reasoning — not add filtering in code.

## Recall

Recall is part of nik's memory management system. The memory skill
(`skills/memory/SKILL.md`) owns how memories are created, updated, and stored.
Recall is the read path — selecting which memories are relevant before the
model thinks.

Before the first LLM round, the brain runs a lightweight recall pass. It reads
`memories/latest.md` (a markdown table of facts about people, preferences,
events), sends the numbered rows plus the prefetched timeline to a fast LLM
call (no tools, 30s timeout), and gets back which rows are relevant. The
selected rows are injected into the prompt under "## What you remember".

Recall uses the same prefetched timeline that round 0 will see. This is why the
prefetch cache exists — recall and round 0 must share the same snapshot.

If recall fails or finds nothing relevant, the section is omitted silently.

**Files:** `internal/recall/recall.go`

## Prompt assembly

The system prompt is assembled from template files:

- `nik-00-base.md` is the root Go template. It pulls in named sub-templates:
  `identity` (01), `conversation` (02), `skills` (03), `brain` (04).
- Template data: current time/timezone/location, soul state, breathing state,
  recall results, DB table list, tool inventories (worker vs nik-only), banned
  words, preloaded + available skills.
- **Hooks** allow model-specific prompt patches. Markdown files in
  `workspace/prompts/` with YAML frontmatter specify target models, section, and
  mode (append or replace). Applied per-section before template parsing.

**Files:** `internal/brain/prompt.go`, `internal/brain/hooks.go`, `prompts/*.md`

## Concurrency

The goal is high responsiveness — the user shouldn't wait for nik to finish
thinking. Two mechanisms make this possible:

1. **Tasks** offload real work. Nik spawns a task and moves on immediately.
2. **Continuous steering** lets nik process new messages between tool calls and
   activation rounds, so it reacts to the conversation as it evolves.

Because continuous steering already handles mid-activation updates, we only
need one activation per conversation at a time (`SyncSet`). New stimuli for a
conversation with an active activation are skipped — thanks to how
[the consumable timeline](#the-consumable-timeline) works, the running
activation will see them on its next round.

- Multiple conversations activate concurrently, up to 6 concurrent LLM sessions
  (semaphore).
- Tasks run as independent activations, parallel with the brain and each other.
- Within a round, multiple tool calls execute in parallel.
- Activation timeout: 20 minutes.

## Data flow

```
tick
 ├── config reload
 ├── reflexes
 │    materialize internal state changes as timeline entries
 │    (fire alarms, detect stale tasks, emit skill events, reap shells)
 │
 └── sensor.Check
      for each allowed conversation:
        peek: any entry after last_read_at? ── no ── skip
                         │
                        yes ── Stimulus

      for each Stimulus (one goroutine per conversation):
        activate
          consume timeline ── cache snapshot ── markRead(now)
          recall(snapshot) ── relevant memories
          build prompt(now, recall)
          create llm.Activation

          brain loop (zero accumulation):
            round 0: user msg = cached snapshot
            each round:
              session.Round ── model output
              API error? ── retry up to 3× with backoff, else fail
              tool calls? ── execute in parallel
                             insert tool_call system messages to DB
                             done in calls? ── exit immediately
                             else reset provider ── fresh timeline read ── next round
              no tool calls? ── nudge once (nik-05-retry.md), else fail
              identical tool calls 4× in a row? ── fail (loop detection)

        release conversation lock
```

## Ownership boundaries: llm vs callers

The `llm` package is a dumb API client. It manages protocol state (the items
array, model params, streaming, pruning) but makes zero decisions about when to
stop, when to retry, or what to do with results. One call in, one result out.

The `brain` and `task runner` own the round loop and all policy:

- **5xx / transient error retry** with exponential backoff (up to 3 attempts)
- **Loop detection**: consecutive identical tool call signatures, fail after 4
- **Idle nudge**: brain loads `nik-05-retry.md`, runner loads `task-01-nudge.md`,
  injected as user messages — one shot
- **Done detection**: brain checks for `done` tool in each round's calls;
  activation exits immediately when `done` is called
- **Activation ID**: generated by the caller, passed via context metadata

The `llm.Activation` type wraps the items array and provides: `SetInput` (replace
items[0]), `Round` (single API call, no retries), `AddToolResult`,
`ResetConversation`, `Prune`, `Usage`, `AppendAssistantText`,
`AppendUserMessage`.

**Files:** `internal/llm/activation.go`, `internal/brain/brain.go` (`think`),
`internal/task/runner.go` (`runLoop`)

## Known issues

**markRead before model decides** — consuming the timeline advances the read
marker before the model chooses what to do. A bad `done` (calling `done` without
handling new messages) buries unhandled messages with no recovery until an
unrelated event arrives. Root cause of the 3-hour "Send it to him" delay.

**Rejected fix: defer markRead until after activation.** This breaks
at-most-once delivery. If markRead is deferred, a crash or timeout mid-activation
leaves the read marker behind. The next tick re-reads the same messages,
triggers a new activation, and the model acts on them again — duplicate replies,
duplicate tasks, duplicate side effects. The consumable timeline's core guarantee
is that messages appear as "New" exactly once. Deferring markRead violates that.
The fix for the done-buries-messages problem must preserve at-most-once; it
cannot move markRead later in the pipeline.
