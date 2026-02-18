package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/queries"
)

func (s *Service) Add(ctx context.Context, content string, metadata map[string]any, source, sourceID string) (*Memory, error) {
	vec, err := s.llm.Embed(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("embed content: %w", err)
	}

	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	embedding, err := serializeEmbedding(vec)
	if err != nil {
		return nil, fmt.Errorf("serialize embedding: %w", err)
	}

	id := db.NewID()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var srcPtr, srcIDPtr *string
	if source != "" {
		srcPtr = &source
	}
	if sourceID != "" {
		srcIDPtr = &sourceID
	}

	_, err = tx.ExecContext(ctx, queries.MemoryInsert, id, content, string(metaJSON), srcPtr, srcIDPtr)
	if err != nil {
		return nil, fmt.Errorf("insert memory: %w", err)
	}

	_, err = tx.ExecContext(ctx, queries.MemoryVecInsert, id, embedding)
	if err != nil {
		return nil, fmt.Errorf("insert vec_memory: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("commit memory insert: %w", err)
	}

	return &Memory{
		ID:       id,
		Content:  content,
		Metadata: metadata,
		Source:   source,
		SourceID: sourceID,
	}, nil
}

func (s *Service) Search(ctx context.Context, query string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 5
	}

	vec, err := s.llm.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	embedding, err := serializeEmbedding(vec)
	if err != nil {
		return nil, fmt.Errorf("serialize embedding: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, queries.MemorySearch, embedding, limit)
	if err != nil {
		return nil, fmt.Errorf("search memories: %w", err)
	}
	defer rows.Close()

	return scanMemories(rows, true)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, queries.MemoryVecDelete, id)
	if err != nil {
		return fmt.Errorf("delete vec_memory %s: %w", id, err)
	}

	_, err = tx.ExecContext(ctx, queries.MemoryDelete, id)
	if err != nil {
		return fmt.Errorf("delete memory %s: %w", id, err)
	}

	return tx.Commit()
}

func (s *Service) List(ctx context.Context, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.QueryContext(ctx, queries.MemoryList, limit)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	defer rows.Close()

	return scanMemories(rows, false)
}

func scanMemories(rows *sql.Rows, withScore bool) ([]Memory, error) {
	var memories []Memory

	for rows.Next() {
		var m Memory
		var metaStr sql.NullString
		var source sql.NullString
		var sourceID sql.NullString
		var distance float64

		var err error
		if withScore {
			err = rows.Scan(&m.ID, &m.Content, &metaStr, &source, &sourceID, &m.CreatedAt, &distance)
			m.Score = 1 - distance
		} else {
			err = rows.Scan(&m.ID, &m.Content, &metaStr, &source, &sourceID, &m.CreatedAt)
		}
		if err != nil {
			return nil, fmt.Errorf("scan memory: %w", err)
		}

		m.Source = source.String
		m.SourceID = sourceID.String

		if metaStr.Valid && metaStr.String != "" {
			_ = json.Unmarshal([]byte(metaStr.String), &m.Metadata)
		}

		memories = append(memories, m)
	}

	return memories, rows.Err()
}

func serializeEmbedding(vec []float64) ([]byte, error) {
	f32 := make([]float32, len(vec))
	for i, v := range vec {
		f32[i] = float32(v)
	}
	return sqlite_vec.SerializeFloat32(f32)
}
