package recall

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
)

const (
	recallTimeout    = 30 * time.Second
	maxContextTokens = 800_000
	charsPerToken    = 4
	recentWindow     = 24 * time.Hour
)

type Service struct {
	cfg    *config.Config
	conn   *sql.DB
	client *llm.Client
}

func NewService(cfg *config.Config, conn *sql.DB, client *llm.Client) *Service {
	return &Service{cfg: cfg, conn: conn, client: client}
}

func (s *Service) Recall(ctx context.Context, stimulus string) string {
	if s.client == nil {
		return ""
	}

	since := time.Now().Add(-recentWindow)

	messages, err := db.RecallMessages(ctx, s.conn, since)
	if err != nil {
		slog.Warn("recall fetch messages", "pkg", "recall", "err", err)
	}

	var memoriesRaw string
	data, err := os.ReadFile(s.cfg.MemoriesPath())
	if err != nil && !os.IsNotExist(err) {
		slog.Warn("recall read memories file", "pkg", "recall", "err", err)
	} else if len(data) > 0 {
		memoriesRaw = string(data)
	}

	contacts, err := db.RecallContacts(ctx, s.conn)
	if err != nil {
		slog.Warn("recall fetch contacts", "pkg", "recall", "err", err)
	}

	alarms, err := db.RecallAlarms(ctx, s.conn)
	if err != nil {
		slog.Warn("recall fetch alarms", "pkg", "recall", "err", err)
	}

	journals, err := db.RecallJournals(ctx, s.conn)
	if err != nil {
		slog.Warn("recall fetch journals", "pkg", "recall", "err", err)
	}

	dreams, err := db.RecallDreams(ctx, s.conn)
	if err != nil {
		slog.Warn("recall fetch dreams", "pkg", "recall", "err", err)
	}

	briefings, err := db.RecallBriefings(ctx, s.conn)
	if err != nil {
		slog.Warn("recall fetch briefings", "pkg", "recall", "err", err)
	}

	block := formatAll(messages, memoriesRaw, contacts, alarms, journals, dreams, briefings)
	if block == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, recallTimeout)
	defer cancel()

	slog.Info("recall starting",
		"pkg", "recall",
		"messages", len(messages),
		"memories_chars", len(memoriesRaw),
		"contacts", len(contacts),
		"alarms", len(alarms),
		"history_chars", len(block),
	)

	result := s.recallRecursive(ctx, stimulus, block, 0)

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

	_, ch := s.client.Complete(ctx, instructions, historyBlock, nil, nil)
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

	_, ch := s.client.Complete(ctx, instructions, chunk, nil, nil)
	result := <-ch

	if result.Err != nil {
		slog.Warn("recall extract failed", "pkg", "recall", "err", result.Err)
		return ""
	}

	return result.Output
}

func (s *Service) recallSynthesize(ctx context.Context, stimulus, extracted string) string {
	instructions := buildSynthesizePrompt(stimulus)

	_, ch := s.client.Complete(ctx, instructions, extracted, nil, nil)
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

func formatAll(
	messages []db.RecallMessage,
	memoriesRaw string,
	contacts []db.RecallContact,
	alarms []db.RecallAlarm,
	journals []db.RecallJournal,
	dreams []db.RecallDream,
	briefings []db.RecallBriefing,
) string {
	var b strings.Builder

	if len(contacts) > 0 {
		b.WriteString("## Contacts\n\n")
		for _, c := range contacts {
			fmt.Fprintf(&b, "- %s", c.Name)
			if len(c.Nicknames) > 0 {
				fmt.Fprintf(&b, " (aka %s)", strings.Join(c.Nicknames, ", "))
			}
			if c.Location.Valid {
				fmt.Fprintf(&b, ", %s", c.Location.String)
			}
			if c.Timezone.Valid {
				fmt.Fprintf(&b, " [%s]", c.Timezone.String)
			}
			if c.OneLiner.Valid {
				fmt.Fprintf(&b, " — %s", c.OneLiner.String)
			}
			b.WriteByte('\n')
			if len(c.Emails) > 0 {
				fmt.Fprintf(&b, "  emails: %s\n", strings.Join(c.Emails, ", "))
			}
			if len(c.PhoneNumbers) > 0 {
				fmt.Fprintf(&b, "  phones: %s\n", strings.Join(c.PhoneNumbers, ", "))
			}
			if c.Notes.Valid && c.Notes.String != "" {
				fmt.Fprintf(&b, "  notes: %s\n", c.Notes.String)
			}
		}
		b.WriteByte('\n')
	}

	if memoriesRaw != "" {
		b.WriteString("## Memories\n\n")
		b.WriteString(memoriesRaw)
		b.WriteByte('\n')
	}

	if len(alarms) > 0 {
		b.WriteString("## Alarms & Reminders\n\n")
		for _, a := range alarms {
			status := "active"
			if a.CancelledAt.Valid {
				status = "cancelled"
			}
			fmt.Fprintf(&b, "- [%s] %s", a.CreatedAt.Format("2006-01-02"), a.Goal)
			if a.Recurrence.Valid {
				fmt.Fprintf(&b, " (recurrence: %s)", a.Recurrence.String)
			}
			if a.NextFireAt.Valid {
				fmt.Fprintf(&b, " next: %s", a.NextFireAt.Time.Format("2006-01-02 15:04"))
			}
			fmt.Fprintf(&b, " [%s]\n", status)
		}
		b.WriteByte('\n')
	}

	if len(journals) > 0 {
		b.WriteString("## Journal Entries\n\n")
		for _, j := range journals {
			fmt.Fprintf(&b, "### %s\n%s\n\n", j.Date, j.Content)
		}
	}

	if len(dreams) > 0 {
		b.WriteString("## Dream Passes\n\n")
		for _, d := range dreams {
			fmt.Fprintf(&b, "### %s (pass %d)\n%s\n\n", d.Date, d.Pass, d.Content)
		}
	}

	if len(briefings) > 0 {
		b.WriteString("## Briefings\n\n")
		for _, br := range briefings {
			fmt.Fprintf(&b, "### %s\n%s\n\n", br.Date, br.Content)
		}
	}

	if len(messages) > 0 {
		b.WriteString("## Recent Conversations (last 24h)\n\n")
		var lastConv string
		for _, m := range messages {
			conv := m.ConversationTitle
			if conv == "" {
				conv = m.ConversationKind
			}
			if conv != lastConv {
				fmt.Fprintf(&b, "\n### %s\n\n", conv)
				lastConv = conv
			}
			sender := m.SenderName
			if sender == "" && m.IsFromMe {
				sender = "Nik"
			}
			ts := m.SentAt.Format("2006-01-02 15:04")
			fmt.Fprintf(&b, "[%s] %s: %s\n", ts, sender, m.Body)
		}
	}

	return b.String()
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
