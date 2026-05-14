package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kciuffolo/nik/internal/alarms"
	"github.com/kciuffolo/nik/internal/brain"
	"github.com/kciuffolo/nik/internal/codex"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/contacts"
	"github.com/kciuffolo/nik/internal/daemonctl"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/fs"
	"github.com/kciuffolo/nik/internal/genesis"
	"github.com/kciuffolo/nik/internal/llm"
	niklog "github.com/kciuffolo/nik/internal/log"
	"github.com/kciuffolo/nik/internal/messaging"
	"github.com/kciuffolo/nik/internal/prompt"
	"github.com/kciuffolo/nik/internal/recall"
	"github.com/kciuffolo/nik/internal/secrets"
	"github.com/kciuffolo/nik/internal/shell"
	"github.com/kciuffolo/nik/internal/skills"
	"github.com/kciuffolo/nik/internal/stats"
	"github.com/kciuffolo/nik/internal/task"
	"github.com/kciuffolo/nik/internal/timeline"
	"github.com/kciuffolo/nik/internal/version"
	"github.com/kciuffolo/nik/internal/whatsapp"
)

func main() {
	subcmd := ""
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		subcmd = os.Args[1]
	}

	known := []string{"daemon", "install", "replay", "secrets", "tui"}

	switch subcmd {
	case "daemon":
		runDaemon(os.Args[2:])
	case "install":
		runInstall(os.Args[2:])
	case "replay":
		runReplay(os.Args[2:])
	case "secrets":
		runSecrets(os.Args[2:])
	case "tui":
		runTUI(os.Args[2:])
	case "":
		runTUI(os.Args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", subcmd)
		fmt.Fprintf(os.Stderr, "available commands: %s\n", strings.Join(known, ", "))
		os.Exit(1)
	}
}

func runDaemon(args []string) {
	flagSet := flag.NewFlagSet("daemon", flag.ExitOnError)
	home := flagSet.String("home", "", "workspace directory")
	readonly := flagSet.Bool("readonly", false, "receive messages but skip reflexes and activations")
	flagSet.Parse(args)

	ascii := []string{
		"oooo   oooo ooooo oooo   oooo",
		" 8888o  88   888   888  o88",
		" 88 888o88   888   888888",
		" 88   8888   888   888  88o",
		"o88o    88  o888o o888o o888o",
		"",
		"Noetic Intelligence Kernel v" + version.V,
		"",
	}

	motd := strings.Join(ascii, "\n")
	fmt.Println()
	fmt.Println(motd)
	fmt.Println()

	h, err := resolveHome(*home)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if _, alive := daemonctl.CheckPID(h); alive {
		fmt.Fprintln(os.Stderr, "error: another daemon is already running")
		os.Exit(1)
	}

	err = daemonctl.WritePID(h)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: write pid file: %v\n", err)
		os.Exit(1)
	}
	defer daemonctl.RemovePID(h)

	logFile, err := os.OpenFile(filepath.Join(h, "nik.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: open log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	errLogFile, err := os.OpenFile(filepath.Join(h, "nik.err.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
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

	fatal := func(msg string, err error) {
		slog.Error(msg, "error", err)
		os.Exit(1)
	}

	idle := func(reason string) {
		slog.Info("not ready, idling until restart", "reason", reason)
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
	}

	cfg, err := config.Load(h)
	if err != nil {
		idle(err.Error())
		return
	}

	time.Local = cfg.TZ()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// database

	conn, err := db.Open(cfg.DBPath(), cfg.TZ())
	if err != nil {
		fatal("open database", err)
	}
	defer conn.Close()

	err = db.SystemContactEnsure(ctx, conn)
	if err != nil {
		fatal("ensure system contact", err)
	}

	err = db.NikContactEnsure(ctx, conn)
	if err != nil {
		fatal("ensure nik contact", err)
	}

	err = db.OwnerContactEnsure(ctx, conn)
	if err != nil {
		fatal("ensure owner contact", err)
	}

	err = db.LocalConversationEnsure(ctx, conn)
	if err != nil {
		fatal("ensure local conversation", err)
	}

	err = db.ToolCallStartRecover(ctx, conn)
	if err != nil {
		slog.Warn("recover orphaned tool call starts", "error", err)
	}

	// read-only database connection for db_query tool
	roConn, err := db.OpenReadOnly(cfg.DBPath(), cfg.TZ())
	if err != nil {
		fatal("open read-only database", err)
	}
	defer roConn.Close()

	slog.Info("database ready", "path", cfg.DBPath())

	// ensure dirs

	err = os.MkdirAll(cfg.MediaPath(), 0o755)
	if err != nil {
		fatal("create media dir", err)
	}

	err = os.MkdirAll(cfg.DownloadsPath(), 0o755)
	if err != nil {
		fatal("create downloads dir", err)
	}

	err = os.MkdirAll(cfg.TmpPath(), 0o755)
	if err != nil {
		fatal("create tmp dir", err)
	}

	secrets.EnsureAdapter(cfg.Home, cfg.SkillsPath())

	// adapters

	contactsSvc := contacts.NewService(conn)
	messagingSvc := messaging.NewService(cfg, conn, contactsSvc)

	// local adapter
	messagingSvc.RegisterPlatform(messaging.NewLocalAdapter(conn))
	slog.Info("local adapter active")

	// whatsapp adapter
	var whatsappClient *whatsapp.Client
	if _, err := os.Stat(cfg.WappSessionDBPath()); err == nil {
		whatsappClient, err = whatsapp.NewClient(cfg.WappSessionDBPath(), cfg.MediaPath())
		if err != nil {
			fatal("create whatsapp client", err)
		}
		defer whatsappClient.Close()

		whatsappAdapter := whatsapp.NewAdapter(whatsappClient)
		messagingSvc.RegisterPlatform(whatsappAdapter)
		err = whatsappAdapter.Start(ctx, messagingSvc)
		if err != nil {
			fatal("start whatsapp adapter", err)
		}
	}

	// llm clients

	secretStore := secrets.New(h)
	openaiKey, _ := secretStore.Get("openai_key")
	anthropicKey, _ := secretStore.Get("anthropic_key")

	var sharedLLMOpts []llm.ClientOption
	if openaiKey != "" {
		sharedLLMOpts = append(sharedLLMOpts, llm.WithAPIKey(openaiKey))
	}
	if anthropicKey != "" {
		sharedLLMOpts = append(sharedLLMOpts, llm.WithAnthropicKey(anthropicKey))
	}

	if openaiKey == "" && anthropicKey == "" && !cfg.Models.NeedsCodexAuth() {
		fatal("auth", fmt.Errorf("no openai_key or anthropic_key in secrets store and no codex subscription configured"))
	}

	var codexAuth *codex.Auth
	if cfg.Models.NeedsCodexAuth() {
		codexAuth, err = codex.Load("")
		if err != nil {
			fatal("codex auth", err)
		}
		slog.Info("codex auth ready", "account_id", codexAuth.AccountID)
	}

	mainLLMOpts := append([]llm.ClientOption{}, sharedLLMOpts...)
	if cfg.Models.Main.UsesCodexAuth() {
		mainLLMOpts = append(mainLLMOpts, llm.WithCodex(codexAuth))
	}
	mainLLMOpts = append(mainLLMOpts, llm.WithReasoningEffort(&cfg.Models.Main.ReasoningEffort))
	mainLLMOpts = append(mainLLMOpts, llm.WithVerbosity(&cfg.Models.Main.Verbosity))
	llmClient := llm.NewClient(&cfg.Models.Main.Model, mainLLMOpts...)

	var recallClient *llm.Client
	if cfg.Models.Recall.Model != "" && len(sharedLLMOpts) > 0 {
		recallLLMOpts := append([]llm.ClientOption{}, sharedLLMOpts...)
		recallLLMOpts = append(recallLLMOpts, llm.WithReasoningEffort(&cfg.Models.Recall.ReasoningEffort))
		recallLLMOpts = append(recallLLMOpts, llm.WithVerbosity(&cfg.Models.Recall.Verbosity))
		recallClient = llm.NewClient(&cfg.Models.Recall.Model, recallLLMOpts...)
		slog.Info("recall client ready", "model", cfg.Models.Recall.Model)
	}

	taskLLMClient := llmClient
	if cfg.Models.Task.Model != "" {
		taskLLMOpts := append([]llm.ClientOption{}, sharedLLMOpts...)
		if cfg.Models.Task.UsesCodexAuth() {
			taskLLMOpts = append(taskLLMOpts, llm.WithCodex(codexAuth))
		}
		taskLLMOpts = append(taskLLMOpts, llm.WithReasoningEffort(&cfg.Models.Task.ReasoningEffort))
		taskLLMOpts = append(taskLLMOpts, llm.WithVerbosity(&cfg.Models.Task.Verbosity))
		taskLLMClient = llm.NewClient(&cfg.Models.Task.Model, taskLLMOpts...)
		slog.Info("task client ready", "model", cfg.Models.Task.Model)
	}

	// services

	pr := prompt.NewRenderer(cfg)

	recorder := stats.NewRecorder(conn)
	alarmSvc := alarms.New(cfg, conn)
	recallSvc := recall.NewService(cfg, recallClient)
	taskSvc := task.NewService(conn)
	nikBin, _ := os.Executable()
	shellSvc := shell.NewService(cfg, conn, nikBin)

	err = shellSvc.EnsureReady()
	if err != nil {
		fatal("shell setup", err)
	}
	slog.Info("shell ready", "pkg", "shell", "docker", cfg.Shell.DockerImage != "")

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

	// task runner

	var taskTools []llm.Tool
	taskTools = append(taskTools, shellSvc.BuildTools()...)
	taskTools = append(taskTools, llm.BuildTools(taskLLMClient, cfg.Home, nil)...)
	taskTools = append(taskTools, db.BuildTools(roConn, conn, cfg.RetentionOrDefault)...)
	taskTools = append(taskTools, fs.BuildTools(cfg.Home)...)
	taskTools = append(taskTools, skills.BuildTools(cfg)...)

	workerToolNames := make([]string, len(taskTools))
	for i, t := range taskTools {
		workerToolNames[i] = t.Def.Name
	}

	taskRunner := task.NewRunner(cfg, taskLLMClient, pr, taskSvc, taskTools)
	taskRunner.SetRecorder(recorder)

	// brain

	b := brain.New(cfg, llmClient, pr)
	b.SetDB(conn)
	b.SetRecorder(recorder)
	b.SetActivity(messagingSvc)
	b.SetWorkerToolNames(workerToolNames)
	b.SetRecaller(recallSvc.Recall)
	b.SetReadonly(*readonly)

	b.SetSensor(timeline.New(cfg, messagingSvc))

	// reflexes: operational always, skill reflexes post-genesis only

	b.RegisterReflex(0, taskSvc.CheckStale)
	b.RegisterReflex(0, alarmSvc.FireDueAlarms)
	b.RegisterReflex(10*time.Second, shellSvc.CheckSessions)
	b.RegisterReflex(30*time.Minute, alarmSvc.StaleAlarmReflex())

	genesisCompletedAt, err := db.SettingGet(ctx, conn, "genesis_completed_at")
	if err != nil {
		fatal("check genesis setting", err)
	}

	if _, err := db.GenesisStartedAtEnsure(ctx, conn); err != nil {
		slog.Warn("stamp genesis_started_at", "pkg", "main", "error", err)
	}

	if genesisCompletedAt == nil {
		b.RegisterReflex(0, genesis.Reflex(conn))
		slog.Info("genesis mode active", "pkg", "main")
	} else {
		b.RegisterReflex(5*time.Minute, skills.SkillChangeReflex(cfg, conn))
		b.RegisterReflex(5*time.Minute, skills.SkillCheckReflex(cfg, conn, llmClient.Generate, shellSvc.RunCommand))
	}

	// tools: shell only during genesis; post-genesis nik delegates to tasks

	b.RegisterTool(llm.Tool{Def: brain.DoneToolDef, Handler: brain.DoneHandler()})
	b.RegisterTool(llm.Tool{
		Def:     daemonctl.RestartToolDef,
		Handler: daemonctl.RestartHandler(),
	})
	b.RegisterTools(config.BuildTools(cfg, conn)...)
	b.RegisterTools(contacts.BuildTools(conn)...)
	b.RegisterTools(messaging.BuildTools(messagingSvc)...)
	b.RegisterTools(llm.BuildTools(llmClient, cfg.Home, messagingSvc)...)
	b.RegisterTools(alarms.BuildTools(alarmSvc)...)
	b.RegisterTools(db.BuildTools(roConn, conn, cfg.RetentionOrDefault)...)
	b.RegisterTools(fs.BuildTools(cfg.Home)...)
	b.RegisterTools(skills.BuildTools(cfg)...)
	b.RegisterTools(task.BuildTools(taskSvc, taskRunner)...)

	if genesisCompletedAt == nil {
		b.RegisterTools(shellSvc.BuildTools()...)
	}

	privilegedTools := []string{
		"config",
		"shell",
		"shell-rebuild",
		"shell-factory-reset",
		"db_query",
		"db_prune",
		"read_file",
		"write_file",
	}

	b.Privileged(privilegedTools...)
	taskRunner.Privileged(privilegedTools...)

	// start

	if whatsappClient != nil {
		err = whatsappClient.Connect(ctx, false)
		if err != nil {
			fatal("whatsapp connect", err)
		}
	}

	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

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

	// shutdown

	<-brainDone
	messagingSvc.StopPresence()
	taskRunner.Wait()
	shellSvc.StopContainer()
	slog.Info("shutdown complete")
}

func resolveHome(override string) (string, error) {
	h := override
	if h == "" {
		h = os.Getenv("NIK_HOME")
	}
	if h == "" {
		u, err := user.Current()
		if err != nil {
			return "", fmt.Errorf("get current user: %w", err)
		}
		h = filepath.Join(u.HomeDir, ".nik")
	}

	abs, err := filepath.Abs(h)
	if err != nil {
		return "", fmt.Errorf("resolve home path: %w", err)
	}

	err = os.MkdirAll(abs, 0o755)
	if err != nil {
		return "", fmt.Errorf("create home dir: %w", err)
	}

	return abs, nil
}
