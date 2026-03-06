package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

type TaskInsertParams struct {
	ID             string
	MetaJSON       string
	CrewMemberID   string
	RetryForTaskID string
	RetryNumber    int
	Goal           string
	Plan           string
	Thinking       string
	Status         string
	CreatedAt      time.Time
}

func TaskGet(ctx context.Context, db *sql.DB, taskID string) (Task, error) {
	row := db.QueryRowContext(ctx, queries.TaskGet, taskID)

	t, err := scanTask(row)
	if err != nil {
		return Task{}, fmt.Errorf("get task %s: %w", taskID, err)
	}

	return t, nil
}

func TaskInsert(ctx context.Context, db *sql.DB, p TaskInsertParams) error {
	var memberID any
	if p.CrewMemberID != "" {
		memberID = p.CrewMemberID
	}

	var retryForTaskID any
	if p.RetryForTaskID != "" {
		retryForTaskID = p.RetryForTaskID
	}

	_, err := db.ExecContext(ctx, queries.TaskInsert,
		p.ID,
		p.MetaJSON,
		memberID,
		retryForTaskID,
		p.RetryNumber,
		p.Goal,
		p.Plan,
		p.Thinking,
		p.Status,
		p.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert task %s: %w", p.ID, err)
	}

	return nil
}

func TaskStart(ctx context.Context, db *sql.DB, taskID, activationID string) error {
	_, err := db.ExecContext(ctx, queries.TaskStart, taskID, activationID)
	if err != nil {
		return fmt.Errorf("start task %s: %w", taskID, err)
	}

	return nil
}

func TaskUpdateStatus(ctx context.Context, db *sql.DB, taskID, status string) error {
	_, err := db.ExecContext(ctx, queries.TaskUpdateStatus, taskID, status)
	if err != nil {
		return fmt.Errorf("update task status %s: %w", taskID, err)
	}

	return nil
}

func TaskActiveTasks(ctx context.Context, db *sql.DB, conversationID string) ([]ActiveTask, error) {
	rows, err := db.QueryContext(ctx, queries.TaskActive, conversationID)
	if err != nil {
		return nil, fmt.Errorf("query active tasks: %w", err)
	}
	defer rows.Close()

	var tasks []ActiveTask
	for rows.Next() {
		t, scanErr := scanActiveTask(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan active task: %w", scanErr)
		}

		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

func scanActiveTask(sc scanner) (ActiveTask, error) {
	var t ActiveTask
	var convID sql.NullString

	err := sc.Scan(
		&t.ID,
		&t.Goal,
		&t.Status,
		&convID,
		&t.RetryNumber,
		&t.CreatedAt,
	)

	t.ConversationID = convID.String
	return t, err
}

// TaskMarkSeen stamps checked_at so the datasource won't resurface this stale alert
// until the task goes idle again.
func TaskMarkSeen(ctx context.Context, db *sql.DB, taskID string) error {
	_, err := db.ExecContext(ctx, queries.TaskMarkSeen, taskID)
	if err != nil {
		return fmt.Errorf("mark task seen %s: %w", taskID, err)
	}

	return nil
}

func TaskStaleTasks(ctx context.Context, db *sql.DB, staleCutoff, maxCutoff time.Time) ([]Task, error) {
	rows, err := db.QueryContext(ctx, queries.TaskStale, staleCutoff, maxCutoff)
	if err != nil {
		return nil, fmt.Errorf("query stale tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("scan stale task: %w", err)
		}

		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

func TaskList(ctx context.Context, db *sql.DB, recency string) ([]TaskListRow, error) {
	rows, err := db.QueryContext(ctx, queries.TaskList, recency)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []TaskListRow
	for rows.Next() {
		var t TaskListRow

		err = rows.Scan(
			&t.ID,
			&t.Goal,
			&t.Status,
			&t.ConversationID,
			&t.CreatedAt,
			&t.StartedAt,
			&t.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan task list row: %w", err)
		}

		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

func scanTask(sc scanner) (Task, error) {
	var t Task
	var metaJSON string
	var activationID, crewMemberID, retryForTaskID sql.NullString

	err := sc.Scan(
		&t.ID,
		&metaJSON,
		&activationID,
		&crewMemberID,
		&retryForTaskID,
		&t.RetryNumber,
		&t.Goal,
		&t.Plan,
		&t.Thinking,
		&t.Status,
		&t.CreatedAt,
		&t.StartedAt,
		&t.CompletedAt,
	)
	if err != nil {
		return Task{}, err
	}

	t.Meta = UnmarshalMeta(metaJSON)
	t.ActivationID = activationID.String
	t.CrewMemberID = crewMemberID.String
	t.RetryForTaskID = retryForTaskID.String
	return t, nil
}

func UnmarshalMeta(raw string) map[string]string {
	m := map[string]string{}
	if raw != "" {
		json.Unmarshal([]byte(raw), &m)
	}
	return m
}
