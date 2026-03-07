package dream

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kciuffolo/nik/internal/brain"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/memory"
)

var passNames = map[int]string{
	1: "Drift",
	2: "Weave",
	3: "Depths",
	4: "Crystallize",
	5: "Wake",
}

const (
	recentMemoryLimit = 15
	randomMemoryLimit = 10
	randomMemoryAge   = 7 * 24 * time.Hour
)

type DataSource struct {
	svc    *Service
	conn   *sql.DB
	mem    *memory.Service
	cfg    *config.Config
	mu     sync.Mutex
	active bool
}

func NewDataSource(svc *Service, conn *sql.DB, mem *memory.Service, cfg *config.Config) *DataSource {
	return &DataSource{
		svc:  svc,
		conn: conn,
		mem:  mem,
		cfg:  cfg,
	}
}

func (d *DataSource) Check(ctx context.Context) ([]brain.DataSourceOutput, error) {
	now := d.svc.now()

	pass := d.duePass(ctx, now)
	if pass == 0 {
		return nil, nil
	}

	d.mu.Lock()
	if d.active {
		d.mu.Unlock()
		return nil, nil
	}
	d.active = true
	d.mu.Unlock()

	lines, err := d.buildContext(ctx, now, pass)
	if err != nil {
		d.mu.Lock()
		d.active = false
		d.mu.Unlock()
		return nil, fmt.Errorf("build dream context pass %d: %w", pass, err)
	}

	passStr := strconv.Itoa(pass)
	name := passNames[pass]

	slog.Info("dream activation", "pkg", "dream", "pass", pass, "name", name, "date", d.svc.tonight())

	output := brain.DataSourceOutput{
		Lines: lines,
		Meta: map[string]string{
			"dream_pass": passStr,
		},
		Processing: func(ctx context.Context) error {
			slog.Info("dream started", "pkg", "dream", "pass", pass, "name", name)
			return d.svc.StartPass(ctx, pass)
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

// duePass returns the latest eligible unfired pass, or 0 if none is due.
// dreams don't replay: if nik missed 2am, at 3am it fires pass 2 not pass 1.
func (d *DataSource) duePass(ctx context.Context, now time.Time) int {
	for pass := totalPasses; pass >= 1; pass-- {
		at := d.cfg.DreamAt(now, pass)
		if now.Before(at) {
			continue
		}

		done, err := d.svc.HasPass(ctx, pass)
		if err != nil {
			slog.Error("check dream pass", "pkg", "dream", "pass", pass, "error", err)
			continue
		}
		if done {
			continue
		}

		return pass
	}

	return 0
}

func (d *DataSource) buildContext(ctx context.Context, now time.Time, pass int) ([]string, error) {
	var lines []string

	if pass == totalPasses {
		lines = append(lines, "[Wake]", "")
	} else {
		name := passNames[pass]
		lines = append(lines, fmt.Sprintf("[Dream — Pass %d: %s]", pass, name), "")
	}

	prompt, err := d.loadPrompt(pass)
	if err != nil {
		return nil, err
	}
	lines = append(lines, prompt...)
	lines = append(lines, "", "---", "")

	if pass == 1 {
		lines = append(lines, d.pass1Context(ctx, now)...)
	} else if pass == totalPasses {
		lines = append(lines, d.wakeContext(ctx)...)
	} else {
		lines = append(lines, d.dreamContext(ctx)...)
	}

	return lines, nil
}

func (d *DataSource) loadPrompt(pass int) ([]string, error) {
	name := "dream.md"
	if pass == totalPasses {
		name = "wake.md"
	}

	path := d.cfg.PromptsPath() + "/" + name

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	return strings.Split(strings.TrimSpace(string(data)), "\n"), nil
}

// pass1Context seeds the first dream with the journal entry and mixed memories.
func (d *DataSource) pass1Context(ctx context.Context, now time.Time) []string {
	var lines []string

	journal := d.journalEntry(ctx)
	if journal != "" {
		lines = append(lines, "## Tonight's journal", "", journal, "")
	}

	recent := d.recentMemories(ctx)
	if len(recent) > 0 {
		lines = append(lines, "## Recent memories", "")
		for _, m := range recent {
			lines = append(lines, fmt.Sprintf("- %s", m))
		}
		lines = append(lines, "")
	}

	older := d.randomOlderMemories(ctx, now)
	if len(older) > 0 {
		lines = append(lines, "## Older memories surfacing", "")
		for _, m := range older {
			lines = append(lines, fmt.Sprintf("- [%s] %s", m.CreatedAt.Format("Jan 2"), m.Content))
		}
		lines = append(lines, "")
	}

	return lines
}

// dreamContext provides previous passes' content for passes 2-4.
func (d *DataSource) dreamContext(ctx context.Context) []string {
	return d.previousPasses(ctx)
}

// wakeContext provides all dream passes for the wake pass.
func (d *DataSource) wakeContext(ctx context.Context) []string {
	return d.previousPasses(ctx)
}

func (d *DataSource) previousPasses(ctx context.Context) []string {
	passes, err := d.svc.GetPasses(ctx)
	if err != nil {
		slog.Warn("dream context: previous passes", "pkg", "dream", "error", err)
		return nil
	}

	if len(passes) == 0 {
		return nil
	}

	var lines []string
	for _, p := range passes {
		name := passNames[p.Pass]
		lines = append(lines, fmt.Sprintf("## Dream Pass %d: %s", p.Pass, name), "")
		lines = append(lines, p.Content, "")
	}

	return lines
}

func (d *DataSource) journalEntry(_ context.Context) string {
	date := d.svc.tonight()
	path := d.cfg.Home + "/journal/" + date + ".md"

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("dream context: journal", "pkg", "dream", "error", err)
		}
		return ""
	}

	return strings.TrimSpace(string(data))
}

func (d *DataSource) recentMemories(ctx context.Context) []string {
	if d.mem == nil {
		return nil
	}

	memories, err := d.mem.List(ctx, recentMemoryLimit)
	if err != nil {
		slog.Warn("dream context: recent memories", "pkg", "dream", "error", err)
		return nil
	}

	var lines []string
	for _, m := range memories {
		lines = append(lines, m.Content)
	}

	return lines
}

func (d *DataSource) randomOlderMemories(ctx context.Context, now time.Time) []db.RandomMemory {
	threshold := now.Add(-randomMemoryAge)

	memories, err := db.MemoryRandom(ctx, d.conn, threshold, randomMemoryLimit)
	if err != nil {
		slog.Warn("dream context: random memories", "pkg", "dream", "error", err)
		return nil
	}

	return memories
}
