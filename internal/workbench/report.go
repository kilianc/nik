package workbench

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
)

var funcMap = template.FuncMap{
	"shorten":        shorten,
	"fmtDate":        fmtDate,
	"truncate":       truncateLines,
	"sub":            sub,
	"add":            add,
	"rate":           rate,
	"desiredStr":     desiredStr,
	"defaultStr":     defaultStr,
	"toolCallNames":  toolCallNames,
	"anchor":         variantAnchor,
	"tcNames":        tcNames,
	"hasRuns":        hasRuns,
	"classifiedRuns": classifiedRuns,
	"reverse":        reverse[db.ExperimentVariant],
}

//go:embed report.md.tpl
var reportTemplate string

var reportTmpl = template.Must(template.New("report").Funcs(funcMap).Parse(reportTemplate))

func RenderReport(ctx context.Context, conn *sql.DB, experimentID string) (string, error) {
	exp, err := db.ExperimentGetFull(ctx, conn, experimentID)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	err = reportTmpl.Execute(&buf, exp)
	if err != nil {
		return "", fmt.Errorf("execute report template: %w", err)
	}

	return buf.String(), nil
}

func ExperimentDir(baseDir string, exp db.Experiment) string {
	date := exp.CreatedAt.Format("2006-01-02")
	return filepath.Join(baseDir, date+"-"+id.Shorten(exp.ID))
}

func WriteReport(ctx context.Context, conn *sql.DB, experimentID, dir string) (string, error) {
	exp, err := db.ExperimentGetFull(ctx, conn, experimentID)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	err = reportTmpl.Execute(&buf, exp)
	if err != nil {
		return "", fmt.Errorf("execute report template: %w", err)
	}

	expDir := ExperimentDir(dir, exp)

	err = os.MkdirAll(expDir, 0o755)
	if err != nil {
		return "", fmt.Errorf("create experiment dir: %w", err)
	}

	path := filepath.Join(expDir, "report.md")

	err = os.WriteFile(path, []byte(buf.String()), 0o644)
	if err != nil {
		return "", fmt.Errorf("write report: %w", err)
	}

	return path, nil
}

func shorten(s string) string {
	return id.Shorten(s)
}

func fmtDate(t time.Time) string {
	return t.Format("2006-01-02")
}

func sub(a, b int) int {
	return a - b
}

func add(a, b int) int {
	return a + b
}

func rate(hit, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(hit) / float64(total) * 100
}

func defaultStr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func tcNames(tcs []db.ToolCallListRow) string {
	var names []string
	for _, tc := range tcs {
		names = append(names, tc.Name)
	}
	return strings.Join(names, ", ")
}

func hasRuns(variants []db.ExperimentVariant) bool {
	for _, v := range variants {
		if len(v.Runs) > 0 {
			return true
		}
	}
	return false
}

func classifiedRuns(v db.ExperimentVariant) int {
	n := 0
	for _, r := range v.Runs {
		if r.IsDesired != nil {
			n++
		}
	}
	return n
}

func reverse[T any](s []T) []T {
	r := make([]T, len(s))
	for i, v := range s {
		r[len(s)-1-i] = v
	}
	return r
}

func desiredStr(b *bool) string {
	if b == nil {
		return "pending"
	}
	if *b {
		return "yes"
	}
	return "no"
}

func toolCallNames(s string) string {
	var tcs []struct {
		Name string `json:"name"`
	}
	var names []string

	err := json.Unmarshal([]byte(s), &tcs)
	if err == nil {
		for _, tc := range tcs {
			names = append(names, tc.Name)
		}
	}

	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}

func variantAnchor(idx int, name string) string {
	tag := fmt.Sprintf("v%d", idx)
	anchor := strings.ReplaceAll(strings.ToLower(name), " ", "-")
	anchor = strings.ReplaceAll(anchor, "_", "-")
	return tag + "--" + anchor
}

func truncateLines(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
