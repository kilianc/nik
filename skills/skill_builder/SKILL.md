---
name: skill_builder
summary: How to create, scaffold, or extend workspace skills.
tools: [shell]
---

# Skill Builder

Create workspace skills that extend nik's capabilities.

## What is a skill

A skill is a **capability domain** — a coherent set of related tools or
approaches, not a single narrow wrapper. Group things that belong together:
"web" covers link reading, headless browsing, and X/Twitter fetching;
"lights" covers all smart lighting backends. Don't create a new skill for
every individual tool.

## Anatomy

A skill is a folder under `workspace/skills/<name>/` with:

- `SKILL.md` (required) -- frontmatter + concise instructions
- Go companion programs (optional) -- standalone `main.go` files invoked via `shell`

Nothing else. No README, no docs, no extra files. The SKILL.md *is*
the documentation.

Single Go program (most skills):

```
workspace/skills/<name>/
  SKILL.md
  main.go
```

Multiple Go programs use subdirectories:

```
workspace/skills/<name>/
  SKILL.md
  fetcher.go
  reporter.go
```

## SKILL.md format

Frontmatter (YAML between `---` lines) must be the first thing in the file.

```yaml
---
name: my_skill
summary: One packed sentence telling the LLM what this skill lets it do.
tools: [shell, store_memory]
---
```

Fields:

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | lowercase, underscores, matches folder name |
| `summary` | yes | inline one-liner — what can I do if I load this? |
| `tools` | yes | tools the skill needs (empty `[]` if none) |
| `preload` | no | `true` to inject into every activation (default `false`) |
| `diagnostic_skip` | no | `true` to skip nightly diagnostic auth/install checks (default `false`) |

Body rules:

- short, declarative, structured with headers and tables
- no prose walls -- every sentence earns its place
- document commands, parameters, and output formats
- include safety notes and error handling

## Install section

If a skill has infrastructure requirements (alarms, credentials, binaries), add a `## Install` heading as the **last section** of the SKILL.md. The heading is the sole machine contract -- the skill change reflex detects it, hashes its content, and emits a MANDATORY install message when the skill is added or the section changes.

Content under `## Install` is freeform prose. Write concise, idempotent instructions -- nik checks current state before acting. No required sub-headings.

For preloaded skills (`preload: true`), the `## Install` section is stripped from prompt content to save tokens. The `load_skill` tool always returns the full SKILL.md including `## Install`.

## Go companion programs

When the skill needs custom logic beyond shell commands, add Go files.

Conventions:

- `package main`, one file named `main.go` per program
- subcommands via positional arg or `-cmd` flag
- all output is JSON to stdout
- errors: `{"error": "message"}` + `os.Exit(1)`
- no external dependencies beyond stdlib when possible
- run with `go run ./skills/<skill>/<program>/main.go <args>` (never compile)

Validate before finishing:

```
cd workspace/skills/<skill>/<program>
gofmt -w .
go vet .
go run main.go --help   # or a no-op subcommand to verify it compiles
```

## Credentials via vault

When a skill needs API keys or secrets, fetch them from the vault
at runtime. Never hardcode secrets.

```
SECRET=$(./vault/cli read <name>)
```

If the vault adapter doesn't exist yet, load the `vault` skill
(`load_skill vault`) and follow the setup instructions. If a secret
is missing, ask the user to add it to their password manager.

## Checklist

Before considering a skill done:

1. `SKILL.md` has valid frontmatter and loads via `load_skill`
2. Go programs (if any) pass `gofmt`, `go vet`, run with `go run`
3. No hardcoded secrets -- credentials come from the vault
4. Output is JSON, errors use `{"error": "..."}` pattern
5. Instructions are concise enough for an LLM to follow in one read
6. Skill covers a coherent capability domain, not a single narrow tool
