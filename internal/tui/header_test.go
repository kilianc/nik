package tui

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/version"
)

func TestHeaderShowsVersionModelStatus(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	home := t.TempDir()
	cfg := &config.Config{Home: home, Timezone: "America/Los_Angeles"}
	cfg.Models.Main.Model = "claude-sonnet-4-20250514"

	wantTail := home
	if userHome, err := os.UserHomeDir(); err == nil && userHome != "" && strings.HasPrefix(home, userHome+string(os.PathSeparator)) {
		wantTail = "~" + strings.TrimPrefix(home, userHome)
	}
	// At minimum, some recognizable tail of the path must be in the header.
	// Layered style packs more onto the top border so the path truncates harder;
	// asserting on the trailing path segment keeps the test robust either way.
	pathLeaf := wantTail
	if i := strings.LastIndex(pathLeaf, "/"); i >= 0 {
		pathLeaf = pathLeaf[i:]
	}

	c := newChatModel(cfg, conn, nil, Options{})
	c, _ = c.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	c.daemonAlive = true
	c.genesisAt = time.Now().Add(-23 * 24 * time.Hour)
	c, _ = c.Update(workloadMsg{alarms: 2, tasks: 1})

	out := stripAnsi(c.renderHeader())
	if !strings.Contains(out, "nik") {
		t.Errorf("expected 'nik' brand, got %q", out)
	}
	if !strings.Contains(out, version.V) {
		t.Errorf("expected version %s, got %q", version.V, out)
	}
	if !strings.Contains(out, pathLeaf) {
		t.Errorf("expected workspace tail %q in header, got %q", pathLeaf, out)
	}
	if !strings.Contains(out, "sonnet-4") {
		t.Errorf("expected acronym sonnet-4, got %q", out)
	}
	if !regexp.MustCompile(`nik is \d+ days? old`).MatchString(out) {
		t.Errorf("expected nik age, got %q", out)
	}
	if !strings.Contains(out, "Los_Angeles") {
		t.Errorf("expected tz Los_Angeles, got %q", out)
	}
	if !strings.Contains(out, "2 alarms") {
		t.Errorf("expected '2 alarms' workload, got %q", out)
	}
	if !strings.Contains(out, "1 task") {
		t.Errorf("expected '1 task' workload, got %q", out)
	}
	if !strings.Contains(out, "online") {
		t.Errorf("expected online status, got %q", out)
	}
	if !strings.Contains(out, "●") {
		t.Errorf("expected status dot, got %q", out)
	}
	if strings.Index(out, "●") < strings.Index(out, "online") {
		t.Errorf("expected status dot to sit right of 'online', got %q", out)
	}
	for _, ch := range []string{"╭", "╮", "╰", "╯", "│", "─"} {
		if !strings.Contains(out, ch) {
			t.Errorf("expected border char %q, got %q", ch, out)
		}
	}

	rows := strings.Split(out, "\n")
	if len(rows) != 3 {
		t.Fatalf("expected 3 header rows, got %d:\n%s", len(rows), out)
	}
	// Brand lives on the top border, body row is intentionally empty.
	if !strings.Contains(rows[0], "nik") {
		t.Errorf("expected 'nik' on top border, got %q", rows[0])
	}
	if strings.Contains(rows[1], "nik") {
		t.Errorf("expected empty body row, got %q", rows[1])
	}

	c.daemonAlive = false
	out = stripAnsi(c.renderHeader())
	if !strings.Contains(out, "offline") {
		t.Errorf("expected offline status, got %q", out)
	}
	if !strings.Contains(out, "○") {
		t.Errorf("expected hollow dot, got %q", out)
	}
	if strings.Index(out, "○") < strings.Index(out, "offline") {
		t.Errorf("expected hollow dot right of 'offline', got %q", out)
	}
}

func TestHeaderShowsThinkingWhenActive(t *testing.T) {
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer conn.Close()

	c := newTestChat(t, conn, nil, Options{})
	c, _ = c.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	c.daemonAlive = true

	idle := stripAnsi(c.renderHeader())
	if strings.Contains(idle, "is thinking") {
		t.Errorf("did not expect thinking text when idle, got %q", idle)
	}
	if !strings.Contains(idle, "online") {
		t.Errorf("expected online status when idle, got %q", idle)
	}

	c.activity = []string{"thinking"}
	active := stripAnsi(c.renderHeader())
	if !strings.Contains(active, "is thinking") {
		t.Errorf("expected 'is thinking' in active header, got %q", active)
	}
	if !strings.Contains(active, "online") {
		t.Errorf("expected 'online' to remain visible when thinking, got %q", active)
	}

	widths := map[int]int{}
	for tick := 0; tick < 32; tick++ {
		c.pulse.tick = tick
		w := lipgloss.Width(c.renderHeader())
		widths[w]++
	}
	if len(widths) != 1 {
		t.Errorf("expected stable header width across dot animation, got %v", widths)
	}
}

func TestModelAcronym(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"gpt-5.3-codex", "gpt-5.3-codex"},
		{"gpt-5.4", "gpt-5.4"},
		{"claude-sonnet-4-20250514", "sonnet-4"},
		{"claude-opus-4-20250514", "opus-4"},
		{"", "?"},
		{"weird", "weird"},
		{"  gpt-5.4  ", "gpt-5.4"},
	}
	for _, tt := range tests {
		got := modelAcronym(tt.in)
		if got != tt.want {
			t.Errorf("modelAcronym(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestModelAge(t *testing.T) {
	now := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		in, want string
	}{
		{"claude-sonnet-4-20260417", "new"},
		{"claude-sonnet-4-20260416", "1d"},
		{"claude-sonnet-4-20260410", "7d"},
		{"claude-sonnet-4-20260301", "6w"},
		{"claude-sonnet-4-20250514", "11mo"},
		{"claude-sonnet-4-20240417", "2y"},
		{"claude-sonnet-4-20230117", "3y3mo"},
		{"gpt-5.4", ""},
		{"", ""},
		{"claude-sonnet-4-99999999", ""},
	}
	for _, tt := range tests {
		got := modelAge(tt.in, now)
		if got != tt.want {
			t.Errorf("modelAge(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNikAgeLabel(t *testing.T) {
	now := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		genesis time.Time
		want    string
	}{
		{"zero genesis returns empty", time.Time{}, ""},
		{"same moment = born today", now, "nik was born today"},
		{"1 day", now.Add(-24 * time.Hour), "nik is 1 day old"},
		{"2 days", now.Add(-2 * 24 * time.Hour), "nik is 2 days old"},
		{"future genesis says born today", now.Add(5 * time.Hour), "nik was born today"},
		{"23 days", now.Add(-23 * 24 * time.Hour), "nik is 23 days old"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nikAgeLabel(tt.genesis, now)
			if got != tt.want {
				t.Errorf("nikAgeLabel = %q, want %q", got, tt.want)
			}
		})
	}
}
