package db

import (
	"context"
	"testing"
	"time"
)

func TestSearchContactExactIdentifierWins(t *testing.T) {
	ctx := context.Background()

	conn, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	seed, err := UpsertContact(ctx, conn, UpsertContactParams{
		Platform:      "whatsapp",
		ExternalID:    "search@s.whatsapp.net",
		Name:          "Search Name",
		Phone:         "77777",
		LastMessageAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	results, err := SearchContact(ctx, conn, "search@s.whatsapp.net", 0.95, 5)
	if err != nil {
		t.Fatalf("search contact: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("expected at least one search result")
	}
	if results[0].ID != seed.ID {
		t.Fatalf("expected top result id %q, got %q", seed.ID, results[0].ID)
	}
	if results[0].Score < 1.0 {
		t.Fatalf("expected exact match score 1.0, got %f", results[0].Score)
	}
}
