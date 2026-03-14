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
		CreatedAt:      time.Now().UTC(),
	}

	err := db.TaskInsert(ctx, s.conn, ip)
	if err != nil {
		return db.Task{}, err
	}

	return db.Task{
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
	}, nil
}

func (s *Service) Get(ctx context.Context, taskID string) (db.Task, error) {
	return db.TaskGet(ctx, s.conn, taskID)
}

func (s *Service) ResolveTaskID(ctx context.Context, shortID string) (string, error) {
	return db.ResolveShortID(ctx, s.conn, "task", shortID)
}

func (s *Service) Start(ctx context.Context, taskID, activationID string) error {
	return db.TaskStart(ctx, s.conn, taskID, activationID)
}

func (s *Service) UpdateStatus(ctx context.Context, taskID, status string) error {
	return db.TaskUpdateStatus(ctx, s.conn, taskID, status)
}

func (s *Service) InsertReport(ctx context.Context, taskID, status, content string) error {
	if status == "" {
		status = "running"
	}
	return db.TaskReportInsert(ctx, s.conn, db.TaskReportInsertParams{
		ID:        id.V7(),
		TaskID:    taskID,
		Status:    status,
		Content:   content,
		CreatedAt: time.Now().UTC(),
	})
}

func (s *Service) LastReportStatus(ctx context.Context, taskID string) (string, error) {
	return db.TaskReportLastStatus(ctx, s.conn, taskID)
}

func (s *Service) ListReports(ctx context.Context, conversationID string, since time.Time) ([]db.TaskReport, error) {
	return db.TaskReportList(ctx, s.conn, conversationID, since)
}

func (s *Service) ListSpawned(ctx context.Context, conversationID string, since time.Time) ([]db.TaskSpawned, error) {
	return db.TaskListSpawned(ctx, s.conn, conversationID, since)
}

func (s *Service) ListCancelled(ctx context.Context, conversationID string, since time.Time) ([]db.TaskCancelled, error) {
	return db.TaskListCancelled(ctx, s.conn, conversationID, since)
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

const staleThreshold = 2 * time.Minute

func (s *Service) CheckStale(ctx context.Context) {
	staleCutoff := time.Now().UTC().Add(-staleThreshold)

	staleIDs, err := db.TaskStaleIDs(ctx, s.conn, staleCutoff)
	if err != nil {
		slog.Warn("check stale tasks", "pkg", "task", "error", err)
		return
	}

	for _, taskID := range staleIDs {
		msg := fmt.Sprintf("No activity for %s. Task may be stuck.", staleThreshold)
		err = s.InsertReport(ctx, taskID, "running", msg)
		if err != nil {
			slog.Warn("insert stale report", "pkg", "task", "task_id", taskID, "error", err)
		}
	}
}

// RecentToolCalls returns the latest tool calls for a task activation,
// used by task_status to show Nik what the worker has been doing.
func (s *Service) RecentToolCalls(ctx context.Context, activationID string) ([]db.ToolCallInfo, error) {
	return db.TaskRecentToolCalls(ctx, s.conn, activationID)
}

func (s *Service) AllToolCalls(ctx context.Context, activationID string) ([]db.ToolCallInfo, error) {
	return db.TaskAllToolCalls(ctx, s.conn, activationID)
}

func (s *Service) ReportsByTask(ctx context.Context, taskID string) ([]db.TaskReportRow, error) {
	return db.TaskReportsByTask(ctx, s.conn, taskID)
}

func (s *Service) InsertAssessment(ctx context.Context, p db.TaskAssessmentInsertParams) error {
	return db.TaskAssessmentInsert(ctx, s.conn, p)
}
