# Skill Reflexes

The brain is reactive — it sleeps until the timeline has something new. Without reflexes, nik would only wake when a human sends a message. Reflexes are the nervous system: involuntary checks that run on a schedule, sense changes in the outside world, and translate them into timeline entries the brain can act on.

Two modes:

- **Command-based** — a script decides what's new. The system runs it, compares stdout to the last record, and fires only when something changed.
- **Schedule-only** — no script. The system fires unconditionally on cron. The skill itself handles discovery when the brain loads it.

## Declaration

Reflexes are declared in the `reflex:` block of a skill's YAML frontmatter. Each item requires `name` and `every`. `command` is optional — omit it for schedule-only.

```yaml
# command-based: script decides what's new
reflex:
  - name: check_gmail
    command: sh skills/google_workspace/check_gmail.sh
    every: every 15 minutes

# schedule-only: fires unconditionally on cron
reflex:
  - name: nightly_review
    every: every day at 11pm
```

Rules:

- `name` + `every` are required. Items missing either are silently dropped.
- `every` is natural language (see [Schedule resolution](#schedule-resolution)).
- A skill can declare multiple reflexes. Each is independent.
- Parsing is hand-rolled in `parseFrontmatter` — only the shapes shown above are recognized.

**Type:** `SkillReflexDef` in `internal/skills/tools.go`:

```go
type SkillReflexDef struct {
    Name    string
    Command string // empty for schedule-only reflexes
    Every   string // natural language schedule
}
```

## Schedule resolution

The `every:` text is converted to a 5-field cron expression on first encounter and cached permanently.

```
every: "every 15 minutes"
         │
         ▼
  ┌─ every_to_cron table (cache) ─┐
  │  natural_text = "every 15 …"  │
  │  cron_expr    = "*/15 * * …"  │
  └───────────────────────────────┘
         │ miss?
         ▼
  LLM call (cronSystemPrompt)
  → response: "*/15 * * * *"
  → parse with internal/cron
  → INSERT OR IGNORE into cache
```

- Cache key is the exact `every` string. Different wording = different entry.
- The LLM call uses a lightweight prompt with examples — no tool calls.
- `internal/cron` parser, minute granularity (no seconds).

**Files:** `internal/skills/every_to_cron.go` (`resolveCron`, `Completer`, `cronSystemPrompt`)

## When does a reflex fire?

`SkillCheckReflex` runs every 5 minutes (registered in `cmd/nik/main.go`). On each tick:

```
for each reflex in ListReflexes():
    sched  = resolveCron(def.Every)
    latest = db.SkillReflexLatest(key)  // last meta + created_at

    baseline = latest.created_at        // last fire time
    if never fired:
        baseline = start of local calendar day (midnight today)

    next = sched.NextAfter(baseline)    // first matching minute strictly after
    if next > now:
        skip                            // not due yet

    runSkillCheck(key, def, latest.meta)
```

- **Key** = `skillName/reflexName` (e.g. `google_workspace/check_gmail`).
- **Baseline** is the `created_at` of the latest `skill_reflex` row for that key. If no row exists (first run), baseline defaults to the start of the current local calendar day, so a reflex declared "every 15 minutes" fires immediately on first tick rather than waiting until the next day.

## Command-based contract

The system runs the script and compares its output to the previous record.

**Invocation:**

| Aspect | Detail |
|--------|--------|
| Command | `def.Command` run via `CommandRunner` (tmux shell) |
| stdin | previous `meta` string from DB (empty on first run) |
| Timeout | 30 seconds (`context.WithTimeout`) |
| Working dir | nik's home directory |

**Decision matrix (stdout after trimming whitespace):**

| stdout | exit code | result |
|--------|-----------|--------|
| empty | 0 | skip — nothing new |
| same as last `meta` | 0 | skip — no change |
| different from last `meta` | 0 | **fire** — new record |
| (any) | non-zero | skip — warn logged, no row, no event |

**On fire:**

1. `db.SkillReflexInsert(key, newMeta)` — append to time series.
2. `db.SystemMessageInsert` with kind `skill_reflex_fired` — body is `{"skill":"<name>","name":"<reflex>","meta":"<stdout>"}`.

The script owns all "what's new" logic. The system is storage + trigger.

### Writing a check script

- **Drain stdin** — even if you don't use it, read it to avoid broken-pipe signals: `cat > /dev/null`.
- **Exit 0 with empty stdout** when there's nothing new.
- **Exit 0 with new content on stdout** when something changed. The content is opaque to the system — JSON, plain text, a counter, whatever the skill needs. It becomes the `meta` field in the timeline.
- **Exit non-zero** on errors. The system logs a warning and skips — no row is persisted, no event fires. The next tick retries.
- **Idempotent side effects** — the script may run more than once for the same logical event (e.g. if the system crashes between running the script and persisting the row). Design accordingly.

## Schedule-only contract

No subprocess. The system generates `meta = time.Now().UTC().Format(time.RFC3339)`. Since the timestamp always differs from the last record, schedule-only reflexes fire every time they're due.

The system message body omits `meta` (only `skill` and `name`): `{"skill":"<name>","name":"<reflex>"}`.

Use case: periodic wake-ups where the skill handles discovery internally (e.g. "check Drive every day at 11pm" — the skill's instructions tell nik what to do, no external script needed).

## What happens when a reflex fires

1. A `skill_reflex` row is inserted (time series, keyed by `skillName/reflexName`).
2. A `skill_reflex_fired` system message is inserted into the first privileged conversation.
3. On the next brain tick, the timeline shows:

```
[HH:MM:SS] system: [skill reflex fired]
           skill: google_workspace
           name:  check_gmail
           meta:  {"count":3,"ids":["abc","def","ghi"]}
           MANDATORY: load this skill with load_skill. Follow its guidance, own the outcome.
```

The `meta` line appears only for command-based reflexes. The MANDATORY directive ensures nik loads the skill and acts on the data.

**Files:** `internal/timeline/system.go` (`renderSkillReflexFired`)

## Core reflexes

The brain runs [core reflexes](BRAIN.md#reflexes-constructing-the-timeline) every tick. These are hard-coded in Go and registered in `main.go`. They handle internal state — no SKILL.md declaration, no scripts.

| Reflex | Interval | What it does |
|--------|----------|--------------|
| `FireDueAlarms` | every tick | creates alarm occurrences, emits `alarm_fired` |
| `StaleAlarmReflex` | 30 min | detects recurring alarms with no next fire time, emits `alarm_stale` |
| `CheckStale` | every tick | flags tasks with no activity, inserts stale task reports |
| `SkillChangeReflex` | 5 min | detects skill file add/remove/change, emits `skill_added`/`skill_removed`/`skill_changed` |
| `SkillCheckReflex` | 5min | runs skill-declared check commands (this doc), emits `skill_reflex_fired` |
| `CheckSessions` | 10 sec | reaps dead/stale shell sessions |

`SkillCheckReflex` is the bridge between core and skill reflexes — it's a core reflex that iterates all skill-declared reflexes and runs them.

## Workspace reflexes

Skill reflexes can be declared in either location (see [SKILLS.md §Built-in vs workspace](SKILLS.md#built-in-vs-workspace) for the full override semantics):

- **`skills/`** — built-in, git-tracked. Core skills that ship with nik. Adding a reflex here requires a code change and a deploy.
- **`workspace/skills/`** — authored at runtime by nik or the user, not git-tracked. Workspace skills override built-in skills by name.

Either location supports the full `reflex:` frontmatter. The system scans both directories on every `SkillCheckReflex` tick. This means nik can teach itself new reflexes by writing a workspace skill with a `reflex:` block — no restart required.
