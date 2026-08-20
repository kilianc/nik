package apisvc

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kciuffolo/nik/internal/api"
	"github.com/kciuffolo/nik/internal/db"
)

// Workload counts what nik is carrying. The event stream publishes changes;
// this is where a client that has just connected starts from.
type Workload struct {
	conn *sql.DB
}

func NewWorkload(conn *sql.DB) *Workload {
	return &Workload{conn: conn}
}

func (w *Workload) Counts(ctx context.Context) (api.WorkloadEvent, error) {
	alarms, err := db.AlarmCountActive(ctx, w.conn)
	if err != nil {
		return api.WorkloadEvent{}, fmt.Errorf("count alarms: %w", err)
	}

	tasks, err := db.TaskCountActive(ctx, w.conn)
	if err != nil {
		return api.WorkloadEvent{}, fmt.Errorf("count tasks: %w", err)
	}

	return api.WorkloadEvent{Alarms: alarms, Tasks: tasks}, nil
}
