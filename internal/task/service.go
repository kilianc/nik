package task

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
)

type Service struct {
	conn *sql.DB
}

func NewService(conn *sql.DB) *Service {
	return &Service{conn: conn}
}

type createParams struct {
	ConversationID string
	ContactID      string
	RetryForTaskID string
	RetryNumber    int
	Goal           string
	Plan           string
	Thinking       string
}

func (s *Service) Create(ctx context.Context, p createParams) (db.Task, error) {
	if p.ConversationID == "" {
		return db.Task{}, fmt.Errorf("empty conversation_id")
	}

	now := time.Now().UTC()

	ip := db.TaskInsertParams{
		ID:             id.V7(),
		ConversationID: p.ConversationID,
		ContactID:      p.ContactID,
		RetryForTaskID: p.RetryForTaskID,
		RetryNumber:    p.RetryNumber,
		Goal:           p.Goal,
		Plan:           p.Plan,
		Thinking:       p.Thinking,
		Status:         "pending",
		CreatedAt:      now,
	}

	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return db.Task{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	err = db.TaskInsert(ctx, tx, ip)
	if err != nil {
		return db.Task{}, err
	}

	t := db.Task{
		ID:             ip.ID,
		ConversationID: p.ConversationID,
		ContactID:      p.ContactID,
		RetryForTaskID: p.RetryForTaskID,
		RetryNumber:    p.RetryNumber,
		Goal:           p.Goal,
		Plan:           p.Plan,
		Thinking:       p.Thinking,
		Status:         ip.Status,
		CreatedAt:      ip.CreatedAt,
	}

	kind := "task_spawned"
	if p.RetryForTaskID != "" {
		kind = "task_retry"
	}

	_, err = db.SystemMessageInsert(ctx, tx, db.SystemMessageParams{
		ConversationID: p.ConversationID,
		Kind:           kind,
		Body:           t,
		SentAt:         now,
	})
	if err != nil {
		return db.Task{}, fmt.Errorf("insert system message: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return db.Task{}, fmt.Errorf("commit tx: %w", err)
	}

	return t, nil
}

func (s *Service) Get(ctx context.Context, taskID string) (db.Task, error) {
	return db.TaskGet(ctx, s.conn, taskID)
}

func (s *Service) ResolveTaskID(ctx context.Context, shortID string) (string, error) {
	return db.ResolveShortID(ctx, s.conn, "task", shortID)
}

func (s *Service) Start(ctx context.Context, taskID, activationID string) error {
	status := "running"
	return db.TaskUpdate(ctx, s.conn, db.TaskUpdateParams{
		ID:           taskID,
		Status:       &status,
		ActivationID: &activationID,
	})
}

func (s *Service) UpdateStatus(ctx context.Context, taskID, status string) error {
	return db.TaskUpdate(ctx, s.conn, db.TaskUpdateParams{
		ID:     taskID,
		Status: &status,
	})
}

func (s *Service) UpdatePlan(ctx context.Context, taskID, plan string) error {
	return db.TaskUpdate(ctx, s.conn, db.TaskUpdateParams{
		ID:   taskID,
		Plan: &plan,
	})
}

func (s *Service) Cancel(ctx context.Context, taskID, reason string) error {
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	status := "cancelled"
	err = db.TaskUpdate(ctx, tx, db.TaskUpdateParams{
		ID:                 taskID,
		Status:             &status,
		CancellationReason: &reason,
	})
	if err != nil {
		return err
	}

	t, err := db.TaskGet(ctx, tx, taskID)
	if err != nil {
		return fmt.Errorf("get task for cancel message: %w", err)
	}

	_, err = db.SystemMessageInsert(ctx, tx, db.SystemMessageParams{
		ConversationID: t.ConversationID,
		Kind:           "task_cancelled",
		Body:           t,
		SentAt:         time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("insert system message: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

func (s *Service) InsertReport(ctx context.Context, taskID, status, content string) error {
	if status == "" {
		status = "running"
	}

	now := time.Now().UTC()
	reportID := id.V7()

	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	t, err := db.TaskGet(ctx, tx, taskID)
	if err != nil {
		return fmt.Errorf("get task for report: %w", err)
	}

	err = db.TaskReportInsert(ctx, tx, db.TaskReportInsertParams{
		ID:        reportID,
		TaskID:    taskID,
		Status:    status,
		Content:   content,
		CreatedAt: now,
	})
	if err != nil {
		return err
	}

	err = db.TaskUpdate(ctx, tx, db.TaskUpdateParams{
		ID:           taskID,
		LastReportAt: &now,
	})
	if err != nil {
		return err
	}

	_, err = db.SystemMessageInsert(ctx, tx, db.SystemMessageParams{
		ConversationID: t.ConversationID,
		Kind:           "task_report",
		Body: db.TaskReport{
			ID:        reportID,
			TaskID:    taskID,
			Content:   content,
			CreatedAt: now,
			Goal:      t.Goal,
			Status:    status,
		},
		SentAt: now,
	})
	if err != nil {
		return fmt.Errorf("insert system message: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

func (s *Service) LastReportStatus(ctx context.Context, taskID string) (string, error) {
	return db.TaskReportLastStatus(ctx, s.conn, taskID)
}

func (s *Service) ListTasks(ctx context.Context, p db.TaskListParams) ([]db.TaskListRow, error) {
	return db.TaskList(ctx, s.conn, p)
}

func (s *Service) ActiveRetries(ctx context.Context, rootID string) ([]db.ActiveTask, error) {
	return db.TaskActiveRetries(ctx, s.conn, rootID)
}

func (s *Service) RetryChain(ctx context.Context, rootID string) ([]db.RetryChainEntry, error) {
	return db.TaskRetryChain(ctx, s.conn, rootID)
}

const StaleThreshold = 5 * time.Minute

func (s *Service) CheckStale(ctx context.Context) {
	staleCutoff := time.Now().UTC().Add(-StaleThreshold)

	staleIDs, err := db.TaskStaleIDs(ctx, s.conn, staleCutoff)
	if err != nil {
		slog.Warn("check stale tasks", "pkg", "task", "error", err)
		return
	}

	for _, taskID := range staleIDs {
		msg := fmt.Sprintf("No activity for %s. Task may be stuck.", StaleThreshold)
		err = s.InsertReport(ctx, taskID, "running", msg)
		if err != nil {
			slog.Warn("insert stale report", "pkg", "task", "task_id", taskID, "error", err)
		}
	}
}

func (s *Service) ReportsByTask(ctx context.Context, taskID string) ([]db.TaskReportRow, error) {
	return db.TaskReportList(ctx, s.conn, taskID)
}
