package briefing

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kciuffolo/nik/internal/brain"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/websearch"
)

const (
	maxTopics          = 10
	resultsPerTopic    = 3
	defaultLocationFmt = "news in %s"
)

type DataSource struct {
	svc    *Service
	cfg    *config.Config
	client *http.Client
	mu     sync.Mutex
	active bool
}

func NewDataSource(svc *Service, cfg *config.Config) *DataSource {
	return &DataSource{
		svc:    svc,
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (d *DataSource) Check(ctx context.Context) ([]brain.DataSourceOutput, error) {
	now := d.svc.now()
	briefingAt := d.cfg.BriefingAt(now)

	if now.Before(briefingAt) {
		return nil, nil
	}

	d.mu.Lock()
	if d.active {
		d.mu.Unlock()
		return nil, nil
	}
	d.mu.Unlock()

	done, err := d.svc.HasBriefing(ctx)
	if err != nil {
		return nil, fmt.Errorf("check briefing: %w", err)
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
		return nil, fmt.Errorf("load briefing prompt: %w", err)
	}

	topics, err := d.svc.ListTopics(ctx)
	if err != nil {
		d.mu.Lock()
		d.active = false
		d.mu.Unlock()
		return nil, fmt.Errorf("list briefing topics: %w", err)
	}

	newsContext := d.fetchNews(ctx, topics)

	journal, err := d.svc.YesterdayJournal(ctx)
	if err != nil {
		slog.Warn("briefing: fetch yesterday journal", "pkg", "briefing", "error", err)
	}

	var lines []string
	lines = append(lines, "[Morning briefing]", "")
	lines = append(lines, prompt...)

	if journal != "" {
		lines = append(lines, "", "---", "", "## Yesterday's journal", "", journal)
	}

	lines = append(lines, "", "---", "")
	lines = append(lines, newsContext...)

	date := now.In(d.cfg.TZ()).Format("2006-01-02")
	slog.Info("briefing activation", "pkg", "briefing", "date", date, "topics", len(topics))

	output := brain.DataSourceOutput{
		Lines: lines,
		Meta: map[string]string{
			"source":    "briefing",
			"source_id": date,
		},
		Processing: func(ctx context.Context) error {
			slog.Info("briefing started", "pkg", "briefing", "date", date)
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

func (d *DataSource) fetchNews(ctx context.Context, topics []db.BriefingTopic) []string {
	apiKey := d.cfg.ExaAPIKey
	if apiKey == "" {
		return []string{"(no exa_api_key configured — skipping news fetch)", ""}
	}

	type topicQuery struct {
		label string
		query string
	}

	var queries []topicQuery

	if d.cfg.Location != "" {
		queries = append(queries, topicQuery{
			label: fmt.Sprintf("Local news (%s)", d.cfg.Location),
			query: fmt.Sprintf(defaultLocationFmt, d.cfg.Location),
		})
	}

	for i, t := range topics {
		if i >= maxTopics {
			break
		}

		label := t.Query
		if t.Reason != "" {
			label += " — " + t.Reason
		}
		if t.ContactID.Valid && t.ContactID.String != "" {
			label += fmt.Sprintf(" (for contact %s)", t.ContactID.String)
		}

		queries = append(queries, topicQuery{
			label: label,
			query: t.Query,
		})
	}

	if len(queries) == 0 {
		return []string{
			"## No topics configured",
			"",
			"You have no briefing topics yet. Create some based on what you know about your family,",
			"their locations, and their interests. Use `briefing_topics` with action `add`.",
			"",
		}
	}

	startDate := d.svc.now().Add(-48 * time.Hour).Format("2006-01-02")

	var lines []string

	for _, tq := range queries {
		result, err := websearch.DoSearch(ctx, d.client, apiKey, tq.query, resultsPerTopic, "news", startDate)
		if err != nil {
			slog.Warn("briefing news fetch", "pkg", "briefing", "query", tq.query, "error", err)
			lines = append(lines, fmt.Sprintf("## %s", tq.label), "", fmt.Sprintf("(fetch failed: %s)", err), "")
			continue
		}

		lines = append(lines, fmt.Sprintf("## %s", tq.label), "", result, "")
	}

	return lines
}

func (d *DataSource) loadPrompt() ([]string, error) {
	path := d.cfg.PromptsPath() + "/briefing.md"

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	return strings.Split(strings.TrimSpace(string(data)), "\n"), nil
}
