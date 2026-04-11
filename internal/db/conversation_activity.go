package db

import (
	"context"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

func ConversationActivityPush(ctx context.Context, db DBTX, convID, state string) error {
	_, err := db.ExecContext(ctx, queries.ConversationActivityPush, convID, state)
	if err != nil {
		return fmt.Errorf("push activity %s on %s: %w", state, convID, err)
	}
	return nil
}

func ConversationActivityPop(ctx context.Context, db DBTX, convID, state string) error {
	_, err := db.ExecContext(ctx, queries.ConversationActivityPop, convID, state)
	if err != nil {
		return fmt.Errorf("pop activity %s on %s: %w", state, convID, err)
	}
	return nil
}

func ConversationActivityReset(ctx context.Context, db DBTX) error {
	_, err := db.ExecContext(ctx, queries.ConversationActivityReset)
	if err != nil {
		return fmt.Errorf("reset activity: %w", err)
	}
	return nil
}
