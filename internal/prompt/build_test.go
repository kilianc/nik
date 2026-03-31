package prompt

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
)

func TestBuildBrainDataToolSplit(t *testing.T) {
	cfg := &config.Config{Timezone: "UTC"}
	workerNames := []string{"shell", "task_report"}
	toolDefs := []llm.ToolDef{
		{Name: "shell"},
		{Name: "task_report"},
		{Name: "send_message"},
		{Name: "done"},
	}

	data := BuildBrainData(cfg, workerNames, toolDefs)

	if len(data.WorkerTools) != 2 {
		t.Fatalf("expected 2 worker tools, got %d", len(data.WorkerTools))
	}
	if len(data.NikTools) != 2 {
		t.Fatalf("expected 2 nik-only tools, got %d", len(data.NikTools))
	}
}

func TestRenderTaskPromptProfile(t *testing.T) {
	home := t.TempDir()
	soulDir := filepath.Join(home, "soul")
	os.MkdirAll(soulDir, 0o755)
	os.WriteFile(filepath.Join(soulDir, "latest.md"), []byte("# Current soul\n\nI value honesty."), 0o644)

	pr := NewRenderer(&config.Config{Home: home})
	tools := []llm.ToolDef{{Name: "shell", Description: "run commands"}}

	t.Run("profile nik", func(t *testing.T) {
		cfg := &config.Config{
			Home:        home,
			Timezone:    "UTC",
			Task:        config.TaskConfig{Profile: "nik"},
			BannedWords: []string{"forbidden", "blocked"},
		}

		got := pr.Task(BuildTaskData(cfg, db.Task{Goal: "test", Plan: "do stuff"}, tools))

		for _, want := range []string{
			"You are NIK",
			"forbidden",
			"I value honesty",
			"spawned to complete a task",
			"Your manager",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("expected %q in output, got:\n%s", want, got)
			}
		}
	})

	t.Run("profile default", func(t *testing.T) {
		cfg := &config.Config{
			Home:     home,
			Timezone: "UTC",
			Task:     config.TaskConfig{},
		}

		got := pr.Task(BuildTaskData(cfg, db.Task{Goal: "test", Plan: "do stuff"}, tools))

		if strings.Contains(got, "You are NIK") {
			t.Fatalf("expected no identity when profile empty, got:\n%s", got)
		}
		for _, want := range []string{
			"spawned to complete a task",
			"Your manager",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("expected %q in output, got:\n%s", want, got)
			}
		}
	})
}

func TestScanTokenTraps(t *testing.T) {
	t.Run("empty home", func(t *testing.T) {
		tt := ScanTokenTraps(t.TempDir())
		if len(tt.Dated) != 0 || len(tt.Large) != 0 || len(tt.Dense) != 0 {
			t.Fatalf("expected empty token traps for empty home")
		}
	})

	home := t.TempDir()

	for _, day := range []string{"12", "13", "14", "15", "16"} {
		dir := filepath.Join(home, "journal", "2026", "03", day)
		os.MkdirAll(dir, 0o755)
		if err := os.WriteFile(filepath.Join(dir, "2026-03-"+day+".md"), []byte("note"), 0o644); err != nil {
			t.Fatalf("write dated journal file: %v", err)
		}
	}
	os.Symlink("2026/03/16/2026-03-16.md", filepath.Join(home, "journal", "latest.md"))

	os.MkdirAll(filepath.Join(home, "soul"), 0o755)
	if err := os.WriteFile(filepath.Join(home, "soul", "latest.md"), []byte("note"), 0o644); err != nil {
		t.Fatalf("write soul latest: %v", err)
	}
	for _, day := range []string{"15", "16"} {
		dir := filepath.Join(home, "soul", "2026", "03", day)
		os.MkdirAll(dir, 0o755)
		if err := os.WriteFile(filepath.Join(dir, "2026-03-"+day+".md"), []byte("note"), 0o644); err != nil {
			t.Fatalf("write dated soul file: %v", err)
		}
	}

	cacheFile := filepath.Join(home, "skills", "google_workspace", "cache", "big.json")
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
		t.Fatalf("mkdir large file dir: %v", err)
	}
	if err := os.WriteFile(cacheFile, bytes.Repeat([]byte("x"), 100*1024), 0o644); err != nil {
		t.Fatalf("write large cache file: %v", err)
	}

	skillDoc := filepath.Join(home, "skills", "google_workspace", "SKILL.md")
	if err := os.WriteFile(skillDoc, bytes.Repeat([]byte("x"), 1024), 0o644); err != nil {
		t.Fatalf("write skill doc: %v", err)
	}

	gitFile := filepath.Join(home, ".git", "big.bin")
	if err := os.MkdirAll(filepath.Dir(gitFile), 0o755); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}
	if err := os.WriteFile(gitFile, bytes.Repeat([]byte("x"), 200*1024), 0o644); err != nil {
		t.Fatalf("write skipped git file: %v", err)
	}

	denseDir := filepath.Join(home, "skills", "dense_cache")
	if err := os.MkdirAll(denseDir, 0o755); err != nil {
		t.Fatalf("mkdir dense dir: %v", err)
	}
	for i := 0; i < 35; i++ {
		file := filepath.Join(denseDir, "entry-"+fmt.Sprintf("%02d", i)+".txt")
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatalf("write dense file: %v", err)
		}
	}

	tt := ScanTokenTraps(home)

	if len(tt.Dated) < 2 {
		t.Fatalf("expected at least 2 dated dirs (journal, soul), got %d", len(tt.Dated))
	}

	var datedPaths []string
	for _, d := range tt.Dated {
		datedPaths = append(datedPaths, d.Path)
	}

	if !slices.Contains(datedPaths, "journal/") {
		t.Fatalf("expected journal/ in dated dirs, got %v", datedPaths)
	}
	if !slices.Contains(datedPaths, "soul/") {
		t.Fatalf("expected soul/ in dated dirs, got %v", datedPaths)
	}

	if len(tt.Large) == 0 {
		t.Fatal("expected at least one large file")
	}
	foundBig := false
	for _, f := range tt.Large {
		if strings.Contains(f.Path, "big.json") {
			foundBig = true
		}
		if strings.Contains(f.Path, ".git") {
			t.Fatalf("expected .git to be skipped, found %q", f.Path)
		}
	}
	if !foundBig {
		t.Fatal("expected big.json in large files")
	}

	if len(tt.Dense) == 0 {
		t.Fatal("expected at least one dense dir")
	}
	foundDense := false
	for _, d := range tt.Dense {
		if strings.Contains(d.Path, "dense_cache") {
			foundDense = true
		}
	}
	if !foundDense {
		t.Fatal("expected dense_cache in dense dirs")
	}
}

func TestLoadSkills(t *testing.T) {
	t.Run("no dirs returns empty", func(t *testing.T) {
		sd := LoadSkills(nil)
		if len(sd.Preloaded) != 0 || len(sd.Available) != 0 {
			t.Fatalf("expected empty skill data, got %d preloaded, %d available", len(sd.Preloaded), len(sd.Available))
		}
	})
}
