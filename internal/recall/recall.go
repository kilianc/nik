package recall

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/llm"
)

const (
	recallTimeout = 30 * time.Second
	charsPerToken = 4
)

type Service struct {
	cfg    *config.Config
	client llm.Completer
}

func NewService(cfg *config.Config, client llm.Completer) *Service {
	return &Service{cfg: cfg, client: client}
}

func (s *Service) Recall(ctx context.Context, stimulus string) string {
	if s.client == nil {
		return ""
	}

	data, err := readMemories(s.cfg.Home)
	if err != nil && !os.IsNotExist(err) {
		slog.Warn("recall read memories file", "pkg", "recall", "err", err)
	}

	memories := string(data)
	if memories == "" {
		return ""
	}

	numbered, header, rows := numberRows(memories)
	if len(rows) == 0 {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, recallTimeout)
	defer cancel()

	slog.Info("recall starting", "pkg", "recall", "memories_chars", len(memories), "rows", len(rows))

	instructions := buildRecallPrompt(stimulus)

	_, ch := s.client.Complete(ctx, instructions, llm.StaticInput(numbered), nil, nil)
	result := <-ch

	if result.Err != nil {
		slog.Warn("recall failed", "pkg", "recall", "err", result.Err)
		return ""
	}

	ids := parseSelectedIDs(result.Output, len(rows))

	slog.Info("recall completed",
		"pkg", "recall",
		"rows", len(rows),
		"selected", len(ids),
		"input_tokens", result.Usage.InputTokens,
		"output_tokens", result.Usage.OutputTokens,
	)

	if len(ids) == 0 {
		return ""
	}

	selected := append([]string{}, header...)
	for _, id := range ids {
		selected = append(selected, rows[id-1])
	}
	return strings.Join(selected, "\n")
}

func buildRecallPrompt(stimulus string) string {
	return `The input is a numbered list of memories (facts about people, preferences, events, decisions).
Return ONLY the row numbers relevant to this conversation as a comma-separated list.
If nothing is relevant, return: nil

Stimulus:
` + stimulus
}

// numberRows parses a markdown table, collecting the header (everything up to
// and including the separator line), and returns a numbered version for the LLM
// plus the original header and data rows for reassembly. Row IDs are 1-based.
func numberRows(memories string) (numbered string, header []string, rows []string) {
	lines := strings.Split(strings.TrimSpace(memories), "\n")

	pastSeparator := false
	var b strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if !pastSeparator {
			header = append(header, line)
			if isSeparator(trimmed) {
				pastSeparator = true
			}
			continue
		}

		rows = append(rows, line)
		fmt.Fprintf(&b, "%d: %s\n", len(rows), trimmed)
	}

	return b.String(), header, rows
}

func isSeparator(line string) bool {
	if !strings.HasPrefix(line, "|") {
		return false
	}

	inner := strings.Trim(line, "| ")
	if inner == "" {
		return false
	}
	for _, c := range inner {
		if c != '-' && c != '|' && c != ' ' {
			return false
		}
	}
	return true
}

// parseSelectedIDs extracts integers from a comma-separated LLM output,
// discarding out-of-range values and non-numeric tokens.
func parseSelectedIDs(output string, maxID int) []int {
	output = strings.TrimSpace(output)
	if output == "" || output == "nil" {
		return nil
	}

	parts := strings.Split(output, ",")
	seen := make(map[int]bool, len(parts))
	var ids []int

	for _, p := range parts {
		p = strings.TrimSpace(p)
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > maxID {
			continue
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		ids = append(ids, n)
	}

	return ids
}

func tokenEstimate(s string) int {
	return len(s) / charsPerToken
}

func readMemories(home string) ([]byte, error) {
	root, err := os.OpenRoot(home)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	f, err := root.Open("memories/latest.md")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return io.ReadAll(f)
}
