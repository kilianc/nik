package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/alarms"
	"github.com/kciuffolo/nik/internal/brain"
	"github.com/kciuffolo/nik/internal/codex"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/contacts"
	"github.com/kciuffolo/nik/internal/crew"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
	niklog "github.com/kciuffolo/nik/internal/log"
	"github.com/kciuffolo/nik/internal/memory"
	"github.com/kciuffolo/nik/internal/messaging"
	"github.com/kciuffolo/nik/internal/shell"
	"github.com/kciuffolo/nik/internal/skills"
	"github.com/kciuffolo/nik/internal/stats"
	"github.com/kciuffolo/nik/internal/task"
	"github.com/kciuffolo/nik/internal/whatsapp"
)

const version = "0.0.1"

var toolEmojis = map[string]string{
	"store_memory":   "🧠",
	"search_memory":  "🔍",
	"alarm":          "⏰",
	"update_alarm":   "⏰",
	"cancel_alarm":   "🔕",
	"update_contact": "📇",
	"load_skill":     "📚",
	"task_spawn":     "🛠️",
	"update_config":  "⚙️",
	"describe_media": "👁️",
}

func main() {
	home := flag.String("home", "", "workspace directory (default: current directory)")
	wappLink := flag.Bool("force-wapp-link", false, "force WhatsApp QR pairing")
	replay := flag.String("wapp-replay-history", "", "replay recorded history sync from file")
	flag.Parse()

	cfg, err := config.Load(*home)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	logFile, err := os.OpenFile(cfg.LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()
	logOpts := &slog.HandlerOptions{Level: slog.LevelInfo}
	fileHandler := slog.NewTextHandler(logFile, logOpts)
	stderrHandler := &niklog.TruncHandler{Inner: slog.NewTextHandler(os.Stderr, logOpts)}
	logger := slog.New(&niklog.MultiHandler{Handlers: []slog.Handler{fileHandler, stderrHandler}})
	slog.SetDefault(logger)

	ascii := []string{
		"oooo   oooo ooooo oooo   oooo",
		" 8888o  88   888   888  o88",
		" 88 888o88   888   888888",
		" 88   8888   888   888  88o",
		"o88o    88  o888o o888o o888o",
		"",
		"Noetic Intelligence Kernel v" + version,
		"",
	}

	motd := strings.Join(ascii, "\n")
	fmt.Println()
	fmt.Println(motd)
	fmt.Println()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	conn, err := db.Open(cfg.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	slog.Info("database ready", "path", cfg.DBPath())

	mediaPath := cfg.MediaPath()
	err = os.MkdirAll(mediaPath, 0o755)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating media dir: %v\n", err)
		os.Exit(1)
	}

	whatsappClient, err := whatsapp.NewClient(cfg.WappSessionDBPath(), mediaPath, cfg.MediaDir())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer whatsappClient.Close()

	contactsSvc := contacts.NewService(conn)
	messagingSvc := messaging.NewService(cfg, conn, contactsSvc)
	whatsappAdapter := whatsapp.NewAdapter(whatsappClient)

	messagingSvc.RegisterPlatform(whatsappAdapter)
	whatsappAdapter.Start(ctx, messagingSvc)

	if *replay != "" {
		if err := whatsappClient.ReplayHistorySync(*replay); err != nil {
			fmt.Fprintf(os.Stderr, "replay error: %v\n", err)
			os.Exit(1)
		}
		slog.Info("replay finished, exiting")
		return
	}

	var llmOpts []llm.ClientOption
	if cfg.OpenAIKey != "" {
		llmOpts = append(llmOpts, llm.WithAPIKey(cfg.OpenAIKey))
	}
	if cfg.UseCodex {
		auth, err := codex.LoadOrLogin("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "codex auth error: %v\n", err)
			os.Exit(1)
		}
		llmOpts = append(llmOpts, llm.WithCodex(auth))
		slog.Info("codex auth ready", "account_id", auth.AccountID)
	}
	llmOpts = append(llmOpts, llm.WithReasoningEffort(&cfg.ReasoningEffort))
	llmClient := llm.NewClient(cfg.Model, llmOpts...)

	alarmSvc := alarms.New(conn)
	memorySvc := memory.NewService(conn, llmClient)
	taskSvc := task.NewService(conn)
	crewSvc := crew.NewService(conn)

	// task runner tools: subset available to background subagents
	var taskTools []llm.Tool
	taskTools = append(taskTools, shell.BuildTools(cfg)...)
	taskTools = append(taskTools, llm.BuildTools(llmClient, cfg.Home)...)
	taskTools = append(taskTools, db.BuildTools(conn)...)
	taskTools = append(taskTools, memory.BuildReadTools(memorySvc)...)
	taskTools = append(taskTools, skills.BuildTools(cfg)...)

	llmClient.SetObserver(stats.NewRecorder(conn))

	taskRunner := task.NewRunner(cfg, llmClient, taskSvc, taskTools)

	b := brain.New(cfg, llmClient)

	b.SetCrewReader(crewSvc.Roster)
	b.SetToolReactor(toolEmojis, messagingSvc.React)
	b.SetDebugRecorder(brain.NewDebugRecorder(cfg.DebugPath(), llmClient.Model(), time.Now, taskSvc))

	b.RegisterDataSource(messaging.NewDataSource(cfg, messagingSvc, taskSvc))
	b.RegisterDataSource(alarms.NewDataSource(alarmSvc, messagingSvc))
	b.RegisterDataSource(task.NewDataSource(taskSvc, messagingSvc))

	b.RegisterTools(llm.BuildTools(llmClient, cfg.Home)...)
	b.RegisterTools(config.BuildTools(cfg, conn)...)
	b.RegisterTools(contacts.BuildTools(conn)...)
	b.RegisterTools(memory.BuildTools(memorySvc)...)
	b.RegisterTools(messaging.BuildTools(messagingSvc)...)
	b.RegisterTools(alarms.BuildTools(alarmSvc)...)
	b.RegisterTools(db.BuildTools(conn)...)
	b.RegisterTools(skills.BuildTools(cfg)...)
	b.RegisterTools(crew.BuildTools(crewSvc)...)
	b.RegisterTools(task.BuildTools(taskSvc, taskRunner, crewSvc)...)

	go b.Awake(ctx, 2*time.Second)

	if err := whatsappClient.Connect(ctx, *wappLink); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	<-ctx.Done()
	slog.Info("shutting down")
}
