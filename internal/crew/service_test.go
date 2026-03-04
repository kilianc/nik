package crew

import (
	"context"
	"testing"

	"github.com/kciuffolo/nik/internal/db"
)

func testDB(t *testing.T) *Service {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return NewService(conn)
}

func TestHireAndGet(t *testing.T) {
	svc := testDB(t)
	ctx := context.Background()

	m, err := svc.Hire(ctx, "Byte", "systems engineer, good at shell commands and debugging")
	if err != nil {
		t.Fatalf("hire: %v", err)
	}

	if m.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if m.Name != "Byte" {
		t.Fatalf("expected name Byte, got %q", m.Name)
	}

	got, err := svc.Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.Name != "Byte" {
		t.Fatalf("expected name Byte, got %q", got.Name)
	}
	if got.Prompt != "systems engineer, good at shell commands and debugging" {
		t.Fatalf("expected prompt preserved, got %q", got.Prompt)
	}

	got, err = svc.Get(ctx, "Byte")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if got.ID != m.ID {
		t.Fatalf("expected id %s, got %s", m.ID, got.ID)
	}
}

func TestHireDuplicateName(t *testing.T) {
	svc := testDB(t)
	ctx := context.Background()

	_, err := svc.Hire(ctx, "Byte", "first prompt")
	if err != nil {
		t.Fatalf("hire: %v", err)
	}

	_, err = svc.Hire(ctx, "Byte", "second prompt")
	if err == nil {
		t.Fatal("expected error on duplicate name")
	}
}

func TestList(t *testing.T) {
	svc := testDB(t)
	ctx := context.Background()

	svc.Hire(ctx, "Byte", "engineer")
	svc.Hire(ctx, "Scout", "researcher")
	svc.Hire(ctx, "Pixel", "designer")

	members, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
	}

	if members[0].Name != "Byte" {
		t.Fatalf("expected first member Byte, got %q", members[0].Name)
	}
	if members[2].Name != "Pixel" {
		t.Fatalf("expected last member Pixel, got %q", members[2].Name)
	}
}

func TestGetNotFound(t *testing.T) {
	svc := testDB(t)
	ctx := context.Background()

	_, err := svc.Get(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent member")
	}
}
