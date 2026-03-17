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

func TestSkillEventRemovedHasNullHash(t *testing.T) {
	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()
	ctx := context.Background()

	_, err = SkillEventInsert(ctx, conn, SkillEventInsertParams{
		Name: "backup",
		Kind: "removed",
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	latest, err := SkillEventLatestPerName(ctx, conn)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}

	if len(latest) != 1 {
		t.Fatalf("expected 1 event, got %d", len(latest))
	}
	if latest[0].ContentHash.Valid {
		t.Errorf("expected null content_hash for removed event, got %q", latest[0].ContentHash.String)
	}
}

func TestSkillUpsertAndList(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	first, err := SkillUpsert(ctx, conn, SkillUpsertParams{
		Name:        "journal",
		Status:      "active",
		ContentHash: "content-hash-1",
		InstallHash: "install-hash-1",
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if first.ID == "" {
		t.Fatal("expected first skill id to be set")
	}

	second, err := SkillUpsert(ctx, conn, SkillUpsertParams{
		Name:        "journal",
		Status:      "removed",
		ContentHash: "content-hash-2",
		InstallHash: "install-hash-2",
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected skill id %s to be preserved, got %s", first.ID, second.ID)
	}

	skills, err := SkillList(ctx, conn)
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill row, got %d", len(skills))
	}
	if skills[0].ID != first.ID {
		t.Fatalf("expected listed skill id %s, got %s", first.ID, skills[0].ID)
	}
	if skills[0].Name != "journal" {
		t.Fatalf("expected journal, got %q", skills[0].Name)
	}
	if skills[0].Status != "removed" {
		t.Fatalf("expected removed status, got %q", skills[0].Status)
	}
	if !skills[0].ContentHash.Valid || skills[0].ContentHash.String != "content-hash-2" {
		t.Fatalf("expected updated content hash, got %+v", skills[0].ContentHash)
	}
	if !skills[0].InstallHash.Valid || skills[0].InstallHash.String != "install-hash-2" {
		t.Fatalf("expected updated install hash, got %+v", skills[0].InstallHash)
	}
}
