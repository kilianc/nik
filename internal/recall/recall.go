package recall

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/llm"
)

const (
	recallTimeout    = 30 * time.Second
	maxContextTokens = 800_000
	charsPerToken    = 4
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

	data, err := os.ReadFile(s.cfg.MemoriesPath())
	if err != nil && !os.IsNotExist(err) {
		slog.Warn("recall read memories file", "pkg", "recall", "err", err)
	}

	memories := string(data)
	if memories == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, recallTimeout)
	defer cancel()

	slog.Info("recall starting", "pkg", "recall", "memories_chars", len(memories))

	result := s.recallRecursive(ctx, stimulus, memories, 0)

	if result == "" || result == "nil" {
		return ""
	}

	return result
}

const maxRecursionDepth = 3

func (s *Service) recallRecursive(ctx context.Context, stimulus, historyBlock string, depth int) string {
	if depth > maxRecursionDepth {
		slog.Warn("recall max recursion depth reached", "pkg", "recall", "depth", depth)
		return historyBlock
	}

	if tokenEstimate(historyBlock) <= maxContextTokens {
		return s.recallSinglePass(ctx, stimulus, historyBlock)
	}

	chunks := partition(historyBlock, maxContextTokens)
	slog.Info("recall partitioning", "pkg", "recall", "chunks", len(chunks), "depth", depth)

	var extracted []string

	for i, chunk := range chunks {
		result := s.recallExtract(ctx, stimulus, chunk)
		if result != "" && result != "nil" {
			extracted = append(extracted, result)
		}
		slog.Info("recall chunk processed", "pkg", "recall", "chunk", i+1, "of", len(chunks), "extracted_len", len(result))
	}

	if len(extracted) == 0 {
		return ""
	}

	merged := strings.Join(extracted, "\n")

	if tokenEstimate(merged) > maxContextTokens {
		return s.recallRecursive(ctx, stimulus, merged, depth+1)
	}

	return s.recallSynthesize(ctx, stimulus, merged)
}

func (s *Service) recallSinglePass(ctx context.Context, stimulus, historyBlock string) string {
	instructions := buildRecallPrompt(stimulus)

	_, ch := s.client.Complete(ctx, instructions, llm.StaticInput(historyBlock), nil, nil)
	result := <-ch

	if result.Err != nil {
		slog.Warn("recall single pass failed", "pkg", "recall", "err", result.Err)
		return ""
	}

	slog.Info("recall completed",
		"pkg", "recall",
		"mode", "single_pass",
		"history_tokens", tokenEstimate(historyBlock),
		"output_len", len(result.Output),
		"input_tokens", result.Usage.InputTokens,
		"output_tokens", result.Usage.OutputTokens,
	)

	return result.Output
}

func (s *Service) recallExtract(ctx context.Context, stimulus, chunk string) string {
	instructions := buildRecallPrompt(stimulus)

	_, ch := s.client.Complete(ctx, instructions, llm.StaticInput(chunk), nil, nil)
	result := <-ch

	if result.Err != nil {
		slog.Warn("recall extract failed", "pkg", "recall", "err", result.Err)
		return ""
	}

	return result.Output
}

func (s *Service) recallSynthesize(ctx context.Context, stimulus, extracted string) string {
	instructions := buildSynthesizePrompt(stimulus)

	_, ch := s.client.Complete(ctx, instructions, llm.StaticInput(extracted), nil, nil)
	result := <-ch

	if result.Err != nil {
		slog.Warn("recall synthesize failed", "pkg", "recall", "err", result.Err)
		return ""
	}

	slog.Info("recall completed",
		"pkg", "recall",
		"mode", "multi_pass",
		"output_len", len(result.Output),
		"input_tokens", result.Usage.InputTokens,
		"output_tokens", result.Usage.OutputTokens,
	)

	return result.Output
}

func buildRecallPrompt(stimulus string) string {
	return `Extract facts relevant to this stimulus from the history below.

Each bullet = one fact about a person, date, preference, event, plan, or commitment.

STOP RULES — never output any of these:
- Nik's tools, skills, capabilities, architecture, or system behavior
- Technical implementation details (APIs, code, configs, IDs)
- Themes, analysis, summaries, or meta-commentary
- Duplicate facts (state each fact once)
- Closing remarks or offers

If nothing relevant, output: nil

Stimulus: ` + stimulus
}

func buildSynthesizePrompt(stimulus string) string {
	return `Merge and deduplicate these pre-filtered facts into a single list. Group by person or topic. Remove redundancy, keep all unique facts.

STOP RULES — never output any of these:
- Nik's tools, skills, capabilities, architecture, or system behavior
- Technical implementation details (APIs, code, configs, IDs)
- Themes, analysis, summaries, or meta-commentary
- Duplicate facts (state each fact once)
- Closing remarks or offers

Stimulus: ` + stimulus
}

func tokenEstimate(s string) int {
	return len(s) / charsPerToken
}

func partition(block string, maxTokens int) []string {
	maxChars := maxTokens * charsPerToken
	lines := strings.Split(block, "\n")

	var chunks []string
	var current strings.Builder

	for _, line := range lines {
		if current.Len()+len(line)+1 > maxChars && current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
		}

		if current.Len() > 0 {
			current.WriteByte('\n')
		}
		current.WriteString(line)
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}
