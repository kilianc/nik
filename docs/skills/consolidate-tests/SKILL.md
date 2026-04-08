---
name: consolidate-tests
description: >-
  Run a test-consolidation pass across Go test files. Use when the user asks to
  clean up, consolidate, deduplicate, or reduce test boilerplate in *_test.go
  files.
---

# Test Consolidation

Systematic pass over `*_test.go` files to maximize coverage with the least test infrastructure. Rules live in AGENTS.md under **Testing** -- read that section first, this skill enforces those rules as a workflow.

## Mode

- **full** (default) -- analyze all `Test*` functions in every `*_test.go` file in scope.
- **new** -- only analyze `Test*` functions added or modified in the current git diff (`git diff HEAD -- '*_test.go'`). Existing unchanged tests are read for context (fold/merge targets) but never modified.

## Classify

For each `Test*` function in scope, assign exactly one category:

| Category | Signal | Action |
|----------|--------|--------|
| **weak** | Only asserts a constructor stored a field, or a single trivial check with no meaningful code path | delete |
| **foldable** | One assertion that logically extends a neighbor test (same file) with matching setup | fold into the neighbor as a trailing check or subtest |
| **redundant** | Code path already exercised by another test -- check callers across packages, not just the same file | delete (the higher-level test is the coverage) |
| **mergeable** | 3+ tests with identical setup/assertion structure, differing only in inputs and expected outputs | table-driven merge with `t.Run` |
| **standalone** | Unique setup or assertion logic that no other test covers | keep as-is |

**Classification order matters.** Check redundant before standalone -- a test can look standalone in isolation but be fully covered by an integration test in a consuming package. For each candidate, ask: "would an existing test break if the code under test were wrong?" If yes, it's redundant.

## Workflow

1. **Scope**: in full mode, list all `*_test.go` files in the target scope. In new mode, run `git diff HEAD -- '*_test.go'` and identify only added/modified `Test*` functions (lines starting with `+func Test`).
2. **Classify** each `Test*` function using the table above. For the redundancy check, grep callers of the function under test across the repo to find higher-level tests that exercise the same path.
3. **Prioritize**: tackle high-impact files first (most mergeable clusters, then redundant, then foldable).
4. **Apply**: edit one file at a time, then run `go test ./<pkg>/...`. When merging into table-driven loops, use `t.Errorf` (not `t.Fatalf`) so remaining cases still run.
5. **Verify**: after all changes, run `make lint && make test`.

## Constraints

- Never remove a test that covers a unique code path. If unsure, keep it.
- Run tests after each file's changes -- never batch edits across files without verifying.
- Follow the repo's strict 1:1 test file naming (`foo.go` -> `foo_test.go`).
- Preserve the existing test package (`package foo` or `package foo_test`).
- In **new** mode, never delete, fold, or restructure tests that predate the current diff. They are read-only context.
