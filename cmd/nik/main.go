package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kciuffolo/nik/internal/alarms"
	"github.com/kciuffolo/nik/internal/brain"
	"github.com/kciuffolo/nik/internal/codex"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/contacts"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
	niklog "github.com/kciuffolo/nik/internal/log"
	"github.com/kciuffolo/nik/internal/messaging"
	"github.com/kciuffolo/nik/internal/recall"
	"github.com/kciuffolo/nik/internal/shell"
	"github.com/kciuffolo/nik/internal/skills"
	"github.com/kciuffolo/nik/internal/stats"
	"github.com/kciuffolo/nik/internal/task"
	"github.com/kciuffolo/nik/internal/timeline"
	"github.com/kciuffolo/nik/internal/whatsapp"
)

const version = "0.0.1"

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	conn, err := db.Open(cfg.DBPath(), cfg.TZ())
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
	llmOpts = append(llmOpts, llm.WithReasoningEffort(&cfg.Models.Main.ReasoningEffort))
	llmOpts = append(llmOpts, llm.WithVerbosity(&cfg.Models.Main.Verbosity))
	llmClient := llm.NewClient(&cfg.Models.Main.Model, llmOpts...)

	var recallClient *llm.Client
	if cfg.Models.Recall.Model != "" && cfg.OpenAIKey != "" {
		recallOpts := []llm.ClientOption{
			llm.WithAPIKey(cfg.OpenAIKey),
			llm.WithReasoningEffort(&cfg.Models.Recall.ReasoningEffort),
			llm.WithVerbosity(&cfg.Models.Recall.Verbosity),
		}
		recallClient = llm.NewClient(&cfg.Models.Recall.Model, recallOpts...)
		slog.Info("recall client ready", "model", cfg.Models.Recall.Model)
	}

	alarmSvc := alarms.New(conn)
	recallSvc := recall.NewService(cfg, recallClient)
	taskSvc := task.NewService(conn)
	shellSvc := shell.NewService(conn, cfg.Home)

	// worker tools: subset available to background task runners.
	// workers can execute commands, query the DB, describe media, and load skills.
	// they cannot message users, manage tasks, or set alarms -- only nik does that.
	var taskTools []llm.Tool
	taskTools = append(taskTools, shellSvc.BuildTools()...)
	taskTools = append(taskTools, llm.BuildTools(llmClient, cfg.Home)...)
	taskTools = append(taskTools, db.BuildTools(conn)...)
	taskTools = append(taskTools, skills.BuildTools(cfg)...)

	var workerToolNames []string
	for _, t := range taskTools {
		workerToolNames = append(workerToolNames, t.Def.Name)
	}

	llmClient.SetObserver(stats.NewRecorder(conn))

	messagingSvc.SetSpeechFn(func(ctx context.Context, text string) (string, error) {
		return llmClient.Speech(
			ctx,
			text,
			cfg.TTSModelOrDefault(),
			cfg.TTSVoiceOrDefault(),
			cfg.Models.TTS.Instructions,
			cfg.TTSSpeedOrDefault(),
		)
	})

	taskRunner := task.NewRunner(cfg, llmClient, taskSvc, taskTools)

	if cfg.Models.Critic.Enabled && cfg.OpenAIKey != "" {
		criticOpts := []llm.ClientOption{
			llm.WithAPIKey(cfg.OpenAIKey),
			llm.WithReasoningEffort(&cfg.Models.Critic.ReasoningEffort),
			llm.WithVerbosity(&cfg.Models.Critic.Verbosity),
		}
		criticClient := llm.NewClient(&cfg.Models.Critic.Model, criticOpts...)
		criticClient.SetObserver(stats.NewRecorder(conn))
		taskRunner.SetCriticLLM(criticClient)
		slog.Info("critic client ready", "model", cfg.Models.Critic.Model, "enabled", cfg.Models.Critic.Enabled)
	}

	b := brain.New(cfg, llmClient)

	b.SetWorkerToolNames(workerToolNames)
	b.SetRecaller(recallSvc.Recall)

	b.RegisterReflex(0, taskSvc.CheckStale)
	b.RegisterReflex(0, alarmSvc.FireDueAlarms)
	b.RegisterReflex(30*time.Minute, alarmSvc.CoreAlarmEnforcer(cfg))
	b.RegisterReflex(10*time.Second, shellSvc.CheckSessions)
	b.SetSensor(timeline.New(cfg, messagingSvc, taskSvc, alarmSvc))

	b.RegisterTools(llm.BuildTools(llmClient, cfg.Home)...)
	b.RegisterTools(config.BuildTools(cfg, conn)...)
	b.RegisterTools(contacts.BuildTools(conn)...)
	b.RegisterTools(messaging.BuildTools(messagingSvc)...)
	b.RegisterTools(alarms.BuildTools(alarmSvc)...)
	b.RegisterTools(db.BuildTools(conn)...)
	b.RegisterTools(skills.BuildTools(cfg)...)
	b.RegisterTools(task.BuildTools(taskSvc, taskRunner)...)

	brainDone := make(chan struct{})
	go func() {
		b.Awake(ctx, 2*time.Second)
		close(brainDone)
	}()

	if err := whatsappClient.Connect(ctx, *wappLink); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	<-sig
	slog.Info("shutting down, waiting for in-flight work (ctrl-c again to force)")
	cancel()

	go func() {
		<-sig
		slog.Warn("force exit")
		os.Exit(1)
	}()

	<-brainDone
	taskRunner.Wait()
	slog.Info("shutdown complete")
}
