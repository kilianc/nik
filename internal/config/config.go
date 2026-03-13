package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Home        string    `yaml:"-"`
	lastModTime time.Time `yaml:"-"`

	OpenAIKey       string `yaml:"openai_key"`
	UseCodex        bool   `yaml:"use_codex"`
	ExaAPIKey       string `yaml:"exa_api_key"`
	Model           string `yaml:"model"`
	ReasoningEffort string `yaml:"reasoning_effort"`
	Verbosity       string `yaml:"verbosity"`
	MediaDirValue   string `yaml:"media_dir"`
	PromptsDirValue string `yaml:"prompts_dir"`
	SkillsDirValue  string `yaml:"skills_dir"`

	AllowConversationIDs      []string `yaml:"allow_conversation_ids"`
	PrivilegedConversationIDs []string `yaml:"privileged_conversation_ids"`

	RecallModel string `yaml:"recall_model"`

	MaxHistory     int    `yaml:"max_history"`
	Timezone       string `yaml:"timezone"`
	Location       string `yaml:"location"`
	JournalTime    string `yaml:"journal_time"`
	DreamStart     string `yaml:"dream_start"`
	BriefingTime   string `yaml:"briefing_time"`
	DiagnosticTime string `yaml:"diagnostic_time"`

	BannedWords []string `yaml:"banned_words"`
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

func (c Config) MemoriesPath() string {
	return filepath.Join(c.Home, "memories", "latest.md")
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

	info, err := os.Stat(path)
	if err == nil {
		cfg.lastModTime = info.ModTime()
	}

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

func (c *Config) ReloadIfChanged() (bool, error) {
	info, err := os.Stat(c.ConfigPath())
	if err != nil {
		return false, fmt.Errorf("stat config: %w", err)
	}

	if !info.ModTime().After(c.lastModTime) {
		return false, nil
	}

	err = c.reload()
	if err != nil {
		return false, err
	}

	c.lastModTime = info.ModTime()
	slog.Info("config reloaded", "pkg", "config")

	return true, nil
}

func (c *Config) reload() error {
	path := c.ConfigPath()

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var fresh Config
	err = yaml.Unmarshal(data, &fresh)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	home := c.Home
	modTime := c.lastModTime
	*c = fresh
	c.Home = home
	c.lastModTime = modTime

	for _, pid := range c.PrivilegedConversationIDs {
		if !slices.Contains(c.AllowConversationIDs, pid) {
			c.AllowConversationIDs = append(c.AllowConversationIDs, pid)
		}
	}

	return nil
}
