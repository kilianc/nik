package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/queries"
)

type TaskInsertParams struct {
	ID             string
	ConversationID string
	ContactID      string
	RetryForTaskID string
	RetryNumber    int
	Goal           string
	Plan           string
	Thinking       string
	Status         string
	CreatedAt      time.Time
}

func TaskGet(ctx context.Context, db DBTX, taskID string) (Task, error) {
	row := db.QueryRowContext(ctx, queries.TaskGet, taskID)

	t, err := scanTask(row)
	if err != nil {
		return Task{}, fmt.Errorf("get task %s: %w", taskID, err)
	}

	return t, nil
}

func TaskInsert(ctx context.Context, db DBTX, p TaskInsertParams) error {
	var convID any
	if p.ConversationID != "" {
		convID = p.ConversationID
	}

	var contactID any
	if p.ContactID != "" {
		contactID = p.ContactID
	}

	var retryForTaskID any
	if p.RetryForTaskID != "" {
		retryForTaskID = p.RetryForTaskID
	}

	_, err := db.ExecContext(ctx, queries.TaskInsert,
		p.ID,
		convID,
		contactID,
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

type TaskUpdateParams struct {
	ID                 string
	Status             *string
	ActivationID       *string
	LastReportAt       *time.Time
	CancellationReason *string
	Plan               *string
}

func TaskUpdate(ctx context.Context, db DBTX, p TaskUpdateParams) error {
	_, err := db.ExecContext(ctx, queries.TaskUpdate,
		p.ID,
		p.Status,
		p.ActivationID,
		p.LastReportAt,
		p.CancellationReason,
		p.Plan,
	)
	if err != nil {
		return fmt.Errorf("update task %s: %w", p.ID, err)
	}

	return nil
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

func TaskStaleIDs(ctx context.Context, db *sql.DB, staleCutoff time.Time) ([]string, error) {
	rows, err := db.QueryContext(ctx, queries.TaskStale, staleCutoff)
	if err != nil {
		return nil, fmt.Errorf("query stale tasks: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		err = rows.Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("scan stale task id: %w", err)
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

type TaskListParams struct {
	ConversationID string
	IncludeRecent  bool
}

func TaskList(ctx context.Context, db *sql.DB, p TaskListParams) ([]TaskListRow, error) {
	recency := "-0 seconds"
	if p.IncludeRecent {
		recency = "-1 hour"
	}

	rows, err := db.QueryContext(ctx, queries.TaskList, p.ConversationID, p.IncludeRecent, recency)
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

func TaskActiveRetries(ctx context.Context, db *sql.DB, rootID string) ([]ActiveTask, error) {
	rows, err := db.QueryContext(ctx, queries.TaskActiveRetries, rootID)
	if err != nil {
		return nil, fmt.Errorf("query active retries for %s: %w", rootID, err)
	}
	defer rows.Close()

	var tasks []ActiveTask
	for rows.Next() {
		t, scanErr := scanActiveTask(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan active retry: %w", scanErr)
		}

		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

func scanTask(sc scanner) (Task, error) {
	var t Task
	var convID, contactID, activationID, retryForTaskID, cancellationReason sql.NullString

	err := sc.Scan(
		&t.ID,
		&convID,
		&contactID,
		&activationID,
		&retryForTaskID,
		&t.RetryNumber,
		&t.Goal,
		&t.Plan,
		&t.Thinking,
		&t.Status,
		&cancellationReason,
		&t.CreatedAt,
		&t.StartedAt,
		&t.CompletedAt,
		&t.LastReportAt,
	)
	if err != nil {
		return Task{}, err
	}

	t.ConversationID = convID.String
	t.ContactID = contactID.String
	t.ActivationID = activationID.String
	t.RetryForTaskID = retryForTaskID.String
	t.CancellationReason = cancellationReason.String
	return t, nil
}

type TaskReportInsertParams struct {
	ID        string
	TaskID    string
	Status    string
	Content   string
	CreatedAt time.Time
}

func TaskReportInsert(ctx context.Context, db DBTX, p TaskReportInsertParams) error {
	_, err := db.ExecContext(ctx, queries.TaskReportInsert,
		p.ID,
		p.TaskID,
		p.Status,
		p.Content,
		p.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert task report for %s: %w", p.TaskID, err)
	}

	return nil
}

func TaskReportLastStatus(ctx context.Context, db *sql.DB, taskID string) (string, error) {
	var status string
	err := db.QueryRowContext(ctx, queries.TaskReportLastStatus, taskID).Scan(&status)
	if err != nil {
		return "", fmt.Errorf("last report status for task %s: %w", taskID, err)
	}
	return status, nil
}

func TaskRetryChain(ctx context.Context, db *sql.DB, rootID string) ([]RetryChainEntry, error) {
	rows, err := db.QueryContext(ctx, queries.TaskRetryChain, rootID)
	if err != nil {
		return nil, fmt.Errorf("query retry chain for %s: %w", rootID, err)
	}
	defer rows.Close()

	idx := map[string]int{}
	var entries []RetryChainEntry

	for rows.Next() {
		var (
			id          string
			retryNumber int
			goal        string
			status      string
			content     string
			createdAt   sql.NullTime
		)

		err = rows.Scan(&id, &retryNumber, &goal, &status, &content, &createdAt)
		if err != nil {
			return nil, fmt.Errorf("scan retry chain row: %w", err)
		}

		i, exists := idx[id]
		if !exists {
			i = len(entries)
			idx[id] = i
			entries = append(entries, RetryChainEntry{
				ID:          id,
				RetryNumber: retryNumber,
				Goal:        goal,
				Status:      status,
			})
		}

		if content != "" {
			entries[i].Reports = append(entries[i].Reports, RetryChainReport{
				Content:   content,
				CreatedAt: createdAt,
			})
		}
	}

	return entries, rows.Err()
}
