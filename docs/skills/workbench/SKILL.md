---
name: workbench
description: >-
  Iterative prompt A/B testing workbench. Use when the user wants to diagnose
  a message handling issue, establish a baseline, test prompt changes, and
  measure improvements. Replaces the diagnose-message skill.
---

# Prompt Workbench

Diagnose a message handling issue, replay the activation round, test prompt changes as hypotheses, and iterate until the desired behavior rate improves. All state is stored in the database. Each experiment gets a folder at `workbench/<date>-<short_id>/` containing the report and working files (patch files, surface dumps).

## CLI reference

All commands go through `make workbench ARGS="..."`. Every mutation auto-renders the report to `workbench/<date>-<short_id>/report.md`.

| Command | Usage |
|---------|-------|
| `create-experiment` | `-activation_round_id <id> -desired_outcome '<text>' -analysis '<text>'` |
| `update-experiment` | `-experiment_id <id> [-status <s>] [-desired_outcome <d>] [-analysis <a>]` |
| `create-experiment-variant` | `-experiment_id <id> -name '<name>' -hypothesis '<text>' [-patches <file>] [-reasoning_effort '<e>'] [-verbosity '<v>']` |
| `create-experiment-variant-run` | `-experiment_variant_id <id> -n <count> [-max_rounds <int>] [-json]` |
| `update-experiment-variant-run` | `-experiment_variant_run_id <id> -is_desired true\|false -rationale '<text>'` |

---

## Input

The user needs to provide:
- A message (quoted text, message ID, or description of the incident)
- A description of what went wrong
- The desired behavior

Ask questions until the user provides all of these.

## Find the message the user is referring to

- Find the message via the sqlite cli.
- Fetch few messages before and after and the conversation they belong to.
- Ask the user to confirm this is a match, if not ask for more info.
- Do not make further progress until the user confirms the message has been found.

For example if the user provides some text, use the sqlite cli:

```sql
SELECT m.id, m.body, m.sent_at, m.is_from_me, m.conversation_id, c.name
FROM message m JOIN contact c ON c.id = m.contact_id
WHERE m.body LIKE '%<text>%'
ORDER BY m.sent_at DESC LIMIT 10;
```

## Find the activation round

Now we need the activation round where nik first saw this message. Rounds store the formatted timeline in `user_input`. Search all rounds created after the message was received for the first one where it appears in the `### New` section:

```sql
SELECT
  ar.id AS round_id,
  ar.round,
  a.id AS activation_id,
  a.model,
  a.created_at
FROM activation_round ar
JOIN activation a ON a.id = ar.activation_id
WHERE ar.user_input LIKE '%<message text>%'
  AND a.created_at >= '<message sent_at>'
ORDER BY a.created_at ASC
LIMIT 5;
```

Read the `user_input` of the first result and confirm the message is under `### New`, not `### Already handled`. That's the round to replay.

## Create the experiment

Create the experiment with `create-experiment` as soon as you have the round. The report auto-renders — open the doc. From here, the user reviews the rendered report, not chat.

## Analysis

Read the round's `model_output`, `reasoning_summaries`, and tool calls. Compare what actually happened vs what the user wanted. Explain WHY — read the `user_input` (`### New` section) and the `reasoning_summaries`. Build the causal chain: "the model saw X, which led it to conclude Y, which caused action Z."

Store the analysis via `update-experiment -experiment_id <id> -analysis '<text>'`. Ask and address any feedback — update via the CLI until the user is satisfied.

## Baseline replay

Once the user confirms, look up the baseline variant ID from the experiment output. Run 10 replays with `create-experiment-variant-run -experiment_variant_id <id> -n 10`. Review results, mark each run with `update-experiment-variant-run -experiment_variant_run_id <id> -is_desired true|false -rationale '<text>'`. The report auto-renders after each command.

## Hypothesize

Based on the trace and baseline results, propose one hypothesis — a specific prompt or skill change stored as a variant via `create-experiment-variant`.

### Creating a patch

Create a single `v<N>.patch` file in the experiment folder containing **unified diffs** for all surfaces you want to change. Use the surface names as file paths in the diff headers (e.g. `a/instructions`, `b/instructions`). Pass it to `create-experiment-variant -patches v<N>.patch`.

Supported surfaces:

- `instructions` — the system prompt (flat text)
- `messages/<index>/content` — content of a specific message in the conversation history (default field is `content` if omitted: `messages/<index>` is equivalent to `messages/<index>/content`)
- `messages/<index>/name` — name field of a message (e.g. tool name)
- `tools/<name>/<field>` — field in a tool definition (e.g. `tools/done/Description`)

Messages are a flat array of all conversation entries (user inputs, assistant outputs, tool calls, tool results, nudges) recorded across all rounds up to the target round. Use `messages/0/content` for the timeline, or find the nudge/tool result by index.

For JSON surfaces (`tools/`), the field value is extracted as real text with actual newlines — you never diff escaped JSON.

Example `v1.patch`:

```diff
--- a/instructions
+++ b/instructions
@@ -12,3 +12,4 @@
 When you see only system events under ### New, call done.
+Do NOT re-acknowledge a user request that appears under ### Already handled.
 
--- a/messages/2/content
+++ b/messages/2/content
@@ -5,3 +5,3 @@
-Call `message_send` now with this text. Do not rephrase or add to it. Then call `done`.
+If this text was meant as a message, call `message_send`. If internal reasoning, call `done` with a reason.
```

Create the variant (report auto-renders). Present the hypothesis to the user. Ask and address any feedback.

## Iterate

Run 10 replays with `create-experiment-variant-run`. Review the output, mark each run with `update-experiment-variant-run`. The report auto-renders after each command. Present results to the user and brainstorm what to try next.

The highest-scoring variant is the **anchor** (baseline at the start). You're free to try new approaches, but if a variant scores lower, return to the anchor and try something different. When a variant beats the anchor, it becomes the new anchor. Repeat.

Each iteration requires user review unless the user explicitly asks you to work independently.

## Conclude

After each variant, ask the user: continue iterating or conclude?

If they want to conclude, ask whether to apply the anchor's patches to the codebase. If yes, apply them to the actual source files. Update status with `update-experiment -experiment_id <id> -status complete`.

## Rules

- The agent interprets each run's output against the desired behavior and sets `is_desired` with `update-experiment-variant-run`. When in doubt, ask the user.
- Every `update-experiment-variant-run` must include a short `-rationale` describing what nik actually did in human terms — e.g. "nik sent a message and rescheduled the alarm" or "nik sent the message but forgot to reschedule". Never describe outcomes in tool-call names; the rationale is how you communicate the result to the user in the report.
- Never apply patches to source files without explicit user approval.
- The report is the primary artifact — re-render after every change.
