package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/kciuffolo/nik/internal/alarms"
	"github.com/kciuffolo/nik/internal/codex"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/contacts"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/fs"
	"github.com/kciuffolo/nik/internal/llm"
	"github.com/kciuffolo/nik/internal/messaging"
	"github.com/kciuffolo/nik/internal/prompt"
	"github.com/kciuffolo/nik/internal/shell"
	"github.com/kciuffolo/nik/internal/skills"
	"github.com/kciuffolo/nik/internal/task"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: call <tool_name> <json_args>")
		os.Exit(1)
	}

	toolName := os.Args[1]
	jsonArgs := strings.Join(os.Args[2:], " ")

	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	var sharedLLMOpts []llm.ClientOption
	if cfg.OpenAIKey != "" {
		sharedLLMOpts = append(sharedLLMOpts, llm.WithAPIKey(cfg.OpenAIKey))
	}
	if cfg.AnthropicKey != "" {
		sharedLLMOpts = append(sharedLLMOpts, llm.WithAnthropicKey(cfg.AnthropicKey))
	}

	var codexAuth *codex.Auth
	if cfg.Models.NeedsCodexAuth() {
		var err error
		codexAuth, err = codex.LoadOrLogin("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "codex auth error: %v\n", err)
			os.Exit(1)
		}
	}

	mainLLMOpts := append([]llm.ClientOption{}, sharedLLMOpts...)
	if cfg.Models.Main.UsesCodexAuth() {
		mainLLMOpts = append(mainLLMOpts, llm.WithCodex(codexAuth))
	}
	mainLLMOpts = append(mainLLMOpts, llm.WithReasoningEffort(&cfg.Models.Main.ReasoningEffort))
	mainLLMOpts = append(mainLLMOpts, llm.WithVerbosity(&cfg.Models.Main.Verbosity))
	llmClient := llm.NewClient(&cfg.Models.Main.Model, mainLLMOpts...)

	conn, err := db.Open(cfg.DBPath(), cfg.TZ())
	if err != nil {
		conn = nil
	}
	if conn != nil {
		defer conn.Close()
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
	}

	tools := buildTools(cfg, llmClient, taskLLMClient, conn)

	handler, ok := tools[toolName]
	if !ok {
		var names []string
		for name := range tools {
			names = append(names, name)
		}
		sort.Strings(names)
		fmt.Fprintf(os.Stderr, "unknown tool %q\navailable: %s\n", toolName, strings.Join(names, ", "))
		os.Exit(1)
	}

	call := llm.ToolCall{
		CallID:    "manual",
		Name:      toolName,
		Arguments: jsonArgs,
	}

	result, err := handler(context.Background(), call)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(result)
}

func buildTools(cfg *config.Config, llmClient, taskLLMClient *llm.Client, conn *sql.DB) map[string]llm.ToolExecutor {
	tools := map[string]llm.ToolExecutor{}

	if conn != nil {
		roConn, roErr := db.OpenReadOnly(cfg.DBPath(), cfg.TZ())
		if roErr != nil {
			roConn = conn
		} else {
			defer roConn.Close()
		}

		contactsSvc := contacts.NewService(conn)
		msgSvc := messaging.NewService(&config.Config{}, conn, contactsSvc)

		for _, t := range contacts.BuildTools(conn) {
			tools[t.Def.Name] = t.Handler
		}

		for _, t := range llm.BuildTools(llmClient, cfg.Home, msgSvc) {
			tools[t.Def.Name] = t.Handler
		}

		for _, t := range db.BuildTools(roConn, conn, cfg.RetentionOrDefault) {
			tools[t.Def.Name] = t.Handler
		}

		alarmSvc := alarms.New(cfg, conn)
		for _, t := range alarms.BuildTools(alarmSvc) {
			tools[t.Def.Name] = t.Handler
		}

		taskSvc := task.NewService(conn)
		shellSvc := shell.NewService(cfg, conn)
		var taskToolList []llm.Tool
		taskToolList = append(taskToolList, shellSvc.BuildTools()...)
		taskToolList = append(taskToolList, llm.BuildTools(taskLLMClient, cfg.Home, nil)...)
		taskToolList = append(taskToolList, db.BuildTools(roConn, conn, cfg.RetentionOrDefault)...)
		taskToolList = append(taskToolList, fs.BuildTools(cfg.Home)...)
		taskToolList = append(taskToolList, skills.BuildTools(cfg)...)

		pr := prompt.NewRenderer(cfg)
		taskRunner := task.NewRunner(cfg, taskLLMClient, pr, taskSvc, taskToolList)
		for _, t := range task.BuildTools(taskSvc, taskRunner) {
			tools[t.Def.Name] = t.Handler
		}
	}

	for _, t := range shell.NewService(cfg, conn).BuildTools() {
		tools[t.Def.Name] = t.Handler
	}

	for _, t := range fs.BuildTools(cfg.Home) {
		tools[t.Def.Name] = t.Handler
	}

	for _, t := range skills.BuildTools(cfg) {
		tools[t.Def.Name] = t.Handler
	}

	return tools
}
