package db

import (
	"context"
	"testing"
)

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
