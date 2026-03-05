package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/kciuffolo/nik/internal/queries"
)

type MemoryInsertParams struct {
	ID        string
	Content   string
	MetaJSON  string
	Source    string
	SourceID  string
	Embedding []byte
}

func MemoryInsert(ctx context.Context, tx DBTX, p MemoryInsertParams) error {
	var srcPtr, srcIDPtr *string
	if p.Source != "" {
		srcPtr = &p.Source
	}
	if p.SourceID != "" {
		srcIDPtr = &p.SourceID
	}

	_, err := tx.ExecContext(ctx, queries.MemoryInsert, p.ID, p.Content, p.MetaJSON, srcPtr, srcIDPtr)
	if err != nil {
		return fmt.Errorf("insert memory: %w", err)
	}

	_, err = tx.ExecContext(ctx, queries.MemoryVecInsert, p.ID, p.Embedding)
	if err != nil {
		return fmt.Errorf("insert vec_memory: %w", err)
	}

	return nil
}

func MemorySearch(ctx context.Context, db *sql.DB, embedding []byte, limit int) ([]Memory, error) {
	rows, err := db.QueryContext(ctx, queries.MemorySearch, embedding, limit)
	if err != nil {
		return nil, fmt.Errorf("search memories: %w", err)
	}
	defer rows.Close()

	return scanMemories(rows, true)
}

func MemoryDelete(ctx context.Context, db *sql.DB, id string) error {
	_, err := db.ExecContext(ctx, queries.MemoryDelete, id)
	if err != nil {
		return fmt.Errorf("soft-delete memory %s: %w", id, err)
	}

	return nil
}

func MemoryList(ctx context.Context, db *sql.DB, limit int) ([]Memory, error) {
	rows, err := db.QueryContext(ctx, queries.MemoryList, limit)
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
		var deletedAt sql.NullString
		var distance float64

		var err error
		if withScore {
			err = rows.Scan(&m.ID, &m.Content, &metaStr, &source, &sourceID, &m.CreatedAt, &deletedAt, &distance)
			m.Score = 1 - distance
		} else {
			err = rows.Scan(&m.ID, &m.Content, &metaStr, &source, &sourceID, &m.CreatedAt)
		}
		if err != nil {
			return nil, fmt.Errorf("scan memory: %w", err)
		}

		if deletedAt.Valid {
			continue
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

func SerializeEmbedding(vec []float64) ([]byte, error) {
	f32 := make([]float32, len(vec))
	for i, v := range vec {
		f32[i] = float32(v)
	}
	return sqlite_vec.SerializeFloat32(f32)
}
