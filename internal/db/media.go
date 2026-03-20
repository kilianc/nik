package db

import (
	"context"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type InsertMediaParams struct {
	ID             string
	MimeType       *string
	SizeBytes      *int64
	LocalPath      *string
	DescribeText   *string
	TranscriptText *string
	DescribedAt    *time.Time
	TranscribedAt  *time.Time
}

func InsertMedia(ctx context.Context, db DBTX, p InsertMediaParams) error {
	if p.ID == "" {
		return fmt.Errorf("empty media id")
	}

	_, err := db.ExecContext(
		ctx,
		queries.MediaInsert,
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
		return fmt.Errorf("insert media %s: %w", p.ID, err)
	}

	return nil
}

type MediaUpdateParams struct {
	ID             string
	DescribeText   *string
	DescribedAt    *time.Time
	TranscriptText *string
	TranscribedAt  *time.Time
}

func MediaUpdate(ctx context.Context, db DBTX, p MediaUpdateParams) (int64, error) {
	result, err := db.ExecContext(ctx, queries.MediaUpdate,
		p.ID,
		p.DescribeText,
		p.DescribedAt,
		p.TranscriptText,
		p.TranscribedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("update media %s: %w", p.ID, err)
	}

	return result.RowsAffected()
}

type MediaResolution struct {
	MediaID           string
	MessageID         string
	ConversationID    string
	Platform          string
	ExternalMessageID string
}

func MediaResolveByPath(ctx context.Context, db DBTX, localPath string) (MediaResolution, error) {
	var r MediaResolution

	err := db.QueryRowContext(ctx, queries.MediaResolveByPath, localPath).Scan(
		&r.MediaID,
		&r.MessageID,
		&r.ConversationID,
		&r.Platform,
		&r.ExternalMessageID,
	)
	if err != nil {
		return MediaResolution{}, fmt.Errorf("resolve media by path %s: %w", localPath, err)
	}

	return r, nil
}

func UpsertMessageMedia(ctx context.Context, db DBTX, messageID, mediaID string) error {
	_, err := db.ExecContext(ctx, queries.MessageMediaUpsert, id.V7(), messageID, mediaID)
	if err != nil {
		return fmt.Errorf("upsert message media %s/%s: %w", messageID, mediaID, err)
	}

	return nil
}
