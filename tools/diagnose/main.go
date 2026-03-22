package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/alarms"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/contacts"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/fs"
	"github.com/kciuffolo/nik/internal/llm"
	"github.com/kciuffolo/nik/internal/messaging"
	"github.com/kciuffolo/nik/internal/shell"
	"github.com/kciuffolo/nik/internal/skills"
	"github.com/kciuffolo/nik/internal/task"
)

func main() {
	home := flag.String("home", "workspace", "nik home directory")
	message := flag.String("message", "", "message body text to search for")
	msgID := flag.String("id", "", "message ID")
	window := flag.Duration("window", 60*time.Second, "time window around message for activation search")
	flag.Parse()

	if *message == "" && *msgID == "" {
		fmt.Fprintln(os.Stderr, "usage: diagnose -message '<text>' | -id <message_id> [-home workspace] [-window 60s]")
		os.Exit(1)
	}

	cfg, err := config.Load(*home)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	conn, err := db.OpenReadOnly(cfg.DBPath(), cfg.TZ())
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	ctx := context.Background()

	msg, err := findMessage(ctx, conn, *message, *msgID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "find message: %v\n", err)
		os.Exit(1)
	}

	conv, err := loadConversation(ctx, conn, msg.ConversationID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load conversation: %v\n", err)
		os.Exit(1)
	}

	contactName := resolveContact(ctx, conn, msg.ContactID)

	activations, err := loadActivations(ctx, conn, msg.ConversationID, msg.SentAt, *window)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load activations: %v\n", err)
		os.Exit(1)
	}

	for i := range activations {
		activations[i].Rounds, err = loadRounds(ctx, conn, activations[i].ID, msg.Body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load rounds for %s: %v\n", activations[i].ID, err)
			os.Exit(1)
		}
	}

	diagnosis := classify(msg, activations)

	logLines := parseLogs(filepath.Join(*home, "nik.log"), msg.SentAt, *window, msg.ConversationID)

	toolDefs := buildAllToolDefs(cfg, conn)

	caseDir, err := writeCase(*home, msg, conv, contactName, activations, diagnosis, logLines, toolDefs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "write case: %v\n", err)
		os.Exit(1)
	}

	printReport(msg, conv, contactName, activations, logLines, diagnosis, caseDir)
}

type messageRow struct {
	ID             string
	ConversationID string
	ContactID      string
	Body           string
	SentAt         time.Time
	IsFromMe       bool
	Kind           string
}

type conversationRow struct {
	ID         string
	Title      sql.NullString
	Kind       string
	LastReadAt sql.NullString
}

type activationRow struct {
	ID            string
	Model         string
	ToolCallCount int
	DurationMS    int
	Error         string
	CreatedAt     string
	Tools         string
	Instructions  string
	Rounds        []roundInfo
}

type roundInfo struct {
	ID             string
	Round          int
	UserInput      string
	ModelOutput    string
	Reasoning      string
	MessagePresent bool
	MessageInNew   bool
	ToolCalls      []toolCallInfo
}

type toolCallInfo struct {
	Name   string
	Input  string
	Output string
	Error  int
}

type diagnosisResult struct {
	Category     string
	Summary      string
	ActivationID string
	Round        int
}

func findMessage(ctx context.Context, conn *sql.DB, body, id string) (messageRow, error) {
	var row messageRow

	var query string
	var arg string
	if id != "" {
		query = `SELECT id, conversation_id, contact_id, body, sent_at, is_from_me, kind
		         FROM message WHERE id = ?1`
		arg = id
	} else {
		query = `SELECT id, conversation_id, contact_id, body, sent_at, is_from_me, kind
		         FROM message WHERE body LIKE '%' || ?1 || '%'
		         ORDER BY sent_at DESC LIMIT 1`
		arg = body
	}

	var sentAt string
	var isFromMe int
	err := conn.QueryRowContext(ctx, query, arg).Scan(
		&row.ID, &row.ConversationID, &row.ContactID,
		&row.Body, &sentAt, &isFromMe, &row.Kind,
	)
	if err != nil {
		return row, fmt.Errorf("query: %w", err)
	}

	row.SentAt, err = time.Parse(time.RFC3339Nano, sentAt)
	if err != nil {
		row.SentAt, _ = time.Parse("2006-01-02T15:04:05.000Z", sentAt)
	}
	row.IsFromMe = isFromMe == 1

	return row, nil
}

func loadConversation(ctx context.Context, conn *sql.DB, id string) (conversationRow, error) {
	var row conversationRow
	err := conn.QueryRowContext(ctx,
		`SELECT id, title, kind, last_read_at FROM conversation WHERE id = ?1`, id,
	).Scan(&row.ID, &row.Title, &row.Kind, &row.LastReadAt)
	return row, err
}

func resolveContact(ctx context.Context, conn *sql.DB, contactID string) string {
	var name string
	err := conn.QueryRowContext(ctx,
		`SELECT name FROM contact WHERE id = ?1`, contactID,
	).Scan(&name)
	if err != nil {
		return "unknown"
	}
	return name
}

func loadActivations(ctx context.Context, conn *sql.DB, convID string, sentAt time.Time, window time.Duration) ([]activationRow, error) {
	start := sentAt.Add(-window).UTC().Format("2006-01-02T15:04:05.000Z")
	end := sentAt.Add(window).UTC().Format("2006-01-02T15:04:05.000Z")

	rows, err := conn.QueryContext(ctx,
		`SELECT id, model, tool_call_count, duration_ms, error, created_at, tools, instructions
		 FROM activation
		 WHERE conversation_id = ?1 AND task_id IS NULL
		   AND created_at >= ?2 AND created_at <= ?3
		 ORDER BY created_at`, convID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []activationRow
	for rows.Next() {
		var a activationRow
		err = rows.Scan(&a.ID, &a.Model, &a.ToolCallCount, &a.DurationMS, &a.Error, &a.CreatedAt, &a.Tools, &a.Instructions)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}

	return result, rows.Err()
}

func loadRounds(ctx context.Context, conn *sql.DB, activationID, messageBody string) ([]roundInfo, error) {
	rows, err := conn.QueryContext(ctx,
		`SELECT id, round, user_input, model_output, reasoning_summaries
		 FROM activation_round
		 WHERE activation_id = ?1
		 ORDER BY round`, activationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []roundInfo
	for rows.Next() {
		var r roundInfo
		err = rows.Scan(&r.ID, &r.Round, &r.UserInput, &r.ModelOutput, &r.Reasoning)
		if err != nil {
			return nil, err
		}

		if messageBody != "" {
			r.MessagePresent = strings.Contains(r.UserInput, messageBody)
			if r.MessagePresent {
				newIdx := strings.Index(r.UserInput, "### New")
				if newIdx >= 0 {
					r.MessageInNew = strings.Contains(r.UserInput[newIdx:], messageBody)
				}
			}
		}

		r.ToolCalls, err = loadToolCalls(ctx, conn, r.ID)
		if err != nil {
			return nil, err
		}

		result = append(result, r)
	}

	return result, rows.Err()
}

func loadToolCalls(ctx context.Context, conn *sql.DB, roundID string) ([]toolCallInfo, error) {
	rows, err := conn.QueryContext(ctx,
		`SELECT name, input, output, error
		 FROM tool_call
		 WHERE activation_round_id = ?1
		 ORDER BY created_at`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []toolCallInfo
	for rows.Next() {
		var tc toolCallInfo
		err = rows.Scan(&tc.Name, &tc.Input, &tc.Output, &tc.Error)
		if err != nil {
			return nil, err
		}
		result = append(result, tc)
	}

	return result, rows.Err()
}

var terminalTools = map[string]bool{
	"message_send":  true,
	"message_noop":  true,
	"message_react": true,
}

func classify(msg messageRow, activations []activationRow) diagnosisResult {
	if len(activations) == 0 {
		return diagnosisResult{
			Category: "NO_ACTIVATION",
			Summary:  "No activation fired for the conversation in the time window.",
		}
	}

	for _, a := range activations {
		if a.Error != "" {
			return diagnosisResult{
				Category:     "ACTIVATION_FAILED",
				Summary:      fmt.Sprintf("Activation %s failed: %s", shortID(a.ID), a.Error),
				ActivationID: a.ID,
			}
		}

		for _, r := range a.Rounds {
			for _, tc := range r.ToolCalls {
				if tc.Error != 0 {
					return diagnosisResult{
						Category:     "TOOL_FAILED",
						Summary:      fmt.Sprintf("Tool %s failed in activation %s round %d: %s", tc.Name, shortID(a.ID), r.Round, truncate(tc.Output, 120)),
						ActivationID: a.ID,
						Round:        r.Round,
					}
				}
			}
		}
	}

	for _, a := range activations {
		for _, r := range a.Rounds {
			if !r.MessageInNew {
				continue
			}

			hasTerminal := false
			terminalName := ""
			for _, tc := range r.ToolCalls {
				if terminalTools[tc.Name] {
					hasTerminal = true
					terminalName = tc.Name
					break
				}
			}

			if hasTerminal {
				extra := ""
				if terminalName == "message_react" {
					hasReply := false
					hasTask := false
					for _, rr := range a.Rounds {
						for _, tc := range rr.ToolCalls {
							if tc.Name == "message_send" {
								hasReply = true
							}
							if tc.Name == "task_spawn" {
								hasTask = true
							}
						}
					}
					if !hasReply && !hasTask {
						extra = " (react only, no reply or task)"
					}
				}
				return diagnosisResult{
					Category:     "SURFACED_HANDLED",
					Summary:      fmt.Sprintf("Message in ### New, terminal tool: %s%s", terminalName, extra),
					ActivationID: a.ID,
					Round:        r.Round,
				}
			}

			if len(r.ToolCalls) == 0 {
				return diagnosisResult{
					Category:     "SURFACED_NOT_ACTED",
					Summary:      fmt.Sprintf("Message in ### New in activation %s round %d, but LLM produced no tool calls.", shortID(a.ID), r.Round),
					ActivationID: a.ID,
					Round:        r.Round,
				}
			}
		}
	}

	surfaced := false
	for _, a := range activations {
		for _, r := range a.Rounds {
			if r.MessagePresent {
				surfaced = true
				break
			}
		}
	}

	if surfaced {
		return diagnosisResult{
			Category: "SURFACED_HANDLED",
			Summary:  "Message present in timeline but under Already handled.",
		}
	}

	return diagnosisResult{
		Category: "NOT_SURFACED",
		Summary:  "Activation(s) fired but message never appeared in any round's user_input.",
	}
}

func parseLogs(logPath string, sentAt time.Time, window time.Duration, convID string) []string {
	f, err := os.Open(logPath)
	if err != nil {
		return []string{fmt.Sprintf("(could not open log: %v)", err)}
	}
	defer f.Close()

	start := sentAt.Add(-window)
	end := sentAt.Add(window)

	shortConv := shortID(convID)

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		ts, ok := parseLogTime(line)
		if !ok {
			continue
		}

		if ts.Before(start) {
			continue
		}
		if ts.After(end) {
			break
		}

		isRelevant := strings.Contains(line, shortConv) ||
			strings.Contains(line, "level=ERROR") ||
			strings.Contains(line, "level=WARN")

		if isRelevant {
			lines = append(lines, line)
		}
	}

	return lines
}

func parseLogTime(line string) (time.Time, bool) {
	idx := strings.Index(line, "time=")
	if idx < 0 {
		return time.Time{}, false
	}

	rest := line[idx+5:]
	end := strings.IndexByte(rest, ' ')
	if end < 0 {
		end = len(rest)
	}
	raw := rest[:end]

	t, err := time.Parse("2006-01-02T15:04:05.000-07:00", raw)
	if err != nil {
		t, err = time.Parse(time.RFC3339Nano, raw)
	}
	if err != nil {
		return time.Time{}, false
	}

	return t, true
}

type caseJSON struct {
	Message      caseMessage      `json:"message"`
	Conversation caseConversation `json:"conversation"`
	Activations  []caseActivation `json:"activations"`
	Diagnosis    caseDiagnosis    `json:"diagnosis"`
}

type caseMessage struct {
	ID       string `json:"id"`
	Body     string `json:"body"`
	SentAt   string `json:"sent_at"`
	Contact  string `json:"contact"`
	IsFromMe bool   `json:"is_from_me"`
	Kind     string `json:"kind"`
}

type caseConversation struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	LastReadAt string `json:"last_read_at"`
}

type caseActivation struct {
	ID         string      `json:"id"`
	Model      string      `json:"model"`
	Tools      []string    `json:"tools"`
	CreatedAt  string      `json:"created_at"`
	DurationMS int         `json:"duration_ms"`
	Error      string      `json:"error,omitempty"`
	Rounds     []caseRound `json:"rounds"`
}

type caseRound struct {
	Round          int            `json:"round"`
	InputFile      string         `json:"input_file"`
	MessagePresent bool           `json:"message_present"`
	MessageInNew   bool           `json:"message_in_new"`
	ToolCalls      []caseToolCall `json:"tool_calls"`
}

type caseToolCall struct {
	Name   string `json:"name"`
	Input  string `json:"input"`
	Output string `json:"output"`
	Error  int    `json:"error,omitempty"`
}

type caseDiagnosis struct {
	Category     string `json:"category"`
	Summary      string `json:"summary"`
	ActivationID string `json:"activation_id,omitempty"`
	Round        int    `json:"round,omitempty"`
}

type toolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

func buildAllToolDefs(cfg *config.Config, conn *sql.DB) []toolSchema {
	var defs []toolSchema

	collect := func(tools []llm.Tool) {
		for _, t := range tools {
			defs = append(defs, toolSchema{
				Name:        t.Def.Name,
				Description: t.Def.Description,
				Parameters:  t.Def.Parameters,
			})
		}
	}

	collect(contacts.BuildTools(conn))
	collect(db.BuildTools(conn))
	collect(alarms.BuildTools(alarms.New(cfg, conn)))

	msgSvc := messaging.NewService(cfg, conn, contacts.NewService(conn))
	collect(messaging.BuildTools(msgSvc))

	taskSvc := task.NewService(conn)
	collect(task.BuildTools(taskSvc, nil))

	shellSvc := shell.NewService(cfg, conn)
	collect(shellSvc.BuildTools())

	collect(fs.BuildTools(cfg.Home))
	collect(skills.BuildTools(cfg))
	collect(config.BuildTools(cfg, conn))

	model := ""
	llmClient := llm.NewClient(&model)
	collect(llm.BuildTools(llmClient, cfg.Home, nil))

	return defs
}

func filterToolDefs(defs []toolSchema, names []string) []toolSchema {
	nameSet := map[string]bool{}
	for _, n := range names {
		nameSet[n] = true
	}

	var filtered []toolSchema
	for _, d := range defs {
		if nameSet[d.Name] {
			filtered = append(filtered, d)
		}
	}

	return filtered
}

func writeCase(home string, msg messageRow, conv conversationRow, contactName string, activations []activationRow, diagnosis diagnosisResult, logLines []string, allToolDefs []toolSchema) (string, error) {
	caseID := shortID(msg.ID)
	caseDir := filepath.Join(home, "cases", caseID)

	err := os.MkdirAll(caseDir, 0o755)
	if err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}

	title := ""
	if conv.Title.Valid {
		title = conv.Title.String
	}
	lastRead := ""
	if conv.LastReadAt.Valid {
		lastRead = conv.LastReadAt.String
	}

	cj := caseJSON{
		Message: caseMessage{
			ID:       msg.ID,
			Body:     msg.Body,
			SentAt:   msg.SentAt.UTC().Format("2006-01-02T15:04:05.000Z"),
			Contact:  contactName,
			IsFromMe: msg.IsFromMe,
			Kind:     msg.Kind,
		},
		Conversation: caseConversation{
			ID:         conv.ID,
			Title:      title,
			Kind:       conv.Kind,
			LastReadAt: lastRead,
		},
		Diagnosis: caseDiagnosis{
			Category:     diagnosis.Category,
			Summary:      diagnosis.Summary,
			ActivationID: diagnosis.ActivationID,
			Round:        diagnosis.Round,
		},
	}

	for _, a := range activations {
		var tools []string
		_ = json.Unmarshal([]byte(a.Tools), &tools)

		ca := caseActivation{
			ID:         a.ID,
			Model:      a.Model,
			Tools:      tools,
			CreatedAt:  a.CreatedAt,
			DurationMS: a.DurationMS,
			Error:      a.Error,
		}

		for _, r := range a.Rounds {
			inputFile := fmt.Sprintf("round_%d_input.txt", r.Round)
			if len(activations) > 1 {
				inputFile = fmt.Sprintf("%s_round_%d_input.txt", shortID(a.ID), r.Round)
			}

			cr := caseRound{
				Round:          r.Round,
				InputFile:      inputFile,
				MessagePresent: r.MessagePresent,
				MessageInNew:   r.MessageInNew,
			}

			for _, tc := range r.ToolCalls {
				cr.ToolCalls = append(cr.ToolCalls, caseToolCall{
					Name:   tc.Name,
					Input:  tc.Input,
					Output: tc.Output,
					Error:  tc.Error,
				})
			}

			ca.Rounds = append(ca.Rounds, cr)

			if r.UserInput != "" {
				err = os.WriteFile(filepath.Join(caseDir, inputFile), []byte(r.UserInput), 0o644)
				if err != nil {
					return "", fmt.Errorf("write %s: %w", inputFile, err)
				}
			}
		}

		cj.Activations = append(cj.Activations, ca)
	}

	if len(activations) == 1 && activations[0].Instructions != "" {
		err = os.WriteFile(filepath.Join(caseDir, "instructions.txt"), []byte(activations[0].Instructions), 0o644)
		if err != nil {
			return "", fmt.Errorf("write instructions: %w", err)
		}
	} else {
		for _, a := range activations {
			if a.Instructions == "" {
				continue
			}
			fname := fmt.Sprintf("%s_instructions.txt", shortID(a.ID))
			err = os.WriteFile(filepath.Join(caseDir, fname), []byte(a.Instructions), 0o644)
			if err != nil {
				return "", fmt.Errorf("write %s: %w", fname, err)
			}
		}
	}

	if len(logLines) > 0 {
		err = os.WriteFile(filepath.Join(caseDir, "logs.txt"), []byte(strings.Join(logLines, "\n")+"\n"), 0o644)
		if err != nil {
			return "", fmt.Errorf("write logs: %w", err)
		}
	}

	data, err := json.MarshalIndent(cj, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal case: %w", err)
	}

	err = os.WriteFile(filepath.Join(caseDir, "case.json"), data, 0o644)
	if err != nil {
		return "", fmt.Errorf("write case.json: %w", err)
	}

	var allNames []string
	for _, a := range activations {
		var names []string
		_ = json.Unmarshal([]byte(a.Tools), &names)
		allNames = append(allNames, names...)
	}

	filtered := filterToolDefs(allToolDefs, allNames)
	toolsData, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal tools: %w", err)
	}

	err = os.WriteFile(filepath.Join(caseDir, "tools.json"), toolsData, 0o644)
	if err != nil {
		return "", fmt.Errorf("write tools.json: %w", err)
	}

	return caseDir, nil
}

func printReport(msg messageRow, conv conversationRow, contactName string, activations []activationRow, logLines []string, diagnosis diagnosisResult, caseDir string) {
	fromLabel := contactName
	if msg.IsFromMe {
		fromLabel += " (from me)"
	}

	title := ""
	if conv.Title.Valid {
		title = conv.Title.String
	}

	body := msg.Body
	if len(body) > 80 {
		body = body[:80] + "..."
	}

	fmt.Printf("MESSAGE  %s  %q\n", shortID(msg.ID), body)
	fmt.Printf("  sent_at:  %s\n", msg.SentAt.UTC().Format("2006-01-02T15:04:05.000Z"))
	fmt.Printf("  from:     %s\n", fromLabel)
	fmt.Printf("  conv:     %s (%s, %s)\n", shortID(msg.ConversationID), title, conv.Kind)
	fmt.Println()

	lastRead := "(not set)"
	if conv.LastReadAt.Valid {
		lastRead = conv.LastReadAt.String
		readTime, err := time.Parse("2006-01-02T15:04:05.000Z", conv.LastReadAt.String)
		if err == nil {
			diff := readTime.Sub(msg.SentAt)
			if diff > 0 {
				lastRead += fmt.Sprintf(" (+%s after message)", diff.Round(time.Millisecond))
			} else if diff < 0 {
				lastRead += fmt.Sprintf(" (%s before message)", (-diff).Round(time.Millisecond))
			}
		}
	}

	fmt.Println("CONVERSATION STATE")
	fmt.Printf("  last_read_at: %s\n", lastRead)
	fmt.Println()

	window := msg.SentAt.Add(-60 * time.Second).UTC().Format("15:04:05")
	windowEnd := msg.SentAt.Add(60 * time.Second).UTC().Format("15:04:05")

	fmt.Printf("ACTIVATIONS [%s - %s]\n", window, windowEnd)
	if len(activations) == 0 {
		fmt.Println("  (none)")
	}
	for _, a := range activations {
		ct := a.CreatedAt
		if len(ct) > 19 {
			ct = ct[11:19]
		}
		fmt.Printf("  %s (%s, %dms, %s)\n", shortID(a.ID), ct, a.DurationMS, a.Model)
		if a.Error != "" {
			fmt.Printf("    ERROR: %s\n", truncate(a.Error, 100))
		}

		for _, r := range a.Rounds {
			presence := "message NOT present"
			if r.MessageInNew {
				presence = "message in ### New"
			} else if r.MessagePresent {
				presence = "message in Already handled"
			}

			tools := "(no tool calls)"
			if len(r.ToolCalls) > 0 {
				var names []string
				for _, tc := range r.ToolCalls {
					label := tc.Name
					if tc.Error != 0 {
						label += " [ERROR]"
					}
					names = append(names, label)
				}
				tools = strings.Join(names, ", ")
			}

			fmt.Printf("    round %d: %s  ->  %s\n", r.Round, presence, tools)
		}
	}
	fmt.Println()

	if len(logLines) > 0 {
		fmt.Printf("LOGS [%s - %s]\n", window, windowEnd)
		for _, line := range logLines {
			if len(line) > 160 {
				line = line[:160] + "..."
			}
			fmt.Printf("  %s\n", line)
		}
		fmt.Println()
	}

	fmt.Printf("DIAGNOSIS: %s\n", diagnosis.Category)
	fmt.Printf("  %s\n", diagnosis.Summary)
	fmt.Println()

	fmt.Printf("CASE: %s/\n", caseDir)
	if diagnosis.ActivationID != "" {
		fmt.Printf("  -> make replay ARGS=\"-case %s -round %d\"\n", caseDir, diagnosis.Round)
	}
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[len(id)-12:]
	}
	return id
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
