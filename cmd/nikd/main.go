// Command nikd is the nik daemon: the brain loop, the gateway session, the
// shell sandbox, and the only process that opens NIK_HOME. It takes flags and
// has no subcommands — everything a person types lives in nikctl.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kciuffolo/nik/internal/alarms"
	"github.com/kciuffolo/nik/internal/api"
	"github.com/kciuffolo/nik/internal/apisvc"
	"github.com/kciuffolo/nik/internal/brain"
	"github.com/kciuffolo/nik/internal/codex"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/contacts"
	"github.com/kciuffolo/nik/internal/daemonctl"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/fs"
	"github.com/kciuffolo/nik/internal/gateway"
	"github.com/kciuffolo/nik/internal/genesis"
	"github.com/kciuffolo/nik/internal/home"
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
)

func main() {
	flagSet := flag.NewFlagSet("nikd", flag.ExitOnError)
	homeFlag := flagSet.String("home", "", "workspace directory")
	readonly := flagSet.Bool("readonly", false, "receive messages but skip reflexes and activations")
	flagSet.Parse(os.Args[1:])

	ascii := []string{
		"oooo   oooo ooooo oooo   oooo",
		" 8888o  88   888   888  o88",
		" 88 888o88   888   888888",
		" 88   8888   888   888  88o",
		"o88o    88  o888o o888o o888o",
		"",
		"Noetic Intelligence Kernel " + version.String(),
		"",
	}

	motd := strings.Join(ascii, "\n")
	fmt.Println()
	fmt.Println(motd)
	fmt.Println()

	h, err := home.Resolve(*homeFlag)
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signals are wired before anything can block, so every wait below ends
	// the same way — including the pre-config idle, which used to have its
	// own handler and its own idea of what shutting down meant.
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

	// The API comes up before the things it reports on, which is the point:
	// a daemon that cannot answer a message can still answer questions about
	// why. It is also what lets an unconfigured nik be configured in place
	// rather than restarted — nothing below is reachable until config loads,
	// but /v1/health and /v1/version are.
	state := api.NewState()
	apiSrv := api.New(state)
	socketPath := api.OwnerSocketPath(h)

	ln, err := api.Listen(socketPath)
	if err != nil {
		fatal("listen on api socket", err)
	}
	defer os.Remove(socketPath)

	apiDone := make(chan struct{})
	go func() {
		defer close(apiDone)
		err := apiSrv.Serve(ctx, ln)
		if err != nil {
			slog.Error("api server", "pkg", "main", "error", err)
		}
	}()
	slog.Info("api listening", "pkg", "main", "socket", socketPath)

	// idle keeps serving instead of sleeping. A nik with no config is not
	// broken, it is unconfigured — and the difference is that somebody can
	// still ask it what it needs.
	idle := func(reason string) {
		state.Set("config", false, reason)
		slog.Info("not configured, serving the api and waiting", "reason", reason)
		<-ctx.Done()
		<-apiDone
	}

	cfg, err := config.Load(h)
	if err != nil {
		idle(err.Error())
		return
	}
	state.Set("config", true, cfg.ConfigPath())

	time.Local = cfg.TZ()

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

	state.Set("db", true, cfg.DBPath())
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

	secrets.EnsureAdapter(cfg.Home, skills.BuiltinFS())
	secretStore := secrets.New(h)

	// adapters

	contactsSvc := contacts.NewService(conn)
	messagingSvc := messaging.NewService(cfg, conn, contactsSvc)

	// The chat endpoints go live here rather than at the end of boot: from
	// this point a client can read history and send into the local
	// conversation, and the brain picking it up is the next tick's problem.
	apiSrv.SetChat(apisvc.NewChat(conn, messagingSvc))

	// local adapter
	messagingSvc.RegisterPlatform(messaging.NewLocalAdapter(conn))
	slog.Info("local adapter active")

	// gateway adapter — nik as a nik-saas agent: no SIM, no QR, no session of
	// its own. whatsapp reaches nik only through the gateway, so a daemon
	// without one can never answer a message: say so and stop.
	if !gateway.Enabled(cfg, secretStore) {
		fatal("gateway not configured", errors.New(
			"set gateway.url in config.yaml and write the gateway_token secret"))
	}

	hostname, _ := os.Hostname()
	gatewayAdapter, err := gateway.FromConfig(cfg, secretStore, hostname)
	if err != nil {
		fatal("create gateway adapter", err)
	}

	messagingSvc.RegisterPlatform(gatewayAdapter)
	err = gatewayAdapter.Start(ctx, messagingSvc)
	if err != nil {
		fatal("start gateway adapter", err)
	}

	// Connecting is a boot step, synchronous like opening the database: the
	// gateway is nik's only transport, so nothing downstream starts until
	// hello.ack proves the token works. Transient failures retry inside the
	// session loop (visible below); a rejected token or a minute of silence
	// ends the boot. The loop keeps running for the daemon's lifetime.
	gwErr := make(chan error, 1)
	go func() { gwErr <- gatewayAdapter.Connect(ctx) }()
	select {
	case <-gatewayAdapter.Ready():
		state.Set("gateway", true, cfg.Gateway.URL)
		slog.Info("gateway connected", "pkg", "main", "url", cfg.Gateway.URL)
	case err := <-gwErr:
		fatal("gateway", err)
	case <-time.After(time.Minute):
		fatal("gateway", errors.New("no connection within 60s: "+cfg.Gateway.URL))
	case <-ctx.Done():
		return
	}
	go func() {
		if err := <-gwErr; err != nil && ctx.Err() == nil {
			// Terminal mid-run: the token was revoked out from under us.
			state.Set("gateway", false, err.Error())
			fatal("gateway", err)
		}
	}()

	// llm clients

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

	state.Set("models", true, cfg.Models.Main.Model)

	mainLLMOpts := append([]llm.ClientOption{}, sharedLLMOpts...)
	if cfg.Models.Main.UsesCodexAuth() {
		mainLLMOpts = append(mainLLMOpts, llm.WithCodex(codexAuth))
	}
	mainLLMOpts = append(mainLLMOpts, llm.WithReasoningEffort(&cfg.Models.Main.ReasoningEffort))
	mainLLMOpts = append(mainLLMOpts, llm.WithVerbosity(&cfg.Models.Main.Verbosity))
	llmClient := llm.NewClient(&cfg.Models.Main.Model, mainLLMOpts...)

	var recallClient *llm.Client
	if cfg.Models.Recall.Model != "" && (len(sharedLLMOpts) > 0 || cfg.Models.Recall.UsesCodexAuth()) {
		recallLLMOpts := append([]llm.ClientOption{}, sharedLLMOpts...)
		if cfg.Models.Recall.UsesCodexAuth() {
			recallLLMOpts = append(recallLLMOpts, llm.WithCodex(codexAuth))
		}
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
	// The sandbox gets nikctl, never nikd. A skill in the container holding
	// this binary would hold one that can open the database and the secret
	// store on its own; holding nikctl leaves it a client of a socket that
	// can refuse it. Missing is not fatal — the sandbox still runs, it just
	// has no nik on its PATH.
	ctlBin, err := daemonctl.SiblingBinary("nikctl")
	if err != nil {
		slog.Warn("nikctl not found, sandbox gets no nik binary", "pkg", "main", "error", err)
	}
	shellSvc := shell.NewService(cfg, conn, ctlBin)

	err = shellSvc.EnsureReady()
	if err != nil {
		fatal("shell setup", err)
	}
	state.Set("shell", true, cfg.Shell.DockerImage)
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

	if _, err := db.GenesisStartedAtEnsure(ctx, conn); err != nil {
		slog.Warn("stamp genesis_started_at", "pkg", "main", "error", err)
	}

	genesisDone := genesis.IsCompleted(ctx, conn)
	if !genesisDone {
		b.RegisterReflex(0, genesis.Reflex(conn))
		slog.Info("genesis mode active", "pkg", "main")
	} else {
		skillSrcs := skills.Sources(cfg.Home)
		b.RegisterReflex(5*time.Minute, skills.SkillChangeReflex(cfg, conn, skillSrcs))
		b.RegisterReflex(5*time.Minute, skills.SkillCheckReflex(cfg, conn, llmClient.Generate, shellSvc.RunCommand, skillSrcs))
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

	if !genesisDone {
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

	brainDone := make(chan struct{})
	go func() {
		state.Set("brain", true, "awake")
		b.Awake(ctx, 2*time.Second)
		state.Set("brain", false, "shutting down")
		close(brainDone)
	}()

	// shutdown

	<-brainDone
	messagingSvc.StopPresence()
	taskRunner.Wait()
	shellSvc.StopContainer()
	<-apiDone
	slog.Info("shutdown complete")
}
