package db

import (
	"context"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

type UpsertMediaParams struct {
	ID             string
	MimeType       *string
	SizeBytes      *int64
	LocalPath      *string
	DescribeText   *string
	TranscriptText *string
	DescribedAt    *time.Time
	TranscribedAt  *time.Time
}

func UpsertMedia(ctx context.Context, db DBTX, p UpsertMediaParams) error {
	if p.ID == "" {
		return fmt.Errorf("empty media id")
	}

	_, err := db.ExecContext(
		ctx,
		queries.MediaUpsert,
		p.ID,
		p.MimeType,
		p.SizeBytes,
		p.LocalPath,
		p.DescribeText,
		p.TranscriptText,
		p.DescribedAt,
		p.TranscribedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert media %s: %w", p.ID, err)
	}

	return nil
}

func UpsertMessageMedia(ctx context.Context, db DBTX, messageID, mediaID string) error {
	_, err := db.ExecContext(ctx, queries.MessageMediaUpsert, messageID, mediaID)
	if err != nil {
		return fmt.Errorf("upsert message media %s/%s: %w", messageID, mediaID, err)
	}

	return nil
}

