package skills

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/kciuffolo/nik/internal/cron"
	"github.com/kciuffolo/nik/internal/db"
)

type Completer func(ctx context.Context, system, user string) (string, error)

const cronSystemPrompt = `Convert the following natural language schedule to a 5-field cron expression (minute hour day-of-month month day-of-week). Respond with ONLY the cron expression, nothing else. Examples:
- "every day at 11:30pm" -> "30 23 * * *"
- "every 15 minutes" -> "*/15 * * * *"
- "every day at 8am and 7pm" -> "0 8,19 * * *"
- "every day at 9am, 2pm, and 7pm" -> "0 9,14,19 * * *"
- "every Sunday at 7pm" -> "0 19 * * 0"
- "first of every month at midnight" -> "0 0 1 * *"`

func resolveCron(ctx context.Context, conn *sql.DB, every string, complete Completer) (*cron.Schedule, error) {
	cached, err := db.EveryToCronGet(ctx, conn, every)
	if err != nil {
		return nil, fmt.Errorf("cache lookup: %w", err)
	}

	if cached != "" {
		sched, err := cron.Parse(cached)
		if err != nil {
			return nil, fmt.Errorf("parse cached cron %q: %w", cached, err)
		}
		return sched, nil
	}

	raw, err := complete(ctx, cronSystemPrompt, every)
	if err != nil {
		return nil, fmt.Errorf("llm complete: %w", err)
	}

	cronExpr := strings.TrimSpace(raw)

	sched, err := cron.Parse(cronExpr)
	if err != nil {
		return nil, fmt.Errorf("parse llm cron %q for %q: %w", cronExpr, every, err)
	}

	err = db.EveryToCronInsert(ctx, conn, every, cronExpr)
	if err != nil {
		return nil, fmt.Errorf("cache insert: %w", err)
	}

	return sched, nil
}
