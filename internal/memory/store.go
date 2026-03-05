package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
)

func (s *Service) Add(ctx context.Context, content string, metadata map[string]any, source, sourceID string) (*db.Memory, error) {
	vec, err := s.llm.Embed(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("embed content: %w", err)
	}

	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	embedding, err := db.SerializeEmbedding(vec)
	if err != nil {
		return nil, fmt.Errorf("serialize embedding: %w", err)
	}

	memID := id.V7()

	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	err = db.MemoryInsert(ctx, tx, db.MemoryInsertParams{
		ID:        memID,
		Content:   content,
		MetaJSON:  string(metaJSON),
		Source:    source,
		SourceID:  sourceID,
		Embedding: embedding,
	})
	if err != nil {
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("commit memory insert: %w", err)
	}

	return &db.Memory{
		ID:       memID,
		Content:  content,
		Metadata: metadata,
		Source:   source,
		SourceID: sourceID,
	}, nil
}

func (s *Service) Search(ctx context.Context, query string, limit int) ([]db.Memory, error) {
	return s.SearchMulti(ctx, []string{query}, limit)
}

func (s *Service) SearchMulti(ctx context.Context, queries []string, limit int) ([]db.Memory, error) {
	if limit <= 0 {
		limit = 5
	}

	if len(queries) == 0 {
		return nil, nil
	}

	vecs, err := s.llm.EmbedBatch(ctx, queries)
	if err != nil {
		return nil, fmt.Errorf("embed queries: %w", err)
	}

	seen := map[string]int{}
	var merged []db.Memory

	for _, vec := range vecs {
		embedding, err := db.SerializeEmbedding(vec)
		if err != nil {
			return nil, fmt.Errorf("serialize embedding: %w", err)
		}

		// over-fetch because deleted rows are filtered in Go after KNN
		// retrieval (sqlite-vec can't filter on joined table columns)
		batch, err := db.MemorySearch(ctx, s.conn, embedding, limit*2)
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

func (s *Service) Delete(ctx context.Context, memID string) error {
	return db.MemoryDelete(ctx, s.conn, memID)
}

func (s *Service) List(ctx context.Context, limit int) ([]db.Memory, error) {
	if limit <= 0 {
		limit = 20
	}

	return db.MemoryList(ctx, s.conn, limit)
}
