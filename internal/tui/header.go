package tui

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/kciuffolo/nik/internal/version"
)

func (m chatModel) renderHeader() string {
	w := m.viewWidth()
	active := len(m.activity) > 0

	brand := headerBrandStyle.Render("nik") + m.activitySuffix()
	top := renderCardTop(w, brand, m.chromeStrip(), m.pulse.Tick(), m.pulse.Energy(), active)
	body := renderCardRow(w, "", "")

	workload := workloadLabel(m.pendingAlarms, m.activeTasks)
	status := m.statusLabel()
	rightInlay := status

	// Bottom border layout: 2 corners + workload inlay (3 + width) + min 1 fill
	// + right inlay (3 + width(ws) + 3 separator + width(status)). Solve for
	// max ws width that keeps everything on one line.
	if ws := m.workspaceLabel(); ws != "" {
		budget := w - 9 - lipgloss.Width(status)
		if workload != "" {
			budget -= lipgloss.Width(workload) + 3
		}
		if budget >= 2 {
			shown := truncatePath(ws, budget)
			rightInlay = headerPathStyle.Render(shown) + headerMetaStyle.Render(" · ") + status
		}
	}

	bottom := renderCardBottom(w, workload, rightInlay, m.pulse.Tick(), m.pulse.Energy(), active)
	return top + "\n" + body + "\n" + bottom
}

func (m chatModel) statusLabel() string {
	if m.daemonAlive {
		return statusOKLabelStyle.Render("online") + " " + statusOKStyle.Render("●")
	}
	return statusDownStyle.Render("offline") + " " + statusDownStyle.Render("○")
}

func (m chatModel) activitySuffix() string {
	if len(m.activity) == 0 {
		return ""
	}
	ndots := (m.pulse.Tick() / 8) % 4
	dots := strings.Repeat(".", ndots) + strings.Repeat(" ", 3-ndots)
	return thinkingStyle.Render(" is thinking" + dots)
}

func (m chatModel) chromeStrip() string {
	parts := []string{"v" + version.V}
	if m.cfg != nil {
		if model := modelAcronym(m.cfg.Models.Main.Model); model != "" {
			parts = append(parts, model)
		}
	}
	if age := nikAgeLabel(m.genesisAt, time.Now()); age != "" {
		parts = append(parts, age)
	}
	if m.cfg != nil {
		if tz := tzLabel(m.cfg.Timezone); tz != "" {
			parts = append(parts, tz)
		}
	}
	return headerMetaStyle.Render(strings.Join(parts, " · "))
}

func (m chatModel) workspaceLabel() string {
	if m.cfg == nil {
		return ""
	}
	return homeDisplay(m.cfg.Home)
}

func renderCardTop(w int, left, right string, tick int, energy float64, active bool) string {
	return renderCardBorder(w, "╭", "╮", left, right, tick, energy, active)
}

func renderCardBottom(w int, left, right string, tick int, energy float64, active bool) string {
	return renderCardBorder(w, "╰", "╯", left, right, tick, energy, active)
}

func renderCardBorder(w int, leftCorner, rightCorner, left, right string, tick int, energy float64, active bool) string {
	if w < 2 {
		return ""
	}
	leftShown, rightShown, fillW := layoutInlaidBorder(w, left, right)

	var middle string
	if active && fillW > 0 {
		middle = thinkMorph(tick, energy, fillW)
	} else {
		middle = headerDividerStyle.Render(strings.Repeat("─", fillW))
	}

	return assembleInlaidBorder(leftCorner, rightCorner, leftShown, rightShown, middle)
}

// Each inlay costs width(label) + 3 (space + label + space + dash); we keep
// at least 1 fill dash when any inlay is present.
func layoutInlaidBorder(w int, left, right string) (string, string, int) {
	available := w - 2

	rightShown := ""
	if right != "" {
		needed := lipgloss.Width(right) + 3 + 1
		if needed <= available {
			rightShown = right
			available -= lipgloss.Width(rightShown) + 3
		}
	}

	leftShown := ""
	if left != "" {
		maxLeft := available - 3 - 1
		if maxLeft >= 1 {
			leftShown = truncatePath(left, maxLeft)
			available -= lipgloss.Width(leftShown) + 3
		}
	}

	if available < 0 {
		available = 0
	}
	return leftShown, rightShown, available
}

func assembleInlaidBorder(leftCornerCh, rightCornerCh, leftLabel, rightLabel, middle string) string {
	d := headerDividerStyle
	var b strings.Builder
	b.WriteString(d.Render(leftCornerCh))
	if leftLabel != "" {
		b.WriteString(d.Render("─ "))
		b.WriteString(headerPathStyle.Render(leftLabel))
		b.WriteString(d.Render(" "))
	}
	b.WriteString(middle)
	if rightLabel != "" {
		b.WriteString(d.Render(" "))
		b.WriteString(headerPathStyle.Render(rightLabel))
		b.WriteString(d.Render(" ─"))
	}
	b.WriteString(d.Render(rightCornerCh))
	return b.String()
}

func renderCardRow(w int, left, right string) string {
	pipe := headerDividerStyle.Render("│")
	if w < 2 {
		return ""
	}
	innerW := w - 4
	if innerW < 1 {
		return pipe + strings.Repeat(" ", w-2) + pipe
	}

	gap := innerW - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	inner := left + strings.Repeat(" ", gap) + right
	if cur := lipgloss.Width(inner); cur < innerW {
		inner += strings.Repeat(" ", innerW-cur)
	}
	return pipe + " " + inner + " " + pipe
}

func homeDisplay(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		if p == h {
			return "~"
		}
		if strings.HasPrefix(p, h+string(os.PathSeparator)) {
			return "~" + strings.TrimPrefix(p, h)
		}
	}
	return p
}

func truncatePath(p string, max int) string {
	if max <= 0 {
		return ""
	}
	if lipgloss.Width(p) <= max {
		return p
	}
	if max == 1 {
		return "…"
	}
	// Keep the tail (most informative part of a path): "…/tail"
	r := []rune(p)
	tail := r[len(r)-(max-1):]
	return "…" + string(tail)
}

var dateSuffixRe = regexp.MustCompile(`-(\d{8})$`)

func modelAcronym(model string) string {
	m := strings.TrimSpace(model)
	if m == "" {
		return "?"
	}
	m = strings.TrimPrefix(m, "claude-")
	m = dateSuffixRe.ReplaceAllString(m, "")
	return m
}

// tzLabel returns a compact form of an IANA timezone for display in the
// header. We keep just the last path segment ("America/Los_Angeles" ->
// "Los_Angeles") since the prefix is rarely informative at a glance.
func tzLabel(tz string) string {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return ""
	}
	if i := strings.LastIndex(tz, "/"); i >= 0 && i+1 < len(tz) {
		return tz[i+1:]
	}
	return tz
}

// workloadLabel summarises pending work as "N alarms · M tasks". Zero
// counts are hidden; when both are zero the label is empty so the inlay
// disappears entirely.
func workloadLabel(alarms, tasks int) string {
	var parts []string
	if alarms > 0 {
		parts = append(parts, plural(alarms, "alarm"))
	}
	if tasks > 0 {
		parts = append(parts, plural(tasks, "task"))
	}
	return strings.Join(parts, " · ")
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func nikAgeLabel(genesisAt, now time.Time) string {
	if genesisAt.IsZero() {
		return ""
	}
	days := int(now.Sub(genesisAt).Hours() / 24)
	switch {
	case days <= 0:
		return "nik was born today"
	case days == 1:
		return "nik is 1 day old"
	default:
		return fmt.Sprintf("nik is %d days old", days)
	}
}

func modelAge(model string, now time.Time) string {
	match := dateSuffixRe.FindStringSubmatch(strings.TrimSpace(model))
	if len(match) != 2 {
		return ""
	}
	t, err := time.Parse("20060102", match[1])
	if err != nil {
		return ""
	}
	days := int(now.Sub(t).Hours() / 24)
	switch {
	case days < 1:
		return "new"
	case days < 14:
		return fmt.Sprintf("%dd", days)
	case days < 60:
		return fmt.Sprintf("%dw", days/7)
	case days < 365:
		return fmt.Sprintf("%dmo", days/30)
	default:
		years := days / 365
		months := (days % 365) / 30
		if months == 0 {
			return fmt.Sprintf("%dy", years)
		}
		return fmt.Sprintf("%dy%dmo", years, months)
	}
}
