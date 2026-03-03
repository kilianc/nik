package memory

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

const testDims = 1536

func fakeEmbedding(seed float64) []byte {
	f32 := make([]float32, testDims)
	for i := range f32 {
		f32[i] = float32(seed + float64(i)*0.0001)
	}

	b, _ := sqlite_vec.SerializeFloat32(f32)
	return b
}

func insertTestMemory(t *testing.T, ctx context.Context, svc *Service, content string, seed float64) string {
	return insertTestMemoryWithSource(t, ctx, svc, content, seed, "", "")
}

func insertTestMemoryWithSource(t *testing.T, ctx context.Context, svc *Service, content string, seed float64, source, sourceID string) string {
	t.Helper()

	memID := id.V7()

	var srcPtr, srcIDPtr *string
	if source != "" {
		srcPtr = &source
	}
	if sourceID != "" {
		srcIDPtr = &sourceID
	}

	_, err := svc.db.ExecContext(ctx, queries.MemoryInsert, memID, content, "{}", srcPtr, srcIDPtr)
	if err != nil {
		t.Fatalf("insert test memory: %v", err)
	}

	_, err = svc.db.ExecContext(ctx, queries.MemoryVecInsert, memID, fakeEmbedding(seed))
	if err != nil {
		t.Fatalf("insert test vec_memory: %v", err)
	}

	return memID
}

func TestListMemories(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	svc := &Service{db: conn}

	insertTestMemory(t, ctx, svc, "user likes coffee", 0.1)
	insertTestMemory(t, ctx, svc, "user birthday is march 15", 0.2)

	memories, err := svc.List(ctx, 10)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}

	if len(memories) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(memories))
	}
}

func TestDeleteMemorySoftDeletes(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	svc := &Service{db: conn}

	memID := insertTestMemory(t, ctx, svc, "temporary fact", 0.3)

	err = svc.Delete(ctx, memID)
	if err != nil {
		t.Fatalf("delete memory: %v", err)
	}

	memories, err := svc.List(ctx, 10)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}

	if len(memories) != 0 {
		t.Fatalf("expected 0 memories in list after soft delete, got %d", len(memories))
	}

	var deletedAt sql.NullString

	err = conn.QueryRowContext(ctx, "SELECT deleted_at FROM memory WHERE id = ?1", memID).Scan(&deletedAt)
	if err != nil {
		t.Fatalf("query deleted memory row: %v", err)
	}

	if !deletedAt.Valid {
		t.Fatal("expected deleted_at to be set, got NULL")
	}

	var vecCount int

	err = conn.QueryRowContext(ctx, "SELECT count(*) FROM vec_memory WHERE id = ?1", memID).Scan(&vecCount)
	if err != nil {
		t.Fatalf("query vec_memory after soft delete: %v", err)
	}

	if vecCount != 1 {
		t.Fatalf("expected vec_memory row preserved after soft delete, got %d", vecCount)
	}
}

func TestListRespectsLimit(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	svc := &Service{db: conn}

	for i := 0; i < 5; i++ {
		insertTestMemory(t, ctx, svc, fmt.Sprintf("memory %d", i), float64(i)*0.1)
	}

	memories, err := svc.List(ctx, 3)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}

	if len(memories) != 3 {
		t.Fatalf("expected 3 memories, got %d", len(memories))
	}
}

func TestListDefaultLimit(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	svc := &Service{db: conn}

	insertTestMemory(t, ctx, svc, "one memory", 0.5)

	memories, err := svc.List(ctx, 0)
	if err != nil {
		t.Fatalf("list with default limit: %v", err)
	}

	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}
}

func TestSearchExcludesDeleted(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	svc := &Service{db: conn}

	seed := 0.42
	insertTestMemory(t, ctx, svc, "alive memory", seed)
	deletedID := insertTestMemory(t, ctx, svc, "deleted memory", seed+0.001)

	err = svc.Delete(ctx, deletedID)
	if err != nil {
		t.Fatalf("delete memory: %v", err)
	}

	rows, err := conn.QueryContext(ctx, queries.MemorySearch, fakeEmbedding(seed), 10)
	if err != nil {
		t.Fatalf("search query: %v", err)
	}

	results, err := scanMemories(rows, true)
	rows.Close()
	if err != nil {
		t.Fatalf("scan search results: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result (deleted excluded), got %d", len(results))
	}

	if results[0].Content != "alive memory" {
		t.Fatalf("expected alive memory, got %q", results[0].Content)
	}
}

func TestSourceStoredAndReturned(t *testing.T) {
	ctx := context.Background()

	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	svc := &Service{db: conn}

	insertTestMemoryWithSource(t, ctx, svc, "fact from message", 0.6, "message", "msg-abc-123")
	insertTestMemoryWithSource(t, ctx, svc, "fact from briefing", 0.7, "briefing", "2026-02-27")
	insertTestMemory(t, ctx, svc, "fact without source", 0.8)

	memories, err := svc.List(ctx, 10)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}

	if len(memories) != 3 {
		t.Fatalf("expected 3 memories, got %d", len(memories))
	}

	byContent := map[string]Memory{}
	for _, m := range memories {
		byContent[m.Content] = m
	}

	msg := byContent["fact from message"]
	if msg.Source != "message" || msg.SourceID != "msg-abc-123" {
		t.Fatalf("expected source=message source_id=msg-abc-123, got %q %q", msg.Source, msg.SourceID)
	}

	briefing := byContent["fact from briefing"]
	if briefing.Source != "briefing" || briefing.SourceID != "2026-02-27" {
		t.Fatalf("expected source=briefing source_id=2026-02-27, got %q %q", briefing.Source, briefing.SourceID)
	}

	noSource := byContent["fact without source"]
	if noSource.Source != "" || noSource.SourceID != "" {
		t.Fatalf("expected empty source fields, got %q %q", noSource.Source, noSource.SourceID)
	}
}
