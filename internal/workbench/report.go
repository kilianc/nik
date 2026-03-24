package workbench

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
)

func RenderReport(ctx context.Context, conn *sql.DB, experimentID string) (string, error) {
	exp, err := db.ExperimentGet(ctx, conn, experimentID)
	if err != nil {
		return "", err
	}

	round, err := db.ActivationRoundGet(ctx, conn, exp.ActivationRoundID)
	if err != nil {
		return "", err
	}

	act, err := db.ActivationGet(ctx, conn, round.ActivationID)
	if err != nil {
		return "", err
	}

	variants, err := db.ExperimentVariantList(ctx, conn, exp.ID)
	if err != nil {
		return "", err
	}

	variantRuns := make(map[string][]db.ExperimentRun)
	for _, v := range variants {
		runs, err := db.ExperimentRunList(ctx, conn, v.ID)
		if err != nil {
			return "", fmt.Errorf("list runs for variant %s: %w", v.ID, err)
		}
		variantRuns[v.ID] = runs
	}

	tcs, err := db.ToolCallList(ctx, conn, round.ActivationID, &round.Round)
	if err != nil {
		return "", err
	}

	var toolCallNames []string
	for _, tc := range tcs {
		toolCallNames = append(toolCallNames, tc.Name)
	}

	var b strings.Builder

	writeHeader(&b, exp)
	writeTarget(&b, exp, round, act, toolCallNames)

	if exp.DesiredOutcome != "" {
		writeDesiredOutcome(&b, exp)
	}

	if exp.Notes != "" {
		writeTrace(&b, exp)
	}

	var baseline *db.ExperimentVariant
	var nonBaseline []db.ExperimentVariant

	for i := range variants {
		if variants[i].Name == "baseline" {
			baseline = &variants[i]
		} else {
			nonBaseline = append(nonBaseline, variants[i])
		}
	}

	if baseline != nil && baseline.RunCount > 0 {
		writeVariantSection(&b, *baseline, variantRuns[baseline.ID], true)
	}

	if len(nonBaseline) > 0 {
		b.WriteString("\n---\n\n## Variants\n")
		for _, v := range nonBaseline {
			writeVariantSection(&b, v, variantRuns[v.ID], false)
		}
	}

	variantsWithRuns := countVariantsWithRuns(variants, variantRuns)
	if variantsWithRuns >= 2 {
		writeComparison(&b, variants, variantRuns)
	}

	return b.String(), nil
}

func writeHeader(b *strings.Builder, exp db.Experiment) {
	b.WriteString(fmt.Sprintf("# Experiment %s\n\n", id.Shorten(exp.ID)))
	b.WriteString(fmt.Sprintf("Created: %s\n", exp.CreatedAt.Format("2006-01-02")))
	b.WriteString(fmt.Sprintf("Status: %s\n", exp.Status))
}

func writeTarget(b *strings.Builder, exp db.Experiment, round db.ActivationRound, act db.ActivationRow, toolCallNames []string) {
	b.WriteString("\n---\n\n## Target\n\n")
	b.WriteString(fmt.Sprintf("- Round %d of activation %s\n", round.Round, id.Shorten(round.ActivationID)))
	b.WriteString(fmt.Sprintf("- Model: %s", act.Model))
	if act.ReasoningEffort != "" {
		b.WriteString(fmt.Sprintf(" | Effort: %s", act.ReasoningEffort))
	}
	if act.Verbosity != "" {
		b.WriteString(fmt.Sprintf(" | Verbosity: %s", act.Verbosity))
	}
	b.WriteString("\n")

	b.WriteString("\n### Actual Response\n\n")
	if round.ModelOutput != "" {
		b.WriteString(fmt.Sprintf("> %s\n\n", truncateLines(round.ModelOutput, 500)))
	}
	if len(toolCallNames) > 0 {
		b.WriteString(fmt.Sprintf("Tool calls: %s\n", strings.Join(toolCallNames, ", ")))
	}
}

func writeDesiredOutcome(b *strings.Builder, exp db.Experiment) {
	b.WriteString("\n---\n\n## Desired Outcome\n\n")
	b.WriteString(exp.DesiredOutcome)
	b.WriteString("\n")
}

func writeTrace(b *strings.Builder, exp db.Experiment) {
	b.WriteString("\n---\n\n## Trace\n\n")
	b.WriteString(exp.Notes)
	b.WriteString("\n")
}

func writeVariantSection(b *strings.Builder, v db.ExperimentVariant, runs []db.ExperimentRun, isBaseline bool) {
	missCount := v.RunCount - v.DesiredCount

	if isBaseline {
		b.WriteString("\n---\n\n")
		if v.RunCount > 0 {
			rate := float64(v.DesiredCount) / float64(v.RunCount) * 100
			b.WriteString(fmt.Sprintf("## Baseline — %d hit, %d miss (%.0f%% desired)\n", v.DesiredCount, missCount, rate))
		} else {
			b.WriteString("## Baseline — pending\n")
		}
	} else {
		b.WriteString("\n")
		if v.RunCount > 0 {
			rate := float64(v.DesiredCount) / float64(v.RunCount) * 100
			b.WriteString(fmt.Sprintf("### %s — %d hit, %d miss (%.0f%% desired)\n", v.Name, v.DesiredCount, missCount, rate))
		} else {
			b.WriteString(fmt.Sprintf("### %s — pending\n", v.Name))
		}
	}

	if v.Hypothesis != "" {
		b.WriteString(fmt.Sprintf("\nHypothesis: %s\n", v.Hypothesis))
	}

	var patches []Patch

	err := json.Unmarshal([]byte(v.Patches), &patches)
	if err == nil && len(patches) > 0 {
		b.WriteString("\nPatches:\n\n")
		for _, p := range patches {
			b.WriteString(fmt.Sprintf("    @@ %s @@\n", p.File))
			for _, line := range strings.Split(p.Old, "\n") {
				b.WriteString(fmt.Sprintf("    -%s\n", line))
			}
			for _, line := range strings.Split(p.New, "\n") {
				b.WriteString(fmt.Sprintf("    +%s\n", line))
			}
		}
	}

	if len(runs) > 0 {
		writeRunTable(b, runs)
	}
}

func writeRunTable(b *strings.Builder, runs []db.ExperimentRun) {
	b.WriteString("\n| # | Desired | Model Output | Tools | Tokens (in/out) |\n")
	b.WriteString("|---|---------|--------------|-------|------------------|\n")

	for i, r := range runs {
		desired := "no"
		if r.IsDesired {
			desired = "yes"
		}

		output := truncateLines(r.ModelOutput, 120)
		if output == "" {
			output = "(none)"
		}

		var toolNames []string

		var tcs []ToolCall

		err := json.Unmarshal([]byte(r.ToolCalls), &tcs)
		if err == nil {
			for _, tc := range tcs {
				toolNames = append(toolNames, tc.Name)
			}
		}

		tools := strings.Join(toolNames, ", ")
		if tools == "" {
			tools = "(none)"
		}

		b.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %d/%d |\n",
			i+1, desired, output, tools, r.InputTokens, r.OutputTokens))
	}
}

func writeComparison(b *strings.Builder, variants []db.ExperimentVariant, variantRuns map[string][]db.ExperimentRun) {
	b.WriteString("\n---\n\n## Comparison\n\n")
	b.WriteString("| Variant | Runs | Hit | Miss | Rate | Avg In | Avg Out |\n")
	b.WriteString("|---------|------|-----|------|------|--------|---------|\n")

	for _, v := range variants {
		if v.RunCount == 0 {
			continue
		}

		missCount := v.RunCount - v.DesiredCount
		rate := float64(v.DesiredCount) / float64(v.RunCount) * 100

		runs := variantRuns[v.ID]
		var totalIn, totalOut int64
		for _, r := range runs {
			totalIn += r.InputTokens
			totalOut += r.OutputTokens
		}

		avgIn := totalIn / int64(len(runs))
		avgOut := totalOut / int64(len(runs))

		b.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %.0f%% | %d | %d |\n",
			v.Name, v.RunCount, v.DesiredCount, missCount, rate, avgIn, avgOut))
	}
}

func countVariantsWithRuns(variants []db.ExperimentVariant, variantRuns map[string][]db.ExperimentRun) int {
	count := 0
	for _, v := range variants {
		if len(variantRuns[v.ID]) > 0 {
			count++
		}
	}
	return count
}

func WriteReport(ctx context.Context, conn *sql.DB, experimentID, dir string) (string, error) {
	report, err := RenderReport(ctx, conn, experimentID)
	if err != nil {
		return "", err
	}

	exp, err := db.ExperimentGet(ctx, conn, experimentID)
	if err != nil {
		return "", err
	}

	err = os.MkdirAll(dir, 0o755)
	if err != nil {
		return "", fmt.Errorf("create workbench dir: %w", err)
	}

	filename := id.Shorten(exp.ID) + ".md"
	path := filepath.Join(dir, filename)

	err = os.WriteFile(path, []byte(report), 0o644)
	if err != nil {
		return "", fmt.Errorf("write report: %w", err)
	}

	return path, nil
}

func truncateLines(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
