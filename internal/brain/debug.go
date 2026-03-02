package brain

import (
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

type debugRecord struct {
	Timestamp string
	Model     string
	Trigger   map[string]string
	Input     debugInput
	Tools     []string
	ToolCalls []debugToolCall
	Output    debugOutput
	Usage     debugUsage
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
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	CachedTokens int64
	CostUSD      float64
}

var nonAlphaNum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func (b *Brain) writeDebugRecord(meta map[string]string, instructions, userInput, rawOutput string, tools []llm.ToolDef, toolCalls []llm.ToolCallRecord, usage llm.Usage, processErr error) {
	if b.cfg.DebugPath() == "" {
		return
	}

	now := b.now
	if now == nil {
		now = time.Now
	}
	timestamp := now()

	debugFile, err := createDebugFilePath(b.cfg.DebugPath(), userInput, timestamp)
	if err != nil {
		slog.Warn("create debug path failed", "pkg", "brain", "error", err)
		return
	}

	toolNames := make([]string, len(tools))
	for i, t := range tools {
		toolNames[i] = t.Name
	}

	record := debugRecord{
		Timestamp: timestamp.Format(time.RFC3339),
		Model:     b.llm.Model(),
		Trigger:   meta,
		Input: debugInput{
			Instructions: instructions,
			UserInput:    userInput,
		},
		Tools:     toolNames,
		ToolCalls: buildDebugToolCalls(toolCalls),
		Output:    debugOutput{Raw: rawOutput},
		Usage: debugUsage{
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
			TotalTokens:  usage.TotalTokens,
			CachedTokens: usage.CachedTokens,
			CostUSD:      llm.ComputeCost(b.llm.Model(), usage.InputTokens, usage.OutputTokens, usage.CachedTokens),
		},
	}

	if processErr != nil {
		record.Output.ProcessErr = processErr.Error()
	}

	err = writeDebugMarkdown(debugFile, record)
	if err != nil {
		slog.Warn("write debug record failed", "pkg", "brain", "error", err)
	}
}

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
	fmt.Fprintf(&w, "**Model:** %s\n\n", rec.Model)

	if source := rec.Trigger["source"]; source != "" {
		detail := rec.Trigger["source_id"]
		if detail != "" {
			fmt.Fprintf(&w, "**Trigger:** %s (`%s`)\n\n", source, detail)
		} else {
			fmt.Fprintf(&w, "**Trigger:** %s\n\n", source)
		}
	}

	writeCostTable(&w, rec.Model, rec.Usage)
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
		w.WriteString("<details open><summary>Tool Calls</summary>\n\n")
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

	w.WriteString("<details open><summary>Output</summary>\n\n")
	writeOutputSection(&w, rec.Output)
	w.WriteString("</details>\n")

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
	} else {
		fmt.Fprintf(w, "| Input | %d | - | - |\n", uncached)
		if u.CachedTokens > 0 {
			fmt.Fprintf(w, "| Cached | %d | - | - |\n", u.CachedTokens)
		}
		fmt.Fprintf(w, "| Output | %d | - | - |\n", u.OutputTokens)
	}

	fmt.Fprintf(w, "| **Total** | **%d** | | **$%.4f** |\n", u.TotalTokens, u.CostUSD)
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
