package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/kciuffolo/nik/internal/alarms"
	"github.com/kciuffolo/nik/internal/briefing"
	"github.com/kciuffolo/nik/internal/codex"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/contacts"
	"github.com/kciuffolo/nik/internal/crew"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/dream"
	"github.com/kciuffolo/nik/internal/llm"
	"github.com/kciuffolo/nik/internal/memory"
	"github.com/kciuffolo/nik/internal/messaging"
	"github.com/kciuffolo/nik/internal/search"
	"github.com/kciuffolo/nik/internal/shell"
	"github.com/kciuffolo/nik/internal/skills"
	"github.com/kciuffolo/nik/internal/task"
	"github.com/kciuffolo/nik/internal/websearch"
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
	}
	llmOpts = append(llmOpts, llm.WithReasoningEffort(&cfg.ReasoningEffort))
	llmClient := llm.NewClient(cfg.Model, llmOpts...)

	conn, err := db.Open(cfg.DBPath())
	if err != nil {
		conn = nil
	}
	if conn != nil {
		defer conn.Close()
	}

	tools := buildTools(cfg, llmClient, conn)

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

func buildTools(cfg *config.Config, llmClient *llm.Client, conn *sql.DB) map[string]llm.ToolExecutor {
	tools := map[string]llm.ToolExecutor{}

	for _, t := range llm.BuildTools(llmClient, cfg.Home) {
		tools[t.Def.Name] = t.Handler
	}

	if conn != nil {
		contactsSvc := contacts.NewService(conn)
		msgSvc := messaging.NewService(&config.Config{}, conn, contactsSvc)
		searchSvc := search.NewService(conn)

		for _, t := range contacts.BuildTools(conn) {
			tools[t.Def.Name] = t.Handler
		}

		for _, t := range messaging.BuildTools(msgSvc) {
			if t.Def.Name != "message_update_media_description" {
				continue
			}

			tools[t.Def.Name] = t.Handler
		}

		for _, t := range search.BuildTools(conn, searchSvc) {
			tools[t.Def.Name] = t.Handler
		}

		memorySvc := memory.NewService(conn, llmClient)
		for _, t := range memory.BuildTools(memorySvc) {
			tools[t.Def.Name] = t.Handler
		}

		dreamSvc := dream.NewService(conn, cfg)
		for _, t := range dream.BuildTools(dreamSvc) {
			tools[t.Def.Name] = t.Handler
		}

		briefingSvc := briefing.NewService(conn, cfg)
		for _, t := range briefing.BuildTools(briefingSvc) {
			tools[t.Def.Name] = t.Handler
		}

		alarmSvc := alarms.New(conn)
		for _, t := range alarms.BuildTools(alarmSvc) {
			tools[t.Def.Name] = t.Handler
		}

		taskSvc := task.NewService(conn)
		searchSvcT := search.NewService(conn)
		var taskToolList []llm.Tool
		taskToolList = append(taskToolList, shell.BuildTools(cfg)...)
		taskToolList = append(taskToolList, websearch.BuildTools(cfg)...)
		taskToolList = append(taskToolList, llm.BuildTools(llmClient, cfg.Home)...)
		taskToolList = append(taskToolList, search.BuildTools(conn, searchSvcT)...)
		taskToolList = append(taskToolList, memory.BuildReadTools(memorySvc)...)
		taskToolList = append(taskToolList, skills.BuildTools(cfg)...)

		taskToolDefs := make([]llm.ToolDef, len(taskToolList))
		taskHandlers := make(map[string]llm.ToolExecutor, len(taskToolList))
		for i, t := range taskToolList {
			taskToolDefs[i] = t.Def
			taskHandlers[t.Def.Name] = t.Handler
		}
		taskExec := func(ctx context.Context, call llm.ToolCall) (string, error) {
			h, ok := taskHandlers[call.Name]
			if !ok {
				return llm.ToolErrorf("unknown tool %q", call.Name), nil
			}
			return h(ctx, call)
		}

		crewSvc := crew.NewService(conn)
		for _, t := range crew.BuildTools(crewSvc) {
			tools[t.Def.Name] = t.Handler
		}

		taskRunner := task.NewRunner(cfg, llmClient, taskSvc, conn, taskToolDefs, taskExec)
		for _, t := range task.BuildTools(taskSvc, taskRunner, crewSvc) {
			tools[t.Def.Name] = t.Handler
		}
	}

	for _, t := range shell.BuildTools(cfg) {
		tools[t.Def.Name] = t.Handler
	}

	for _, t := range websearch.BuildTools(cfg) {
		tools[t.Def.Name] = t.Handler
	}

	for _, t := range skills.BuildTools(cfg) {
		tools[t.Def.Name] = t.Handler
	}

	return tools
}
