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
	"github.com/kciuffolo/nik/internal/messaging"
)

type Service struct {
	conn *sql.DB
}

func NewService(conn *sql.DB) *Service {
	return &Service{conn: conn}
}

func (s *Service) Create(ctx context.Context, crewMemberID, goal, plan, thinking string, meta map[string]string) (db.Task, error) {
	if meta == nil {
		meta = map[string]string{}
	}

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return db.Task{}, fmt.Errorf("marshal task meta: %w", err)
	}

	p := db.TaskInsertParams{
		ID:           id.V7(),
		MetaJSON:     string(metaJSON),
		CrewMemberID: crewMemberID,
		Goal:         goal,
		Plan:         plan,
		Thinking:     thinking,
		Status:       "pending",
		CreatedAt:    time.Now().UTC(),
	}

	err = db.TaskInsert(ctx, s.conn, p)
	if err != nil {
		return db.Task{}, err
	}

	return db.Task{
		ID:           p.ID,
		Meta:         meta,
		CrewMemberID: crewMemberID,
		Goal:         goal,
		Plan:         plan,
		Thinking:     thinking,
		Status:       p.Status,
		CreatedAt:    p.CreatedAt,
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

func (s *Service) InsertReport(ctx context.Context, taskID, kind, content string) error {
	return db.TaskReportInsert(ctx, s.conn, db.TaskReportInsertParams{
		ID:        id.V7(),
		TaskID:    taskID,
		Kind:      kind,
		Content:   content,
		CreatedAt: time.Now().UTC(),
	})
}

func (s *Service) UnreadReports(ctx context.Context) ([]db.TaskReport, error) {
	return db.TaskReportUnread(ctx, s.conn)
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

func (s *Service) ActiveConversationTasks(ctx context.Context, conversationID string) ([]messaging.TaskInfo, error) {
	active, err := s.ActiveTasks(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	out := make([]messaging.TaskInfo, len(active))
	for i, t := range active {
		out[i] = messaging.TaskInfo{
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
