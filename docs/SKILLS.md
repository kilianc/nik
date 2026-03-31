# Skills

The [brain](BRAIN.md) is nik's thinking. Skills are nik's knowledge — learned capabilities that teach it what it can do and how. Without skills, nik can think and talk but not act on the world. Each skill is a manual for a capability domain: how to send email, how to manage alarms, how to browse the web. The `load_skill` tool is nik picking up a manual and reading it before acting.

Skills are not single-purpose wrappers. A skill is a **coherent capability domain** — a group of related tools and approaches. "web" covers link reading, headless browsing, and X/Twitter fetching. "google_workspace" covers Calendar, Gmail, Drive, Docs, and Sheets. Don't create a skill for every individual tool.

## Anatomy

A skill is a folder with a `SKILL.md` file:

```
skills/<name>/
  SKILL.md          # required — frontmatter + instructions
  main.go           # optional — Go companion program
  check_something.sh  # optional — reflex check script
```

The `SKILL.md` is the documentation, the instructions, and the machine contract all in one file. Nothing else is required. For the full authoring guide (Go companions, vault credentials, checklist), load the `skill_builder` skill.

## SKILL.md format

YAML frontmatter between `---` lines, followed by a markdown body.

```yaml
---
name: my_skill
summary: One packed sentence telling the LLM what this skill lets it do.
tools: [shell, store_memory]
preload: false
reflex:
  - name: check_something
    command: sh skills/my_skill/check_something.sh
    every: every 15 minutes
---

# My Skill

Instructions go here...

## Install

Infrastructure setup steps (last section, idempotent).
```

### Frontmatter fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | lowercase, underscores, matches folder name |
| `summary` | yes | one-liner — what can I do if I load this? |
| `tools` | yes | tools the skill needs (empty `[]` if none) |
| `preload` | no | `true` to inject into every activation (default `false`) |
| `diagnostic_skip` | no | `true` to skip nightly maintenance checks (default `false`) |
| `reflex` | no | periodic check commands — see [REFLEXES.md](REFLEXES.md) |

### Body rules

- Short, declarative, structured with headers and tables.
- No prose walls — every sentence earns its place.
- Document commands, parameters, and output formats.

### The `## Install` section

If a skill needs infrastructure (alarms, credentials, binaries), add `## Install` as the **last section**. This heading is a machine contract:

- `SkillChangeReflex` hashes the install section separately. When the hash changes, it emits a `skill_changed` event and the brain re-evaluates the install steps.
- For preloaded skills, `## Install` is stripped from prompt content to save tokens. The `load_skill` tool always returns the full file including install.
- Install instructions must be **idempotent** — nik checks current state before acting (no duplicate alarms, no re-creating existing credentials).

## Built-in vs workspace

Skills live in two directories:

| Location | Tracked | Who writes them | Override |
|----------|---------|-----------------|----------|
| `skills/` | git-tracked | developer | base layer |
| `workspace/skills/` | not tracked | nik or user at runtime | overrides built-in by name |

**Override semantics:** the system processes built-in first, workspace second. When both directories contain a skill with the same name, workspace wins:

- **Metadata** (`ListSkills`, `PreloadedSkills`): `walkSkillDirs` uses a `seen` map keyed by name. Workspace entries replace built-in entries.
- **Full file** (`load_skill`): iterates directories in reverse — workspace is checked first, first match wins.

This means a workspace skill can completely replace a built-in skill just by using the same `name:` in its frontmatter.

## Preload

Most skills are loaded on demand via `load_skill`. A skill with `preload: true` is injected into **every activation's prompt** automatically.

How it works:

1. `PreloadedSkills(dirs...)` collects skills with `preload: true`.
2. Body is stripped of frontmatter and `## Install` section.
3. Headings are shifted by 3 levels (so `#` becomes `####`).
4. Content appears under "Preloaded Skills" in the system prompt.
5. Preloaded skills are omitted from the "Available Skills" index to avoid duplication.

Currently preloaded: **messaging** and **vault**. Use sparingly — every preloaded skill costs tokens on every activation.

## How skills appear in the prompt

The `prompts/nik-03-skills.md` template renders two sections:

**Preloaded Skills** — full body of each `preload: true` skill, headings shifted. The LLM sees the complete instructions without needing to call `load_skill`.

**Available Skills** — one-line index of all other skills:

```
- **web**: Search the web, fetch URLs, and read tweets. (tools: shell)
- **alarm**: Schedule one-shot or recurring alarms... (tools: )
```

The LLM reads this index, decides which skill it needs, calls `load_skill` to get the full instructions, then acts. This two-tier approach keeps prompt size manageable while giving nik access to its full repertoire.

## Lifecycle

`SkillChangeReflex` runs every 2 seconds. It walks both skill directories, hashes each `SKILL.md` (full content hash + install section hash separately), and compares against the `skill` table in the database.

| What changed | Event | System message | Brain action |
|-------------|-------|----------------|-------------|
| New skill appears (or reappears after removal) | `skill_added` | `[skill added]` | loads skill, runs `## Install` if present |
| `## Install` section hash changed | `skill_changed` | `[skill changed]` | loads skill, re-evaluates install idempotently |
| Body changed, install unchanged | *(none)* | *(none)* | DB hashes updated silently |
| Skill file removed from disk | `skill_removed` | `[skill removed]` | asks user before cleaning up resources |

Events are stored in the `skill_event` table. System messages land in the timeline and trigger a brain activation — nik sees them and acts.

**DB wipe recovery:** if all `skill_event` rows are deleted, the reflex re-detects all skills as `added` and re-emits events. Install sections are idempotent, so re-running them is safe.

For periodic check commands declared via `reflex:` in frontmatter, see [REFLEXES.md](REFLEXES.md).

## Built-in skill inventory

| Name | Summary | Preload |
|------|---------|---------|
| alarm | schedule one-shot or recurring alarms for reminders, follow-ups, and routines | |
| briefing | morning news research session and topic management | |
| config | read and update nik's runtime config and conversation allow list (owner-only) | |
| contacts | address book — search and update contact profiles | |
| critic | post-task quality assessment for completed/failed tasks | |
| dream | nightly dream cycle — five passes of reflection and growth | |
| journal | end-of-day private journal | |
| maintenance | nightly system maintenance — prune old data, test auth, verify alarms | |
| media | describe or transcribe images, audio, stickers, and documents | |
| memory | extract durable facts from conversations into memories | |
| messaging | send messages, reactions, typing indicators, and presence | yes |
| search | read-only SQL against nik's SQLite database (owner-only) | |
| seeds | extract forward-looking opportunities from conversations | |
| shell | run commands in persistent tmux sessions, optionally in Docker (owner-only) | |
| skill_builder | how to create, scaffold, or extend workspace skills | |
| vault | secure credential access via a user-chosen secret store | yes |
| web | search the web, fetch URLs, and read tweets | |

Workspace skills are not listed here — they are runtime artifacts, not git-tracked, and vary per deployment.
