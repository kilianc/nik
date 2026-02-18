package journal

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kciuffolo/nik/internal/brain"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/messaging"
)

type DataSource struct {
	svc    *Service
	conn   *sql.DB
	msgs   *messaging.Service
	cfg    *config.Config
	mu     sync.Mutex
	active bool
}

func NewDataSource(svc *Service, conn *sql.DB, msgsSvc *messaging.Service, cfg *config.Config) *DataSource {
	return &DataSource{
		svc:  svc,
		conn: conn,
		msgs: msgsSvc,
		cfg:  cfg,
	}
}

func (d *DataSource) Check(ctx context.Context) ([]brain.DataSourceOutput, error) {
	now := d.svc.now()
	journalAt := d.cfg.JournalAt(now)

	if now.Before(journalAt) {
		return nil, nil
	}

	d.mu.Lock()
	if d.active {
		d.mu.Unlock()
		return nil, nil
	}
	d.mu.Unlock()

	done, err := d.svc.HasPage(ctx)
	if err != nil {
		return nil, fmt.Errorf("check journal page: %w", err)
	}
	if done {
		return nil, nil
	}

	d.mu.Lock()
	d.active = true
	d.mu.Unlock()

	prompt, err := d.loadPrompt()
	if err != nil {
		d.mu.Lock()
		d.active = false
		d.mu.Unlock()
		return nil, fmt.Errorf("load journal prompt: %w", err)
	}

	date := now.In(d.cfg.TZ()).Format("2006-01-02")
	dayStart, dayEnd := dayBounds(now, d.cfg)
	dayContext := buildDayContext(ctx, d.conn, d.msgs, dayStart, dayEnd)

	var lines []string
	lines = append(lines, "[End of day journal]", "")
	lines = append(lines, prompt...)
	lines = append(lines, "")
	lines = append(lines, "---", "")
	lines = append(lines, dayContext...)

	slog.Info("journal activation", "pkg", "journal", "date", date)

	output := brain.DataSourceOutput{
		Lines: lines,
		Meta:  map[string]string{},
		Processing: func(ctx context.Context) error {
			slog.Info("journal started", "pkg", "journal", "date", date)
			return nil
		},
		Processed: func(ctx context.Context) error {
			d.mu.Lock()
			d.active = false
			d.mu.Unlock()
			return nil
		},
	}

	return []brain.DataSourceOutput{output}, nil
}

// dayBounds returns the calendar day the journal reflects on. at midnight,
// this is the day that just ended.
func dayBounds(now time.Time, cfg *config.Config) (time.Time, time.Time) {
	loc := cfg.TZ()
	journalAt := cfg.JournalAt(now)
	effectiveDay := journalAt.Add(-time.Minute)

	y, m, d := effectiveDay.In(loc).Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)

	return start, end
}

func (d *DataSource) loadPrompt() ([]string, error) {
	path := d.cfg.PromptsPath() + "/01-journal.md"

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	return strings.Split(strings.TrimSpace(string(data)), "\n"), nil
}
