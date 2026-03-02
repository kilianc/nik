package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Home string `yaml:"-"`

	OpenAIKey       string `yaml:"openai_key"`
	UseCodex        bool   `yaml:"use_codex"`
	ExaAPIKey       string `yaml:"exa_api_key"`
	Model           string `yaml:"model"`
	ReasoningEffort string `yaml:"reasoning_effort"`
	DebugDirValue   string `yaml:"debug_dir"`
	MediaDirValue   string `yaml:"media_dir"`
	PromptsDirValue string `yaml:"prompts_dir"`
	SkillsDirValue  string `yaml:"skills_dir"`

	AllowConversationIDs      []string `yaml:"allow_conversation_ids"`
	PrivilegedConversationIDs []string `yaml:"privileged_conversation_ids"`

	MaxHistory   int    `yaml:"max_history"`
	Timezone     string `yaml:"timezone"`
	Location     string `yaml:"location"`
	JournalTime  string `yaml:"journal_time"`
	DreamStart   string `yaml:"dream_start"`
	BriefingTime string `yaml:"briefing_time"`
}

func (c Config) LogPath() string {
	return filepath.Join(c.Home, "nik.log")
}

func (c Config) DBPath() string {
	return filepath.Join(c.Home, "nik.db")
}

func (c Config) WappSessionDBPath() string {
	return filepath.Join(c.Home, "wapp_session.db")
}

func (c Config) ConfigPath() string {
	return filepath.Join(c.Home, "config.yaml")
}

func (c Config) PromptsPath() string {
	dir := c.PromptsDirValue
	if dir == "" {
		dir = "prompts"
	}

	if filepath.IsAbs(dir) {
		return dir
	}

	return filepath.Join(c.Home, dir)
}

func (c Config) SkillsPath() string {
	dir := c.SkillsDirValue
	if dir == "" {
		dir = "skills"
	}

	if filepath.IsAbs(dir) {
		return dir
	}

	return filepath.Join(c.Home, dir)
}

func (c Config) WorkspaceSkillsPath() string {
	return filepath.Join(c.Home, "skills")
}

func (c Config) MediaDir() string {
	if c.MediaDirValue == "" {
		return "media"
	}

	return c.MediaDirValue
}

func (c Config) MediaPath() string {
	dir := c.MediaDirValue
	if dir == "" {
		dir = "media"
	}

	if filepath.IsAbs(dir) {
		return dir
	}

	return filepath.Join(c.Home, dir)
}

func (c Config) TZ() *time.Location {
	if c.Timezone == "" {
		return time.Local
	}

	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return time.Local
	}

	return loc
}

func (c Config) JournalAt(now time.Time) time.Time {
	jt := c.JournalTime
	if jt == "" {
		jt = "00:00"
	}

	hour, min := 0, 0
	fmt.Sscanf(jt, "%d:%d", &hour, &min)

	loc := c.TZ()
	y, m, d := now.In(loc).Date()

	return time.Date(y, m, d, hour, min, 0, 0, loc)
}

func (c Config) BriefingAt(now time.Time) time.Time {
	bt := c.BriefingTime
	if bt == "" {
		bt = "08:00"
	}

	hour, min := 8, 0
	fmt.Sscanf(bt, "%d:%d", &hour, &min)

	loc := c.TZ()
	y, m, d := now.In(loc).Date()

	return time.Date(y, m, d, hour, min, 0, 0, loc)
}

// DreamAt returns the scheduled time for a dream pass on the current night.
// passes 1-4 are hourly from dream_start; pass 5 (wake) is dream_start + 4h.
// handles overnight boundaries: at 11pm, returns tomorrow's 2am.
func (c Config) DreamAt(now time.Time, pass int) time.Time {
	ds := c.DreamStart
	if ds == "" {
		ds = "02:00"
	}

	hour, min := 2, 0
	fmt.Sscanf(ds, "%d:%d", &hour, &min)

	loc := c.TZ()
	y, m, d := now.In(loc).Date()
	base := time.Date(y, m, d, hour, min, 0, 0, loc)

	if now.Sub(base) > 12*time.Hour {
		base = base.AddDate(0, 0, 1)
	}

	offset := time.Duration(pass-1) * time.Hour

	return base.Add(offset)
}

func (c Config) DebugPath() string {
	if c.DebugDirValue == "" {
		return ""
	}

	if filepath.IsAbs(c.DebugDirValue) {
		return c.DebugDirValue
	}

	return filepath.Join(c.Home, c.DebugDirValue)
}

func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	err = os.WriteFile(path, data, 0o644)
	if err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}

	return nil
}

func Load(home string) (*Config, error) {
	if home == "" {
		var err error
		home, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
	}

	home, err := filepath.Abs(home)
	if err != nil {
		return nil, fmt.Errorf("resolve home path: %w", err)
	}

	path := filepath.Join(home, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	cfg.Home = home

	for _, pid := range cfg.PrivilegedConversationIDs {
		if !slices.Contains(cfg.AllowConversationIDs, pid) {
			cfg.AllowConversationIDs = append(cfg.AllowConversationIDs, pid)
		}
	}

	if strings.TrimSpace(cfg.OpenAIKey) == "" && !cfg.UseCodex {
		return nil, fmt.Errorf("missing required config key openai_key (or set use_codex: true)")
	}

	return &cfg, nil
}
