package db

import (
	"context"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
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

func UpdateMediaDescription(ctx context.Context, db DBTX, mediaID, description string, describedAt time.Time) (int64, error) {
	result, err := db.ExecContext(ctx, queries.MediaUpdateDescription, description, describedAt, mediaID)
	if err != nil {
		return 0, fmt.Errorf("update media description %s: %w", mediaID, err)
	}

	return result.RowsAffected()
}

func UpdateMediaTranscript(ctx context.Context, db DBTX, mediaID, transcript string, transcribedAt time.Time) (int64, error) {
	result, err := db.ExecContext(ctx, queries.MediaUpdateTranscript, transcript, transcribedAt, mediaID)
	if err != nil {
		return 0, fmt.Errorf("update media transcript %s: %w", mediaID, err)
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
