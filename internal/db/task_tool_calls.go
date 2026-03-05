package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kciuffolo/nik/internal/queries"
)

func TaskRecentToolCalls(ctx context.Context, db *sql.DB, activationID string) ([]ToolCallInfo, error) {
	rows, err := db.QueryContext(ctx, queries.TaskToolCalls, activationID)
	if err != nil {
		return nil, fmt.Errorf("query tool calls for activation %s: %w", activationID, err)
	}
	defer rows.Close()

	var calls []ToolCallInfo
	for rows.Next() {
		var tc ToolCallInfo
		var errFlag int

		err = rows.Scan(&tc.Name, &tc.DurationMS, &errFlag, &tc.At)
		if err != nil {
			return nil, fmt.Errorf("scan tool call: %w", err)
		}

		tc.Error = errFlag != 0
		calls = append(calls, tc)
	}

	return calls, rows.Err()
}
