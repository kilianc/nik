package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
	"github.com/kciuffolo/nik/internal/messaging"
	"github.com/kciuffolo/nik/internal/prompt"
	"github.com/kciuffolo/nik/internal/timeline"
)

func main() {
	taskID := flag.String("task", "", "render task prompt for this task ID")
	convID := flag.String("conv", "", "conversation ID (default: first allowed)")
	mode := flag.String("mode", "input", "what to render: input, brain, task, nudge, task-nudge")
	flag.Parse()

	cfg, err := config.Load(os.Getenv("NIK_HOME"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	conn, err := db.Open(cfg.DBPath(), cfg.TZ())
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	r := prompt.NewRenderer(cfg)
	ctx := context.Background()

	switch *mode {
	case "input":
		renderInput(ctx, cfg, conn, r, *convID)
	case "brain":
		renderBrain(cfg, r)
	case "task":
		renderTask(ctx, cfg, conn, r, *taskID)
	case "nudge":
		fmt.Print(r.Nudge("nik-05-retry.md", struct{ Text string }{"I think we should check the logs first."}))
	case "task-nudge":
		fmt.Print(r.Nudge("task-01-nudge.md", nil))
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", *mode)
		os.Exit(1)
	}
}

func renderInput(ctx context.Context, cfg *config.Config, conn *sql.DB, r *prompt.Renderer, convID string) {
	msgSvc := messaging.NewService(cfg, conn, nil)
	tl := timeline.New(cfg, msgSvc)

	if convID == "" {
		convID = cfg.AllowedIDs()[0]
	}

	timelineStr := tl.Peek(ctx, convID)
	recall := readMemoriesRaw(cfg.Home)

	fmt.Print(r.Input(prompt.InputData{Recall: recall, Timeline: timelineStr}))
}

func renderBrain(cfg *config.Config, r *prompt.Renderer) {
	fmt.Print(r.Brain(prompt.BuildBrainData(cfg, nil, nil)))
}

func renderTask(ctx context.Context, cfg *config.Config, conn *sql.DB, r *prompt.Renderer, taskID string) {
	if taskID == "" {
		row := conn.QueryRow(`
			SELECT id FROM task
			WHERE status IN ('completed', 'failed', 'running')
			ORDER BY created_at DESC LIMIT 1
		`)
		err := row.Scan(&taskID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "find task: %v\n", err)
			os.Exit(1)
		}
	}

	t, err := db.TaskGet(ctx, conn, taskID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load task %s: %v\n", taskID, err)
		os.Exit(1)
	}

	var defs []llm.ToolDef
	row := conn.QueryRow(`
		SELECT tool_schemas FROM activation
		WHERE task_id = ?1
		ORDER BY created_at DESC LIMIT 1
	`, taskID)

	var schemasJSON string
	if row.Scan(&schemasJSON) == nil && schemasJSON != "[]" {
		_ = json.Unmarshal([]byte(schemasJSON), &defs)
	}

	fmt.Print(r.Task(prompt.BuildTaskData(cfg, t, defs)))
}

func readMemoriesRaw(home string) string {
	root, err := os.OpenRoot(home)
	if err != nil {
		return ""
	}
	defer root.Close()

	f, err := root.Open("memories/latest.md")
	if err != nil {
		return ""
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}
