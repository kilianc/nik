package task

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/queries"
)

type Task struct {
	ID           string
	Source       string
	SourceID     string
	ActivationID string
	CrewMemberID string
	Goal         string
	Plan         string
	Thinking     string
	Status       string
	CreatedAt    time.Time
	StartedAt    sql.NullTime
	CompletedAt  sql.NullTime
}

type Report struct {
	ID         string
	TaskID     string
	Kind       string
	Content    string
	ReportedAt sql.NullTime
	CreatedAt  time.Time

	// joined from task
	Source   string
	SourceID string
	Goal     string
	Status   string
}

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Create(ctx context.Context, source, sourceID, crewMemberID, goal, plan, thinking string) (Task, error) {
	t := Task{
		ID:           id.V7(),
		Source:       source,
		SourceID:     sourceID,
		CrewMemberID: crewMemberID,
		Goal:         goal,
		Plan:         plan,
		Thinking:     thinking,
		Status:       "pending",
		CreatedAt:    time.Now().UTC(),
	}

	var memberID any
	if crewMemberID != "" {
		memberID = crewMemberID
	}

	_, err := s.db.ExecContext(ctx, queries.TaskInsert,
		t.ID,
		t.Source,
		t.SourceID,
		nil,
		memberID,
		t.Goal,
		t.Plan,
		t.Thinking,
		t.Status,
		t.CreatedAt,
		nil,
		nil,
	)
	if err != nil {
		return Task{}, fmt.Errorf("insert task %s: %w", t.ID, err)
	}

	return t, nil
}

func (s *Service) Get(ctx context.Context, taskID string) (Task, error) {
	row := s.db.QueryRowContext(ctx, queries.TaskGet, taskID)

	var t Task
	var activationID sql.NullString
	var crewMemberID sql.NullString

	err := row.Scan(
		&t.ID,
		&t.Source,
		&t.SourceID,
		&activationID,
		&crewMemberID,
		&t.Goal,
		&t.Plan,
		&t.Thinking,
		&t.Status,
		&t.CreatedAt,
		&t.StartedAt,
		&t.CompletedAt,
	)
	if err != nil {
		return Task{}, fmt.Errorf("get task %s: %w", taskID, err)
	}

	t.ActivationID = activationID.String
	t.CrewMemberID = crewMemberID.String

	return t, nil
}

func (s *Service) List(ctx context.Context, source, sourceID string) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, queries.TaskList, source, sourceID)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var activationID sql.NullString
		var crewMemberID sql.NullString

		err = rows.Scan(
			&t.ID,
			&t.Source,
			&t.SourceID,
			&activationID,
			&crewMemberID,
			&t.Goal,
			&t.Plan,
			&t.Thinking,
			&t.Status,
			&t.CreatedAt,
			&t.StartedAt,
			&t.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}

		t.ActivationID = activationID.String
		t.CrewMemberID = crewMemberID.String
		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

func (s *Service) SetActivationID(ctx context.Context, taskID, activationID string) error {
	_, err := s.db.ExecContext(ctx, queries.TaskSetActivationID, taskID, activationID)
	if err != nil {
		return fmt.Errorf("set activation id for task %s: %w", taskID, err)
	}

	return nil
}

func (s *Service) UpdateStatus(ctx context.Context, taskID, status string) error {
	_, err := s.db.ExecContext(ctx, queries.TaskUpdateStatus, taskID, status)
	if err != nil {
		return fmt.Errorf("update task status %s: %w", taskID, err)
	}

	return nil
}

func (s *Service) InsertReport(ctx context.Context, taskID, kind, content string) error {
	_, err := s.db.ExecContext(ctx, queries.TaskReportInsert,
		id.V7(),
		taskID,
		kind,
		content,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert task report for %s: %w", taskID, err)
	}

	return nil
}

func (s *Service) UnreportedReports(ctx context.Context) ([]Report, error) {
	rows, err := s.db.QueryContext(ctx, queries.TaskReportUnreported)
	if err != nil {
		return nil, fmt.Errorf("query unreported reports: %w", err)
	}
	defer rows.Close()

	var reports []Report
	for rows.Next() {
		var r Report

		err = rows.Scan(
			&r.ID,
			&r.TaskID,
			&r.Kind,
			&r.Content,
			&r.ReportedAt,
			&r.CreatedAt,
			&r.Source,
			&r.SourceID,
			&r.Goal,
			&r.Status,
		)
		if err != nil {
			return nil, fmt.Errorf("scan report: %w", err)
		}

		reports = append(reports, r)
	}

	return reports, rows.Err()
}

func (s *Service) MarkReported(ctx context.Context, reportID string) error {
	_, err := s.db.ExecContext(ctx, queries.TaskReportMarkReported, reportID)
	if err != nil {
		return fmt.Errorf("mark report reported %s: %w", reportID, err)
	}

	return nil
}

type ActiveTask struct {
	ID        string
	Goal      string
	Status    string
	CreatedAt time.Time
}

func (s *Service) ActiveTasks(ctx context.Context, source, sourceID string) ([]ActiveTask, error) {
	rows, err := s.db.QueryContext(ctx, queries.TaskActive, source, sourceID)
	if err != nil {
		return nil, fmt.Errorf("query active tasks: %w", err)
	}
	defer rows.Close()

	var tasks []ActiveTask
	for rows.Next() {
		var t ActiveTask

		err = rows.Scan(&t.ID, &t.Goal, &t.Status, &t.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan active task: %w", err)
		}

		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

func (s *Service) StaleTasks(ctx context.Context, threshold time.Duration) ([]Task, error) {
	cutoff := time.Now().UTC().Add(-threshold)

	rows, err := s.db.QueryContext(ctx, queries.TaskStale, cutoff)
	if err != nil {
		return nil, fmt.Errorf("query stale tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var activationID sql.NullString
		var crewMemberID sql.NullString

		err = rows.Scan(
			&t.ID,
			&t.Source,
			&t.SourceID,
			&activationID,
			&crewMemberID,
			&t.Goal,
			&t.Plan,
			&t.Thinking,
			&t.Status,
			&t.CreatedAt,
			&t.StartedAt,
			&t.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan stale task: %w", err)
		}

		t.ActivationID = activationID.String
		t.CrewMemberID = crewMemberID.String
		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}
