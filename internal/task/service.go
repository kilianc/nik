package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/brain"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
)

type Service struct {
	conn *sql.DB
}

func NewService(conn *sql.DB) *Service {
	return &Service{conn: conn}
}

type CreateParams struct {
	CrewMemberID   string
	RetryForTaskID string
	RetryNumber    int
	Goal           string
	Plan           string
	Thinking       string
	Meta           map[string]string
}

func (s *Service) Create(ctx context.Context, p CreateParams) (db.Task, error) {
	if p.Meta == nil {
		p.Meta = map[string]string{}
	}

	metaJSON, err := json.Marshal(p.Meta)
	if err != nil {
		return db.Task{}, fmt.Errorf("marshal task meta: %w", err)
	}

	ip := db.TaskInsertParams{
		ID:             id.V7(),
		MetaJSON:       string(metaJSON),
		CrewMemberID:   p.CrewMemberID,
		RetryForTaskID: p.RetryForTaskID,
		RetryNumber:    p.RetryNumber,
		Goal:           p.Goal,
		Plan:           p.Plan,
		Thinking:       p.Thinking,
		Status:         "pending",
		CreatedAt:      time.Now().UTC(),
	}

	err = db.TaskInsert(ctx, s.conn, ip)
	if err != nil {
		return db.Task{}, err
	}

	return db.Task{
		ID:             ip.ID,
		Meta:           p.Meta,
		CrewMemberID:   p.CrewMemberID,
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

func (s *Service) Start(ctx context.Context, taskID, activationID string) error {
	return db.TaskStart(ctx, s.conn, taskID, activationID)
}

func (s *Service) UpdateStatus(ctx context.Context, taskID, status string) error {
	return db.TaskUpdateStatus(ctx, s.conn, taskID, status)
}

func (s *Service) InsertReport(ctx context.Context, taskID, content string) error {
	return db.TaskReportInsert(ctx, s.conn, db.TaskReportInsertParams{
		ID:        id.V7(),
		TaskID:    taskID,
		Kind:      "report",
		Content:   content,
		CreatedAt: time.Now().UTC(),
	})
}

func (s *Service) TasksNeedingAttention(ctx context.Context) ([]db.TaskAttention, error) {
	return db.TasksNeedingAttention(ctx, s.conn)
}

func (s *Service) MarkRead(ctx context.Context, reportID string) error {
	return db.TaskReportMarkRead(ctx, s.conn, reportID)
}

func (s *Service) List(ctx context.Context, includeRecent bool) ([]db.TaskListRow, error) {
	recency := "-0 seconds"
	if includeRecent {
		recency = "-1 hour"
	}

	return db.TaskList(ctx, s.conn, recency)
}

func (s *Service) ActiveTasks(ctx context.Context, conversationID string) ([]db.ActiveTask, error) {
	return db.TaskActiveTasks(ctx, s.conn, conversationID)
}

func (s *Service) ActiveRetries(ctx context.Context, rootID string) ([]db.ActiveTask, error) {
	return db.TaskActiveRetries(ctx, s.conn, rootID)
}

func (s *Service) RetryChain(ctx context.Context, rootID string) ([]db.RetryChainEntry, error) {
	return db.TaskRetryChain(ctx, s.conn, rootID)
}

func (s *Service) AllActiveTasks(ctx context.Context) ([]db.ActiveTask, error) {
	return db.TaskAllActive(ctx, s.conn)
}

// MarkSeen stamps checked_at so the datasource won't resurface this stale alert
// until the task goes idle again.
func (s *Service) MarkSeen(ctx context.Context, taskID string) error {
	return db.TaskMarkSeen(ctx, s.conn, taskID)
}

func (s *Service) StaleTasks(ctx context.Context, staleThreshold, maxRunning time.Duration) ([]db.Task, error) {
	now := time.Now().UTC()
	staleCutoff := now.Add(-staleThreshold)
	maxCutoff := now.Add(-maxRunning)

	return db.TaskStaleTasks(ctx, s.conn, staleCutoff, maxCutoff)
}

func (s *Service) ActiveTasksForConversation(ctx context.Context, conversationID string) ([]brain.DebugTaskInfo, error) {
	active, err := s.ActiveTasks(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	out := make([]brain.DebugTaskInfo, len(active))
	for i, t := range active {
		out[i] = brain.DebugTaskInfo{
			ID:        t.ID,
			Goal:      t.Goal,
			Status:    t.Status,
			CreatedAt: t.CreatedAt,
		}
	}

	return out, nil
}

// RecentToolCalls returns the latest tool calls for a task activation,
// used by task_status to show Nik what the worker has been doing.
func (s *Service) RecentToolCalls(ctx context.Context, activationID string) ([]db.ToolCallInfo, error) {
	return db.TaskRecentToolCalls(ctx, s.conn, activationID)
}
