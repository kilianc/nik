package journal

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
)

func TestBuildDayContextEmptyDB(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	now := time.Now()
	dayStart := now.Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)
	lines := buildDayContext(ctx, conn, nil, "", dayStart, dayEnd)

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "No conversations today") {
		t.Fatal("expected 'No conversations today' in empty context")
	}
}

func TestBuildDayContextIncludesMemories(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	memID := id.V7()
	_, err = conn.ExecContext(ctx, "INSERT INTO memory (id, content) VALUES (?1, ?2)", memID, "nik likes dogs")
	if err != nil {
		t.Fatalf("insert memory: %v", err)
	}

	now := time.Now()
	dayStart := now.Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)
	lines := buildDayContext(ctx, conn, nil, "", dayStart, dayEnd)

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "nik likes dogs") {
		t.Fatal("expected memory content in day context")
	}
	if !strings.Contains(joined, "Memories formed today") {
		t.Fatal("expected memories section header")
	}
}

func TestGitChangelogSectionEmpty(t *testing.T) {
	lines := gitChangelogSection("", time.Now(), time.Now())
	if lines != nil {
		t.Fatal("expected nil for empty home")
	}
}

func TestGitChangelogSectionWithCommits(t *testing.T) {
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}

	run("git", "init")
	run("git", "checkout", "-b", "main")

	err := os.WriteFile(dir+"/hello.txt", []byte("hello"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	run("git", "add", ".")
	run("git", "commit", "-m", "add hello")

	now := time.Now()
	dayStart := now.Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)

	lines := gitChangelogSection(dir, dayStart, dayEnd)

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Git changelog") {
		t.Fatal("expected 'Git changelog' header")
	}
	if !strings.Contains(joined, "add hello") {
		t.Fatal("expected commit message in changelog")
	}
}

func TestGitChangelogSectionNoRepo(t *testing.T) {
	dir := t.TempDir()

	now := time.Now()
	dayStart := now.Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)

	lines := gitChangelogSection(dir, dayStart, dayEnd)
	if lines != nil {
		t.Fatal("expected nil for non-git directory")
	}
}
