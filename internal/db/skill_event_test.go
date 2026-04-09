package db

import (
	"context"
	"testing"
	"time"
)

func TestSkillEventInsertAndLatestPerName(t *testing.T) {
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()
	ctx := context.Background()

	_, err = SkillEventInsert(ctx, conn, SkillEventInsertParams{
		Name:        "journal",
		Kind:        "added",
		ContentHash: "abc123",
	})
	if err != nil {
		t.Fatalf("insert journal added: %v", err)
	}

	_, err = SkillEventInsert(ctx, conn, SkillEventInsertParams{
		Name:        "backup",
		Kind:        "added",
		ContentHash: "def456",
	})
	if err != nil {
		t.Fatalf("insert backup added: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	_, err = SkillEventInsert(ctx, conn, SkillEventInsertParams{
		Name:        "journal",
		Kind:        "changed",
		ContentHash: "abc789",
	})
	if err != nil {
		t.Fatalf("insert journal changed: %v", err)
	}

	latest, err := SkillEventLatestPerName(ctx, conn)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}

	if len(latest) != 2 {
		t.Fatalf("expected 2 latest events, got %d", len(latest))
	}

	byName := map[string]SkillEvent{}
	for _, e := range latest {
		byName[e.Name] = e
	}

	if byName["journal"].Kind != "changed" {
		t.Errorf("expected journal latest to be 'changed', got %q", byName["journal"].Kind)
	}
	if byName["journal"].ContentHash.String != "abc789" {
		t.Errorf("expected journal hash 'abc789', got %q", byName["journal"].ContentHash.String)
	}
	if byName["backup"].Kind != "added" {
		t.Errorf("expected backup latest to be 'added', got %q", byName["backup"].Kind)
	}

	t.Run("removed has null hash", func(t *testing.T) {
		_, err := SkillEventInsert(ctx, conn, SkillEventInsertParams{
			Name: "removed_skill",
			Kind: "removed",
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}

		latest, err = SkillEventLatestPerName(ctx, conn)
		if err != nil {
			t.Fatalf("latest: %v", err)
		}

		byName = map[string]SkillEvent{}
		for _, e := range latest {
			byName[e.Name] = e
		}

		if byName["removed_skill"].ContentHash.Valid {
			t.Errorf("expected null content_hash for removed event, got %q", byName["removed_skill"].ContentHash.String)
		}
	})
}

func TestSkillEventListSince(t *testing.T) {
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()
	ctx := context.Background()

	before := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)

	_, err = SkillEventInsert(ctx, conn, SkillEventInsertParams{
		Name:        "journal",
		Kind:        "added",
		ContentHash: "abc",
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	events, err := SkillEventList(ctx, conn, before)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Name != "journal" {
		t.Errorf("expected name 'journal', got %q", events[0].Name)
	}
}
