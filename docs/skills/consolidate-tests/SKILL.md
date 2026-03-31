---
name: consolidate-tests
description: >-
  Run a test-consolidation pass across Go test files. Use when the user asks to
  clean up, consolidate, deduplicate, or reduce test boilerplate in *_test.go
  files.
---

# Test Consolidation
Systematic pass over `*_test.go` files to reduce redundancy while preserving
coverage. Every unique code path must still be exercised after changes.

## Mode

Pass a mode when invoking:

- **full** (default) — analyze all `Test*` functions in every `*_test.go` file
  in scope. Current behavior.
- **new** — only analyze `Test*` functions added or modified in the current git
  diff (`git diff HEAD -- '*_test.go'`). Existing unchanged tests are read for
  context (fold/merge targets) but never modified.

## Operations

### 1. Delete weak tests

Remove tests whose only assertion is that a constructor stores its arguments.

```go
// DELETE — asserts only that New stores the field
func TestNewServiceStoresDB(t *testing.T) {
    svc := New(nil)
    if svc.db != nil { t.Fatal("...") }
}
```

**Keep** if the constructor performs validation, allocation, or has observable
side-effects worth testing.

### 2. Fold single-assertion tests into neighbors

When a standalone test makes one assertion that logically belongs with an
existing test, add it as a subtest or trailing check.

```go
// BEFORE — two test functions
func TestCreateAndGet(t *testing.T) { /* ... */ }
func TestConversationIDRoundTrip(t *testing.T) { /* same setup, one extra assertion */ }

// AFTER — folded into existing test
func TestCreateAndGet(t *testing.T) {
    // ... existing assertions ...
    if got.ConversationID != want { t.Fatalf("...") }
}
```

### 3. Merge clusters into table-driven tests

When 3+ tests share identical setup/assertion structure and differ only in
inputs and expected outputs, consolidate into a table.

```go
func TestParseFrontmatter(t *testing.T) {
    tests := []struct {
        name  string
        input string
        want  Frontmatter
    }{
        {"basic", "---\nname: x\n---\n", Frontmatter{Name: "x"}},
        {"with preload", "---\npreload: true\n---\n", Frontmatter{Preload: true}},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := parseFrontmatter(tt.input)
            // assert got == tt.want
        })
    }
}
```

When tests share setup but differ in assertion logic (e.g. path-security
checks that inspect different error conditions), use `t.Run` subtests instead
of a data table.

## Workflow

1. **Scope**:
   - **full mode**: list all `*_test.go` files in the target scope.
   - **new mode**: run `git diff HEAD -- '*_test.go'` to find modified test
     files. Within each file, identify only **added or modified** `Test*`
     functions (diff lines starting with `+func Test`). These are the
     consolidation candidates. Read the full file for context (existing tests
     may be fold/merge targets) but never modify unchanged tests.
2. **Classify**: for each file, categorize every `Test*` function:
   - **weak** — constructor-stores-field, single trivial assertion
   - **foldable** — single assertion that extends a neighbor test
   - **mergeable** — 3+ tests with identical shape, differing only in data
   - **standalone** — unique setup or assertion logic, keep as-is
3. **Prioritize**: tackle high-impact files first (most mergeable clusters).
4. **Apply**: edit one file at a time, then run `go test ./<pkg>/...`.
5. **Verify**: after all changes, run `make lint && make test`.

## Decision criteria

| Signal | Action |
|--------|--------|
| Only asserts constructor stored a field | delete |
| One assertion, same setup as neighbor test | fold into neighbor |
| 3+ tests, identical structure, differ by inputs | table-driven merge |
| Tests share setup but have distinct assertion logic | `t.Run` subtests |
| Unique setup or covers a distinct code path | keep standalone |

## Constraints

- **Never remove a test that covers a unique code path.** If unsure, keep it.
- Run tests after each file's changes — never batch edits across files without
  verifying.
- Follow the repo's strict 1:1 test file naming (`foo.go` -> `foo_test.go`).
- Use `t.Run` subtests so individual cases remain independently runnable.
- Preserve the existing test package (`package foo` or `package foo_test`).
- **`new` mode**: never delete, fold, or restructure tests that predate the
  current diff. They are read-only context.
