package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/kciuffolo/nik/internal/id"
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

	memID := id.V7()

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

	_, err = tx.ExecContext(ctx, queries.MemoryInsert, memID, content, string(metaJSON), srcPtr, srcIDPtr)
	if err != nil {
		return nil, fmt.Errorf("insert memory: %w", err)
	}

	_, err = tx.ExecContext(ctx, queries.MemoryVecInsert, memID, embedding)
	if err != nil {
		return nil, fmt.Errorf("insert vec_memory: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("commit memory insert: %w", err)
	}

	return &Memory{
		ID:       memID,
		Content:  content,
		Metadata: metadata,
		Source:   source,
		SourceID: sourceID,
	}, nil
}

func (s *Service) Search(ctx context.Context, query string, limit int) ([]Memory, error) {
	return s.SearchMulti(ctx, []string{query}, limit)
}

func (s *Service) SearchMulti(ctx context.Context, queries_ []string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 5
	}

	if len(queries_) == 0 {
		return nil, nil
	}

	vecs, err := s.llm.EmbedBatch(ctx, queries_)
	if err != nil {
		return nil, fmt.Errorf("embed queries: %w", err)
	}

	seen := map[string]int{}
	var merged []Memory

	for _, vec := range vecs {
		embedding, err := serializeEmbedding(vec)
		if err != nil {
			return nil, fmt.Errorf("serialize embedding: %w", err)
		}

		// over-fetch from the vector index because the deleted_at filter
		// runs after KNN retrieval and may discard some of the k results
		rows, err := s.db.QueryContext(ctx, queries.MemorySearch, embedding, limit*2)
		if err != nil {
			return nil, fmt.Errorf("search memories: %w", err)
		}

		batch, err := scanMemories(rows, true)
		rows.Close()
		if err != nil {
			return nil, err
		}

		for _, m := range batch {
			if idx, ok := seen[m.ID]; ok {
				if m.Score > merged[idx].Score {
					merged[idx].Score = m.Score
				}
				continue
			}

			seen[m.ID] = len(merged)
			merged = append(merged, m)
		}
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})

	return merged, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, queries.MemoryDelete, id)
	if err != nil {
		return fmt.Errorf("soft-delete memory %s: %w", id, err)
	}

	return nil
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
