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
	"github.com/kciuffolo/nik/internal/fs"
	"github.com/kciuffolo/nik/internal/llm"
	niklog "github.com/kciuffolo/nik/internal/log"
	"github.com/kciuffolo/nik/internal/messaging"
	"github.com/kciuffolo/nik/internal/prompt"
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
	readonly := flag.Bool("readonly", false, "receive messages but skip reflexes and activations")
	flag.Parse()

	cfg, err := config.Load(*home)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	time.Local = cfg.TZ()

	logFile, err := os.OpenFile(cfg.LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: open log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	errLogFile, err := os.OpenFile(cfg.ErrLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: open error log file: %v\n", err)
		os.Exit(1)
	}
	defer errLogFile.Close()

	logOpts := &slog.HandlerOptions{Level: slog.LevelInfo}
	fileHandler := slog.NewTextHandler(logFile, logOpts)
	stderrHandler := &niklog.TruncHandler{Inner: slog.NewTextHandler(os.Stderr, logOpts)}
	errHandler := slog.NewTextHandler(errLogFile, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(&niklog.MultiHandler{Handlers: []slog.Handler{fileHandler, stderrHandler, errHandler}})
	slog.SetDefault(logger)

	if err := run(cfg, *wappLink, *replay, *readonly); err != nil {
		slog.Error("startup", "error", err)
		os.Exit(1)
	}
}

func run(cfg *config.Config, wappLink bool, replay string, readonly bool) error {
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
		return err
	}
	defer conn.Close()

	err = db.SystemContactEnsure(ctx, conn)
	if err != nil {
		return fmt.Errorf("ensure system contact: %w", err)
	}

	roConn, err := db.OpenReadOnly(cfg.DBPath(), cfg.TZ())
	if err != nil {
		return err
	}
	defer roConn.Close()

	slog.Info("database ready", "path", cfg.DBPath())

	mediaPath := cfg.MediaPath()

	err = os.MkdirAll(mediaPath, 0o755)
	if err != nil {
		return fmt.Errorf("create media dir: %w", err)
	}

	err = os.MkdirAll(cfg.DownloadsPath(), 0o755)
	if err != nil {
		return fmt.Errorf("create downloads dir: %w", err)
	}

	err = os.MkdirAll(cfg.TmpPath(), 0o755)
	if err != nil {
		return fmt.Errorf("create tmp dir: %w", err)
	}

	if wappLink {
		_ = os.Remove(cfg.WappSessionDBPath())
	}

	whatsappClient, err := whatsapp.NewClient(cfg.WappSessionDBPath(), mediaPath)
	if err != nil {
		return err
	}
	defer whatsappClient.Close()

	contactsSvc := contacts.NewService(conn)
	messagingSvc := messaging.NewService(cfg, conn, contactsSvc)
	whatsappAdapter := whatsapp.NewAdapter(whatsappClient)

	messagingSvc.RegisterPlatform(whatsappAdapter)
	whatsappAdapter.Start(ctx, messagingSvc)

	if replay != "" {
		err = whatsappClient.ReplayHistorySync(replay)
		if err != nil {
			return fmt.Errorf("replay: %w", err)
		}
		slog.Info("replay finished, exiting")
		return nil
	}

	var keyOpts []llm.ClientOption
	if cfg.OpenAIKey != "" {
		keyOpts = append(keyOpts, llm.WithAPIKey(cfg.OpenAIKey))
	}
	if cfg.AnthropicKey != "" {
		keyOpts = append(keyOpts, llm.WithAnthropicKey(cfg.AnthropicKey))
	}

	var codexAuth *codex.Auth
	if cfg.Models.AnySubscription() {
		codexAuth, err = codex.LoadOrLogin("")
		if err != nil {
			return fmt.Errorf("codex auth: %w", err)
		}
		slog.Info("codex auth ready", "account_id", codexAuth.AccountID)
	}

	mainOpts := append([]llm.ClientOption{}, keyOpts...)
	if cfg.Models.Main.IsSubscription() {
		mainOpts = append(mainOpts, llm.WithCodex(codexAuth))
	}
	mainOpts = append(mainOpts, llm.WithReasoningEffort(&cfg.Models.Main.ReasoningEffort))
	mainOpts = append(mainOpts, llm.WithVerbosity(&cfg.Models.Main.Verbosity))
	llmClient := llm.NewClient(&cfg.Models.Main.Model, mainOpts...)

	var recallClient *llm.Client
	if cfg.Models.Recall.Model != "" && (cfg.OpenAIKey != "" || cfg.AnthropicKey != "") {
		recallOpts := []llm.ClientOption{
			llm.WithReasoningEffort(&cfg.Models.Recall.ReasoningEffort),
			llm.WithVerbosity(&cfg.Models.Recall.Verbosity),
		}
		if cfg.OpenAIKey != "" {
			recallOpts = append(recallOpts, llm.WithAPIKey(cfg.OpenAIKey))
		}
		if cfg.AnthropicKey != "" {
			recallOpts = append(recallOpts, llm.WithAnthropicKey(cfg.AnthropicKey))
		}
		recallClient = llm.NewClient(&cfg.Models.Recall.Model, recallOpts...)
		slog.Info("recall client ready", "model", cfg.Models.Recall.Model)
	}

	taskLLMClient := llmClient
	if cfg.Models.Task.Model != "" {
		taskOpts := append([]llm.ClientOption{}, keyOpts...)
		if cfg.Models.Task.IsSubscription() {
			taskOpts = append(taskOpts, llm.WithCodex(codexAuth))
		}
		taskOpts = append(taskOpts, llm.WithReasoningEffort(&cfg.Models.Task.ReasoningEffort))
		taskOpts = append(taskOpts, llm.WithVerbosity(&cfg.Models.Task.Verbosity))
		taskLLMClient = llm.NewClient(&cfg.Models.Task.Model, taskOpts...)
		slog.Info("task client ready", "model", cfg.Models.Task.Model)
	}

	alarmSvc := alarms.New(cfg, conn)
	recallSvc := recall.NewService(cfg, recallClient)
	taskSvc := task.NewService(conn)
	shellSvc := shell.NewService(cfg, conn)

	err = shellSvc.EnsureReady()
	if err != nil {
		return err
	}
	slog.Info("shell ready", "pkg", "shell", "docker", cfg.Shell.DockerImage != "")

	var taskTools []llm.Tool
	taskTools = append(taskTools, shellSvc.BuildTools()...)
	taskTools = append(taskTools, llm.BuildTools(taskLLMClient, cfg.Home, nil)...)
	taskTools = append(taskTools, db.BuildTools(roConn, conn, cfg.RetentionOrDefault)...)
	taskTools = append(taskTools, fs.BuildTools(cfg.Home)...)

	var workerToolNames []string
	taskTools = append(taskTools, skills.BuildTools(cfg)...)

	for _, t := range taskTools {
		workerToolNames = append(workerToolNames, t.Def.Name)
	}

	recorder := stats.NewRecorder(conn)
	pr := prompt.NewRenderer(cfg)

	messagingSvc.SetSpeechFn(func(ctx context.Context, text string) (string, error) {
		return llmClient.Speech(
			ctx,
			text,
			cfg.TTSModelOrDefault(),
			cfg.TTSVoiceOrDefault(),
			pr.TTS(),
			cfg.TTSSpeedOrDefault(),
		)
	})

	taskRunner := task.NewRunner(cfg, taskLLMClient, pr, taskSvc, taskTools)
	taskRunner.SetRecorder(recorder)

	b := brain.New(cfg, llmClient, pr)
	b.SetDB(conn)
	b.SetRecorder(recorder)

	b.SetWorkerToolNames(workerToolNames)
	b.SetRecaller(recallSvc.Recall)
	b.SetReadonly(readonly)

	b.RegisterReflex(0, taskSvc.CheckStale)
	b.RegisterReflex(0, alarmSvc.FireDueAlarms)
	b.RegisterReflex(30*time.Minute, alarmSvc.StaleAlarmReflex())
	b.RegisterReflex(5*time.Minute, skills.SkillChangeReflex(cfg, conn))
	b.RegisterReflex(5*time.Minute, skills.SkillCheckReflex(cfg, conn, llmClient.Generate, shellSvc.RunCommand))
	b.RegisterReflex(10*time.Second, shellSvc.CheckSessions)
	b.SetSensor(timeline.New(cfg, messagingSvc))

	b.RegisterTools(config.BuildTools(cfg, conn)...)
	b.RegisterTools(contacts.BuildTools(conn)...)
	b.RegisterTools(messaging.BuildTools(messagingSvc)...)
	b.RegisterTools(llm.BuildTools(llmClient, cfg.Home, messagingSvc)...)
	b.RegisterTools(alarms.BuildTools(alarmSvc)...)
	b.RegisterTools(db.BuildTools(roConn, conn, cfg.RetentionOrDefault)...)
	b.RegisterTools(fs.BuildTools(cfg.Home)...)
	b.RegisterTools(skills.BuildTools(cfg)...)
	b.RegisterTools(task.BuildTools(taskSvc, taskRunner)...)

	go func() {
		<-sig
		slog.Info("shutting down, waiting for in-flight work (ctrl-c again to force)")
		cancel()
		go func() {
			<-sig
			slog.Warn("force exit")
			os.Exit(1)
		}()
	}()

	brainDone := make(chan struct{})
	go func() {
		b.Awake(ctx, 2*time.Second)
		close(brainDone)
	}()

	err = whatsappClient.Connect(ctx, wappLink)
	if err != nil {
		return err
	}

	<-brainDone
	messagingSvc.StopPresence()
	taskRunner.Wait()
	shellSvc.StopContainer()
	slog.Info("shutdown complete")

	return nil
}
