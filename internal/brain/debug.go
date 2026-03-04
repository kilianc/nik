package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/llm"
)

// DebugInput holds everything the brain knows about a single LLM round.
type DebugInput struct {
	Meta         map[string]string
	Instructions string
	UserInput    string
	RawOutput    string
	Tools        []llm.ToolDef
	ToolCalls    []llm.ToolCallRecord
	Extra        llm.CompletionExtra
	Usage        llm.Usage
	ProcessErr   error
}

// DebugRecorder writes debug output for a single LLM round.
type DebugRecorder func(DebugInput)

func (b *Brain) SetDebugRecorder(fn DebugRecorder) {
	b.debugRecorder = fn
}

// DebugTaskInfo holds task data for debug output.
type DebugTaskInfo struct {
	ID        string
	Goal      string
	Status    string
	CreatedAt time.Time
}

// DebugTaskQuerier fetches active tasks for a conversation.
type DebugTaskQuerier interface {
	ActiveTasksForConversation(ctx context.Context, conversationID string) ([]DebugTaskInfo, error)
}

// NewDebugRecorder creates a DebugRecorder that writes markdown files.
// All debug state (paths, model, task querier) is captured in the closure.
func NewDebugRecorder(debugPath, model string, now func() time.Time, tasks DebugTaskQuerier) DebugRecorder {
	return func(input DebugInput) {
		if debugPath == "" {
			return
		}

		if now == nil {
			now = time.Now
		}
		timestamp := now()

		debugFile, err := createDebugFilePath(debugPath, input.UserInput, timestamp)
		if err != nil {
			slog.Warn("create debug path failed", "pkg", "brain", "error", err)
			return
		}

		toolNames := make([]string, len(input.Tools))
		for i, t := range input.Tools {
			toolNames[i] = t.Name
		}

		var activeTasks []debugTaskInfo
		if tasks != nil {
			convID := input.Meta["conversation_id"]
			if convID != "" {
				found, qErr := tasks.ActiveTasksForConversation(context.Background(), convID)
				if qErr == nil {
					for _, t := range found {
						activeTasks = append(activeTasks, debugTaskInfo{
							ID:     t.ID,
							Goal:   t.Goal,
							Status: t.Status,
							Age:    time.Since(t.CreatedAt).Truncate(time.Second).String(),
						})
					}
				}
			}
		}

		record := debugRecord{
			Timestamp:       timestamp.Format(time.RFC3339),
			Model:           model,
			ReasoningEffort: input.Extra.ReasoningEffort,
			Trigger:         input.Meta,
			ActiveTasks:     activeTasks,
			Input: debugInput{
				Instructions: input.Instructions,
				UserInput:    input.UserInput,
			},
			Tools:              toolNames,
			ToolCalls:          buildDebugToolCalls(input.ToolCalls),
			ReasoningSummaries: input.Extra.ReasoningSummaries,
			RawResponses:       input.Extra.RawResponses,
			Output:             debugOutput{Raw: input.RawOutput},
			Usage: debugUsage{
				InputTokens:     input.Usage.InputTokens,
				OutputTokens:    input.Usage.OutputTokens,
				TotalTokens:     input.Usage.TotalTokens,
				CachedTokens:    input.Usage.CachedTokens,
				ReasoningTokens: input.Usage.ReasoningTokens,
				CostUSD:         llm.ComputeCost(model, input.Usage.InputTokens, input.Usage.OutputTokens, input.Usage.CachedTokens),
			},
		}

		if input.ProcessErr != nil {
			record.Output.ProcessErr = input.ProcessErr.Error()
		}

		err = writeDebugMarkdown(debugFile, record)
		if err != nil {
			slog.Warn("write debug record failed", "pkg", "brain", "error", err)
		}
	}
}

type debugRecord struct {
	Timestamp          string
	Model              string
	ReasoningEffort    string
	Trigger            map[string]string
	ActiveTasks        []debugTaskInfo
	Input              debugInput
	Tools              []string
	ToolCalls          []debugToolCall
	ReasoningSummaries []string
	RawResponses       []string
	Output             debugOutput
	Usage              debugUsage
}

type debugTaskInfo struct {
	ID     string
	Goal   string
	Status string
	Age    string
}

type debugToolCall struct {
	Name   string
	Args   string
	Result string
	Error  bool
}

type debugInput struct {
	Instructions string
	UserInput    string
}

type debugOutput struct {
	Raw        string
	ProcessErr string
}

type debugUsage struct {
	InputTokens     int64
	OutputTokens    int64
	TotalTokens     int64
	CachedTokens    int64
	ReasoningTokens int64
	CostUSD         float64
}

var nonAlphaNum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func buildDebugToolCalls(calls []llm.ToolCallRecord) []debugToolCall {
	if len(calls) == 0 {
		return nil
	}

	out := make([]debugToolCall, len(calls))
	for i, c := range calls {
		out[i] = debugToolCall{
			Name:   c.Name,
			Args:   c.Args,
			Result: c.Result,
			Error:  c.Error,
		}
	}

	return out
}

func createDebugFilePath(baseDir, userInput string, timestamp time.Time) (string, error) {
	slug := sanitizeSlug(firstMeaningfulLine(userInput), 40)
	if slug == "" {
		slug = "message"
	}

	hourDir := filepath.Join(baseDir, timestamp.Format("2006-01-02"), timestamp.Format("15"))
	err := os.MkdirAll(hourDir, 0o755)
	if err != nil {
		return "", fmt.Errorf("create debug directory %s: %w", hourDir, err)
	}

	fileName := fmt.Sprintf("%s_%s.md", timestamp.Format("0405"), slug)
	return filepath.Join(hourDir, fileName), nil
}

func writeDebugMarkdown(path string, rec debugRecord) error {
	var w strings.Builder

	fmt.Fprintf(&w, "# Session: %s\n\n", rec.Timestamp)

	if rec.ReasoningEffort != "" {
		fmt.Fprintf(&w, "**Model:** %s | **Reasoning:** %s\n\n", rec.Model, rec.ReasoningEffort)
	} else {
		fmt.Fprintf(&w, "**Model:** %s\n\n", rec.Model)
	}

	if source := rec.Trigger["source"]; source != "" {
		detail := rec.Trigger["source_id"]
		if detail != "" {
			fmt.Fprintf(&w, "**Trigger:** %s (`%s`)\n\n", source, detail)
		} else {
			fmt.Fprintf(&w, "**Trigger:** %s\n\n", source)
		}
	}

	writeCostTable(&w, rec.Model, rec.Usage)

	if len(rec.ActiveTasks) > 0 {
		fmt.Fprintf(&w, "\n**Active Tasks:** %d\n", len(rec.ActiveTasks))
		for _, t := range rec.ActiveTasks {
			fmt.Fprintf(&w, "- %s | %s | %s | %s ago\n", t.ID, t.Goal, t.Status, t.Age)
		}
	}

	w.WriteString("\n---\n\n")

	w.WriteString("<details><summary>Instructions (system prompt)</summary>\n\n")
	w.WriteString(preserveNewlines(rec.Input.Instructions))
	w.WriteString("\n\n</details>\n\n---\n\n")

	w.WriteString("<details><summary>User Input</summary>\n\n")
	w.WriteString(preserveNewlines(rec.Input.UserInput))
	w.WriteString("\n\n</details>\n\n---\n\n")

	if len(rec.Tools) > 0 {
		w.WriteString("<details><summary>Tools</summary>\n\n")
		for i, t := range rec.Tools {
			if i > 0 {
				w.WriteString(", ")
			}
			fmt.Fprintf(&w, "`%s`", t)
		}
		w.WriteString("\n\n</details>\n\n---\n\n")
	}

	if len(rec.ToolCalls) > 0 {
		w.WriteString("<details><summary>Tool Calls</summary>\n\n")
		for i, tc := range rec.ToolCalls {
			label := tc.Name
			if tc.Error {
				label += " (error)"
			}

			fmt.Fprintf(&w, "<details><summary>%d. %s</summary>\n\n", i+1, label)
			w.WriteString("**Args:**\n\n")
			writeFencedBlock(&w, tc.Args)
			w.WriteString("**Result:**\n\n")
			writeFencedBlock(&w, tc.Result)
			w.WriteString("</details>\n\n")
		}
		w.WriteString("</details>\n\n---\n\n")
	}

	if len(rec.ReasoningSummaries) > 0 {
		w.WriteString("<details><summary>Reasoning</summary>\n\n")
		for _, s := range rec.ReasoningSummaries {
			w.WriteString(s)
			w.WriteString("\n\n")
		}
		w.WriteString("</details>\n\n---\n\n")
	}

	w.WriteString("<details><summary>Output</summary>\n\n")
	writeOutputSection(&w, rec.Output)
	w.WriteString("</details>\n")

	if len(rec.RawResponses) > 0 {
		w.WriteString("\n---\n\n")
		w.WriteString("<details><summary>Raw API Responses</summary>\n\n")
		for i, raw := range rec.RawResponses {
			fmt.Fprintf(&w, "### Round %d\n\n", i)
			writeFencedBlock(&w, raw)
		}
		w.WriteString("</details>\n")
	}

	err := os.WriteFile(path, []byte(w.String()), 0o644)
	if err != nil {
		return fmt.Errorf("write debug file %s: %w", path, err)
	}

	return nil
}

func writeOutputSection(w *strings.Builder, out debugOutput) {
	if out.Raw != "" {
		w.WriteString("### Thinking\n\n")
		w.WriteString(out.Raw)
		w.WriteString("\n\n")
	}

	if out.ProcessErr != "" {
		w.WriteString("### Process Error\n\n")
		w.WriteString(out.ProcessErr)
		w.WriteString("\n\n")
	}
}

func writeCostTable(w *strings.Builder, model string, u debugUsage) {
	rates, ok := llm.ModelRates(model)

	w.WriteString("| | Tokens | Rate | Cost |\n")
	w.WriteString("|---|---|---|---|\n")

	uncached := u.InputTokens - u.CachedTokens

	if ok {
		inputCost := float64(uncached) * rates.Input / 1e6
		fmt.Fprintf(w, "| Input | %d | $%.2f/M | $%.4f |\n", uncached, rates.Input, inputCost)

		if u.CachedTokens > 0 {
			cachedCost := float64(u.CachedTokens) * rates.Cached / 1e6
			fmt.Fprintf(w, "| Cached | %d | $%.2f/M | $%.4f |\n", u.CachedTokens, rates.Cached, cachedCost)
		}

		outputCost := float64(u.OutputTokens) * rates.Output / 1e6
		fmt.Fprintf(w, "| Output | %d | $%.2f/M | $%.4f |\n", u.OutputTokens, rates.Output, outputCost)

		if u.ReasoningTokens > 0 {
			fmt.Fprintf(w, "| ↳ Reasoning | %d | | |\n", u.ReasoningTokens)
		}
	} else {
		fmt.Fprintf(w, "| Input | %d | - | - |\n", uncached)
		if u.CachedTokens > 0 {
			fmt.Fprintf(w, "| Cached | %d | - | - |\n", u.CachedTokens)
		}
		fmt.Fprintf(w, "| Output | %d | - | - |\n", u.OutputTokens)

		if u.ReasoningTokens > 0 {
			fmt.Fprintf(w, "| ↳ Reasoning | %d | | |\n", u.ReasoningTokens)
		}
	}

	fmt.Fprintf(w, "| **Total** | **%d** | | **$%.4f** |\n", u.TotalTokens, u.CostUSD)

	ctxWindow, ctxOk := llm.ModelContextWindow(model)
	if ctxOk && u.InputTokens > 0 {
		pct := float64(u.InputTokens) / float64(ctxWindow) * 100
		fmt.Fprintf(w, "\n**Context:** %d / %d tokens (%.1f%%)\n", u.InputTokens, ctxWindow, pct)
	}
}

func writeFencedBlock(w *strings.Builder, s string) {
	if s == "" {
		w.WriteString("```\n(empty)\n```\n\n")
		return
	}

	var parsed any
	err := json.Unmarshal([]byte(s), &parsed)
	if err != nil {
		w.WriteString("```\n")
		w.WriteString(s)
		w.WriteString("\n```\n\n")
		return
	}

	formatted, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		w.WriteString("```\n")
		w.WriteString(s)
		w.WriteString("\n```\n\n")
		return
	}

	w.WriteString("```json\n")
	w.Write(formatted)
	w.WriteString("\n```\n\n")
}

// preserveNewlines doubles single newlines between non-blank lines so
// markdown renderers treat each line as a separate paragraph.
func preserveNewlines(s string) string {
	lines := strings.Split(s, "\n")
	var out strings.Builder

	for i, line := range lines {
		out.WriteString(line)
		if i >= len(lines)-1 {
			break
		}

		next := lines[i+1]
		if line == "" || next == "" {
			out.WriteString("\n")
		} else {
			out.WriteString("\n\n")
		}
	}

	return out.String()
}

func firstMeaningfulLine(input string) string {
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}

	return ""
}

func sanitizeSlug(s string, maxLen int) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonAlphaNum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")

	if len(s) > maxLen {
		s = s[:maxLen]
		lastDash := strings.LastIndex(s, "-")
		if lastDash > 0 {
			s = s[:lastDash]
		}
	}

	return s
}
