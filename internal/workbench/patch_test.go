package workbench

import (
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/db"
)

func TestParseDiff(t *testing.T) {
	diff := strings.Join([]string{
		"--- a/instructions",
		"+++ b/instructions",
		"@@ -2,3 +2,4 @@",
		" line2",
		"-line3",
		"+line3a",
		"+line3b",
		" line4",
		"",
		"--- a/messages/0/content",
		"+++ b/messages/0/content",
		"@@ -1,1 +1,1 @@",
		"-old input",
		"+new input",
	}, "\n")

	patches, err := ParseDiff(diff)
	if err != nil {
		t.Fatalf("parse diff: %v", err)
	}

	if len(patches) != 2 {
		t.Fatalf("expected 2 file patches, got %d", len(patches))
	}

	if patches[0].Path != "instructions" {
		t.Fatalf("expected path %q, got %q", "instructions", patches[0].Path)
	}
	if patches[1].Path != "messages/0/content" {
		t.Fatalf("expected path %q, got %q", "messages/0/content", patches[1].Path)
	}

	if len(patches[0].Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(patches[0].Hunks))
	}
	if patches[0].Hunks[0].OldStart != 2 {
		t.Fatalf("expected old start 2, got %d", patches[0].Hunks[0].OldStart)
	}
}

func TestApplyPatches(t *testing.T) {
	t.Run("patch instructions", func(t *testing.T) {
		run := db.ExperimentVariantRun{
			Instructions: "line1\nline2\nline3\nline4\nline5",
			Patches: strings.Join([]string{
				"--- a/instructions",
				"+++ b/instructions",
				"@@ -2,3 +2,4 @@",
				" line2",
				"-line3",
				"+line3a",
				"+line3b",
				" line4",
			}, "\n"),
		}

		err := ApplyPatches(&run)
		if err != nil {
			t.Fatalf("apply patches: %v", err)
		}

		want := "line1\nline2\nline3a\nline3b\nline4\nline5"
		if run.Instructions != want {
			t.Fatalf("expected:\n%s\n\ngot:\n%s", want, run.Instructions)
		}
	})

	t.Run("patch messages content", func(t *testing.T) {
		run := db.ExperimentVariantRun{
			Messages: `[{"role":"user","content":"line1\nline2\nline3"},{"role":"assistant","content":"reply"}]`,
			Patches: strings.Join([]string{
				"--- a/messages/0/content",
				"+++ b/messages/0/content",
				"@@ -2,1 +2,1 @@",
				"-line2",
				"+replaced",
			}, "\n"),
		}

		err := ApplyPatches(&run)
		if err != nil {
			t.Fatalf("apply patches: %v", err)
		}

		if !strings.Contains(run.Messages, "replaced") {
			t.Fatalf("expected patched content, got: %s", run.Messages)
		}
		if strings.Contains(run.Messages, `"line2"`) {
			t.Fatalf("old text still present: %s", run.Messages)
		}
	})

	t.Run("patch messages name field", func(t *testing.T) {
		run := db.ExperimentVariantRun{
			Messages: `[{"role":"tool_call","content":"{}","name":"old_tool","call_id":"c1"}]`,
			Patches: strings.Join([]string{
				"--- a/messages/0/name",
				"+++ b/messages/0/name",
				"@@ -1,1 +1,1 @@",
				"-old_tool",
				"+new_tool",
			}, "\n"),
		}

		err := ApplyPatches(&run)
		if err != nil {
			t.Fatalf("apply patches: %v", err)
		}

		if !strings.Contains(run.Messages, "new_tool") {
			t.Fatalf("expected patched name, got: %s", run.Messages)
		}
	})

	t.Run("patch tools field", func(t *testing.T) {
		run := db.ExperimentVariantRun{
			ToolSchemas: `[{"Name":"message_noop","Description":"Acknowledge intentional silence."}]`,
			Patches: strings.Join([]string{
				"--- a/tools/message_noop/Name",
				"+++ b/tools/message_noop/Name",
				"@@ -1,1 +1,1 @@",
				"-message_noop",
				"+done",
			}, "\n"),
		}

		err := ApplyPatches(&run)
		if err != nil {
			t.Fatalf("apply patches: %v", err)
		}

		if !strings.Contains(run.ToolSchemas, `"done"`) {
			t.Fatalf("expected tool renamed, got: %s", run.ToolSchemas)
		}
		if strings.Contains(run.ToolSchemas, "message_noop") {
			t.Fatalf("old name still present: %s", run.ToolSchemas)
		}
	})

	t.Run("multi-surface patch", func(t *testing.T) {
		run := db.ExperimentVariantRun{
			Instructions: "line1\nline2",
			Messages:     `[{"role":"user","content":"input1\ninput2"}]`,
			Patches: strings.Join([]string{
				"--- a/instructions",
				"+++ b/instructions",
				"@@ -1,1 +1,1 @@",
				"-line1",
				"+modified1",
				"",
				"--- a/messages/0/content",
				"+++ b/messages/0/content",
				"@@ -2,1 +2,1 @@",
				"-input2",
				"+modified2",
			}, "\n"),
		}

		err := ApplyPatches(&run)
		if err != nil {
			t.Fatalf("apply patches: %v", err)
		}

		if run.Instructions != "modified1\nline2" {
			t.Fatalf("instructions: %q", run.Instructions)
		}
		if !strings.Contains(run.Messages, "modified2") {
			t.Fatalf("messages: %s", run.Messages)
		}
	})

	t.Run("context mismatch", func(t *testing.T) {
		run := db.ExperimentVariantRun{
			Instructions: "line1\nline2\nline3",
			Patches: strings.Join([]string{
				"--- a/instructions",
				"+++ b/instructions",
				"@@ -1,2 +1,2 @@",
				" line1",
				"-wrong",
				"+new",
			}, "\n"),
		}

		err := ApplyPatches(&run)
		if err == nil {
			t.Fatal("expected error for context mismatch")
		}
	})

	t.Run("unknown surface", func(t *testing.T) {
		run := db.ExperimentVariantRun{
			Patches: strings.Join([]string{
				"--- a/nonexistent",
				"+++ b/nonexistent",
				"@@ -1,1 +1,1 @@",
				"-old",
				"+new",
			}, "\n"),
		}

		err := ApplyPatches(&run)
		if err == nil {
			t.Fatal("expected error for unknown surface")
		}
	})

	t.Run("messages index out of range", func(t *testing.T) {
		run := db.ExperimentVariantRun{
			Messages: `[{"role":"user","content":"only one"}]`,
			Patches: strings.Join([]string{
				"--- a/messages/5/content",
				"+++ b/messages/5/content",
				"@@ -1,1 +1,1 @@",
				"-old",
				"+new",
			}, "\n"),
		}

		err := ApplyPatches(&run)
		if err == nil {
			t.Fatal("expected error for out of range message index")
		}
	})

	t.Run("empty patches", func(t *testing.T) {
		run := db.ExperimentVariantRun{
			Instructions: "original",
			Patches:      "",
		}

		err := ApplyPatches(&run)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if run.Instructions != "original" {
			t.Fatalf("expected unchanged, got %q", run.Instructions)
		}
	})
}
