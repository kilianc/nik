---
name: consolidate-tests
description: >-
  Run a test-consolidation pass across Go test files. Use when the user asks to
  clean up, consolidate, deduplicate, or reduce test boilerplate in *_test.go
  files.
---

# Test Consolidation

Systematic pass over `*_test.go` files to reduce redundancy while preserving coverage. Rules live in AGENTS.md under **Testing** -- read that section first, this skill enforces those rules as a workflow.

## Mode

- **full** (default) -- analyze all `Test*` functions in every `*_test.go` file in scope.
- **new** -- only analyze `Test*` functions added or modified in the current git diff (`git diff HEAD -- '*_test.go'`). Existing unchanged tests are read for context (fold/merge targets) but never modified.

## Workflow

1. **Scope**: in full mode, list all `*_test.go` files in the target scope. In new mode, run `git diff HEAD -- '*_test.go'` and identify only added/modified `Test*` functions (lines starting with `+func Test`).
2. **Classify** each `Test*` function into one of four categories:
   - **weak** -- constructor-stores-field or single trivial assertion (delete)
   - **foldable** -- single assertion that extends a neighbor test (fold into neighbor)
   - **mergeable** -- 3+ tests with identical shape, differing only in data (table-driven merge)
   - **standalone** -- unique setup or assertion logic (keep as-is)
3. **Prioritize**: tackle high-impact files first (most mergeable clusters).
4. **Apply**: edit one file at a time, then run `go test ./<pkg>/...`. When merging into table-driven loops, use `t.Errorf` (not `t.Fatalf`) so remaining cases still run.
5. **Verify**: after all changes, run `make lint && make test`.

In **new** mode, never delete, fold, or restructure tests that predate the current diff. They are read-only context.
