---
name: workbench
description: >-
  Iterative prompt A/B testing workbench. Use when the user wants to diagnose
  a message handling issue, establish a baseline, test prompt changes, and
  measure improvements. Replaces the diagnose-message skill.
---

# Prompt Workbench

Diagnose a message handling issue, replay the activation round, test prompt
changes as hypotheses, and iterate until the desired behavior rate improves.
All state is stored in the database. Results are rendered to
`workbench/<short_id>.md`.

## Input

The user provides:
- A message (quoted text, message ID, or description of the incident)
- A description of what went wrong

## Workflow

Two phases: **analysis** (always runs first, stops for user confirmation)
and **experiment** (only after user approves).

### Analysis Phase

#### 1. Diagnose (interactive)

Find the message via the `db_query` tool:

```sql
SELECT m.id, m.body, m.sent_at, m.is_from_me, m.conversation_id, c.name
FROM message m JOIN contact c ON c.id = m.contact_id
WHERE m.body LIKE '%<text>%'
ORDER BY m.sent_at DESC LIMIT 10;
```

Show the candidate message with surrounding context — messages before and
after, the conversation name, and participants. **Confirm with the user**
that this is the correct message before proceeding.

Then trace to the activation round that handled it:

```sql
SELECT a.id, a.model, a.reasoning_effort, a.error, a.created_at,
  ar.id AS round_id, ar.round, ar.model_output, ar.reasoning_summaries
FROM activation a
JOIN activation_round ar ON ar.activation_id = a.id
WHERE a.conversation_id = '<conv_id>'
  AND a.created_at >= '<sent_at minus 5 seconds>'
ORDER BY a.created_at ASC, ar.round ASC
LIMIT 20;
```

Identify the specific `activation_round.id` that should have handled the
message.

#### 2. Desired vs actual (interactive)

Read the `activation_round` data: `model_output`, `reasoning_summaries`,
and the tool calls for that round. State what actually happened.

**Always ask the user** to confirm or describe the desired behavior. Never
assume what the correct behavior should be.

#### 3. Trace

Explain WHY the model behaved as it did. Read the round's `user_input`
(search for `### New` in it) and the `reasoning_summaries`. Quote the
specific content that misled or informed the model. Build the causal chain:
"the model saw X, which led it to conclude Y, which caused action Z."

#### 4. Baseline replay

Create the experiment and run 10 baseline replays:

```bash
make workbench ARGS="experiment create -round <round_id> -desired '<behavior>'"
```

Note the experiment ID. Then run baseline:

```bash
make workbench ARGS="replay -round <round_id> -n 10 -desired <key> -json"
```

For each result, record it with the `db_query` tool or use the variant ID
from the experiment's baseline variant. Parse the JSON output to determine
hit/miss for each attempt.

Generate the report:

```bash
make workbench ARGS="experiment report -experiment <experiment_id>"
```

#### 5. Stop and ask

Present the analysis summary to the user:
- What happened and why (trace)
- Baseline success rate (N/10 desired)
- The report file location

Ask: "Should we proceed with experiments to improve this?"

**Do not continue to the experiment phase without explicit user approval.**

### Experiment Phase

Only after the user confirms.

#### 1. Update experiment status

```bash
make workbench ARGS="experiment status -experiment <id>"
```

Update the experiment status to `experimenting` via `db_query`:

```sql
UPDATE experiment SET status = 'experimenting', updated_at = NOW_ISO8601_MS()
WHERE id = '<experiment_id>';
```

#### 2. Hypothesize

Propose specific prompt-file-level changes with justification. Each change
is a hypothesis with an expected impact on the desired behavior rate.

For each hypothesis:
- Name the source prompt file (e.g. `prompts/nik-04-brain.md`)
- Describe the specific text change
- Explain how it addresses the root cause from the trace
- State the expected improvement

Write the patches JSON to a temp file:

```json
[{"file": "prompts/nik-04-brain.md", "old": "existing text", "new": "modified text"}]
```

Create the variant:

```bash
make workbench ARGS="experiment variant -experiment <id> -name '<name>' -hypothesis '<text>' -patches /path/to/patches.json"
```

Regenerate the report — it now shows the variant with hypothesis and diff,
status `proposed`.

```bash
make workbench ARGS="experiment report -experiment <id>"
```

#### 3. User approves

Present the hypothesis and diff to the user. **Wait for approval** before
running replays.

#### 4. Test variant

```bash
make workbench ARGS="replay -round <round_id> -n 10 -desired <key> -variant <variant_id> -json"
```

Record each result. Regenerate the report:

```bash
make workbench ARGS="experiment report -experiment <id>"
```

#### 5. Compare

```bash
make workbench ARGS="experiment status -experiment <id>"
```

Present the comparison: baseline vs variant success rates.

#### 6. Loop or stop

If improvement is sufficient, propose applying the patch to the source
prompt file. Show the user the exact edit and ask for confirmation.

If not sufficient, propose a new variant and repeat from step 2.

When the user accepts a variant:
1. Apply the patch to the actual prompt file using the edit tool
2. Update experiment status to `complete`
3. Regenerate the final report

## Rules

- Never make prompt file changes without explicit user approval
- Never assume the desired behavior — always confirm with the user
- Always regenerate the report after each major step
- Present data, not opinions — let the success rates speak
- Every hypothesis must trace back to the root cause analysis
- The report in `workbench/<short_id>.md` is the primary artifact
