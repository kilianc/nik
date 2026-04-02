---
name: learning
summary: How you learn and get better. Manage howto/ guides — write, consult, refine, tend.
tools: [read_file, write_file, shell]
reflex:
  - name: tend
    every: every sunday at 9pm
---

# Learning

Your procedural memory. Skills are generic tool manuals — lean, generic. How-to guides are different: context-driven knowledge from experience. "When Calendar writes fail, here's the exact diagnostic." "Thread count is marketing — GSM is the real metric." Things you learned the hard way that would otherwise be stranded in a finished project folder.

## File layout

```
howto/
  calendar-write-guard.md   -- one file per topic
  google-docs-formatting.md
  archive/                  -- retired guides
  opslog.md                 -- append-only log of every tend pass
```

Use `read_file` and `write_file` for guide files. Use `shell` for `ls`, `mv`, `mkdir -p`, and `rg` to search guides by content or tag.

## Guide format

YAML frontmatter + freeform markdown body. The frontmatter tracks metadata. The body is whatever structure fits the topic — step-by-step procedure, decision framework, pitfalls list, worked example. You decide the shape.

```yaml
---
title: Calendar write failure diagnostic
tags: [google_workspace, calendar, debugging]
consulted: 0
last_consulted: ~
created: 2026-03-15
updated: 2026-03-15
---

When a Google Calendar write call fails, never ask the owner to reauth
on the first failure. Instead...

## Changelog

- 2026-03-15: initial guide after debugging write failures on kciuffolo account
- 2026-03-31: added tokeninfo step — scope snapshot was missing from evidence
```

| Field | Purpose |
|-------|---------|
| `title` | what this guide is about |
| `tags` | for discoverability when scanning |
| `consulted` | how many times you read this before starting work |
| `last_consulted` | when it was last consulted |
| `created` | when first written |
| `updated` | last revision date |

## Lifecycle

Guides are living documents. They get sharper with use, not stale with age.

### Write

After completing work where you discovered something non-obvious — a failure mode, a workaround, a constraint that contradicts docs — write a guide. The litmus test: "Would I get this wrong next time without this note?" If yes, write it. If no, don't.

A guide is a recipe — the exact tool calls, shell commands, vault keys, and endpoints a worker replays. Record what worked, what to skip, and any steps that collapse now that the answer is known. The next run should be shorter, faster, more effective than the current one.

Use a short descriptive slug for the filename: `calendar-write-guard.md`, `exa-research-methodology.md`. Set `consulted: 0`, `last_consulted: ~`, `created` and `updated` to today.

A guide earns its place by recording something you'd get wrong by default — an API quirk, a failure diagnostic, a workaround, a learned preference, an undocumented constraint. Response templates, decision-brief scaffolding, and "load context, analyze, write output" sequences are just task decomposition, which is what you already do.

### Consult

Before starting work, search `howto/` for relevant guides by tags and content. Match on the problem you're solving.

If a guide matches, follow it — it's the compressed path. Deviate only when the current situation is meaningfully different from what the guide describes. After reading, bump the frontmatter:

- Increment `consulted` by 1
- Set `last_consulted` to today

### Refine

After completing work that used a guide — was it accurate? Incomplete? Wrong? Rewrite the parts that need it while the experience is fresh. Update `updated` to today. Bad guides get rewritten, not patched with addenda.

Append to the `## Changelog` section at the bottom of the guide:

```
- YYYY-MM-DD: <what changed and why>
```

Every guide should have a `## Changelog` as its last section. Create it on the first refinement if it doesn't exist.

### Tend

A schedule-only reflex fires weekly. When the `skill_reflex_fired` event appears for `tend`, load this skill and review the collection:

1. Read all active guides (`ls howto/*.md`).
2. **Investigate usage.** For each guide that was consulted since the last tend (`last_consulted` is recent), check the outcome — did the task that used it succeed? Was the guide accurate? Did it save time or lead the worker astray? This is the real signal, not the counter alone.
3. **Review each guide:**
   - **Stale?** `consulted: 0` with an old `updated` date — is this still relevant, or should it be retired?
   - **Overlapping?** Two guides covering similar ground — merge into one tighter guide, retire the other.
   - **Wrong?** Investigation or recent experience shows the guide led to bad outcomes — rewrite it.
   - **Vague?** Too abstract to be useful — sharpen it with specifics or retire it.
4. Retire guides by moving them to `howto/archive/`:
   ```
   shell action: "run", command: "mkdir -p howto/archive && mv howto/<slug>.md howto/archive/<slug>.md"
   ```
5. **Log the pass.** Append a tend entry to `howto/opslog.md`:
   ```
   write_file action: "append", path: "howto/opslog.md", content: "\n## <YYYY-MM-DD> — Tend pass\n\n**Guides reviewed:** <count>\n**Consulted since last tend:** <slugs or none>\n**Useful:** <slug — what it helped with, or —>\n**Rewritten:** <slug — what changed, or —>\n**Retired:** <slug — why, or —>\n**New guides:** <slugs or —>\n**Notes:** <observations about the collection>\n"
   ```

The tend pass is silent — no messages to anyone. The goal is a tight, trustworthy collection where every guide earns its place.

## What belongs here vs elsewhere

- **Non-obvious technical knowledge from execution** → howto guide (API quirks, failure diagnostics, workarounds, undocumented constraints)
- **How to use a tool or system** → skill (generic, token-optimal)
- **Facts about people and relationships** → memory
- **Skill gaps or better workflows for existing skills** → update the workspace skill directly
- **How to handle a type of request** → nowhere — you already know how to think
