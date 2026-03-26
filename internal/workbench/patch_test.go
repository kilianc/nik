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
		"--- a/input",
		"+++ b/input",
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
	if patches[1].Path != "input" {
		t.Fatalf("expected path %q, got %q", "input", patches[1].Path)
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

	t.Run("patch tool-result with field", func(t *testing.T) {
		run := db.ExperimentVariantRun{
			PriorToolCalls: []db.ToolCallListRow{
				{
					Round:  0,
					Name:   "load_skill",
					Output: `{"content":"# Alarm\n\nMUST end with update_alarm.\nDone."}`,
				},
			},
			Patches: strings.Join([]string{
				"--- a/tool-result/0/load_skill/content",
				"+++ b/tool-result/0/load_skill/content",
				"@@ -3,1 +3,1 @@",
				"-MUST end with update_alarm.",
				"+MUST include update_alarm.",
			}, "\n"),
		}

		err := ApplyPatches(&run)
		if err != nil {
			t.Fatalf("apply patches: %v", err)
		}

		if !strings.Contains(run.PriorToolCalls[0].Output, "MUST include update_alarm.") {
			t.Fatalf("expected patched output, got: %s", run.PriorToolCalls[0].Output)
		}
		if strings.Contains(run.PriorToolCalls[0].Output, "MUST end with") {
			t.Fatalf("old text still present: %s", run.PriorToolCalls[0].Output)
		}
	})

	t.Run("patch tool-result plain text", func(t *testing.T) {
		run := db.ExperimentVariantRun{
			PriorToolCalls: []db.ToolCallListRow{
				{
					Round:  1,
					Name:   "shell_exec",
					Output: "line1\nline2\nline3",
				},
			},
			Patches: strings.Join([]string{
				"--- a/tool-result/1/shell_exec",
				"+++ b/tool-result/1/shell_exec",
				"@@ -2,1 +2,1 @@",
				"-line2",
				"+replaced",
			}, "\n"),
		}

		err := ApplyPatches(&run)
		if err != nil {
			t.Fatalf("apply patches: %v", err)
		}

		want := "line1\nreplaced\nline3"
		if run.PriorToolCalls[0].Output != want {
			t.Fatalf("expected %q, got %q", want, run.PriorToolCalls[0].Output)
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

	t.Run("multi-file patch", func(t *testing.T) {
		run := db.ExperimentVariantRun{
			Instructions: "line1\nline2",
			UserInput:    "input1\ninput2",
			Patches: strings.Join([]string{
				"--- a/instructions",
				"+++ b/instructions",
				"@@ -1,1 +1,1 @@",
				"-line1",
				"+modified1",
				"",
				"--- a/input",
				"+++ b/input",
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
		if run.UserInput != "input1\nmodified2" {
			t.Fatalf("input: %q", run.UserInput)
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

	t.Run("bad tool-result round", func(t *testing.T) {
		run := db.ExperimentVariantRun{
			Patches: strings.Join([]string{
				"--- a/tool-result/99/missing_tool",
				"+++ b/tool-result/99/missing_tool",
				"@@ -1,1 +1,1 @@",
				"-old",
				"+new",
			}, "\n"),
		}

		err := ApplyPatches(&run)
		if err == nil {
			t.Fatal("expected error for missing tool call")
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
