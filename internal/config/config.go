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

	AllowConversationIDs      map[string]string `yaml:"allow_conversation_ids"`
	PrivilegedConversationIDs map[string]string `yaml:"privileged_conversation_ids"`

	RecallModel string `yaml:"recall_model"`

	MaxHistory     int    `yaml:"max_history"`
	Timezone       string `yaml:"timezone"`
	Location       string `yaml:"location"`
	JournalTime    string `yaml:"journal_time"`
	DreamTime      string `yaml:"dream_time"`
	BriefingTime   string `yaml:"briefing_time"`
	DiagnosticTime string `yaml:"diagnostic_time"`

	BannedWords []string `yaml:"banned_words"`

	TTSVoice        string  `yaml:"tts_voice"`
	TTSInstructions string  `yaml:"tts_instructions"`
	TTSSpeed        float64 `yaml:"tts_speed"`
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

func (c Config) TTSVoiceOrDefault() string {
	if c.TTSVoice == "" {
		return "ash"
	}
	return c.TTSVoice
}

func (c Config) TTSSpeedOrDefault() float64 {
	if c.TTSSpeed == 0 {
		return 1.0
	}
	return c.TTSSpeed
}

func (c Config) MemoriesPath() string {
	return filepath.Join(c.Home, "memories", "latest.md")
}

func (c Config) AllowedIDs() []string {
	ids := make([]string, 0, len(c.AllowConversationIDs))
	for _, id := range c.AllowConversationIDs {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func (c Config) PrivilegedIDs() []string {
	ids := make([]string, 0, len(c.PrivilegedConversationIDs))
	for _, id := range c.PrivilegedConversationIDs {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func (c Config) IsPrivileged(id string) bool {
	for _, v := range c.PrivilegedConversationIDs {
		if v == id {
			return true
		}
	}
	return false
}

func (c Config) ConversationLabel(id string) string {
	for label, v := range c.AllowConversationIDs {
		if v == id {
			return label
		}
	}
	for label, v := range c.PrivilegedConversationIDs {
		if v == id {
			return label
		}
	}
	return ""
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

	if cfg.AllowConversationIDs == nil {
		cfg.AllowConversationIDs = make(map[string]string)
	}
	for label, pid := range cfg.PrivilegedConversationIDs {
		if !mapContainsValue(cfg.AllowConversationIDs, pid) {
			cfg.AllowConversationIDs[label] = pid
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

	if c.AllowConversationIDs == nil {
		c.AllowConversationIDs = make(map[string]string)
	}
	for label, pid := range c.PrivilegedConversationIDs {
		if !mapContainsValue(c.AllowConversationIDs, pid) {
			c.AllowConversationIDs[label] = pid
		}
	}

	return nil
}

func mapContainsValue(m map[string]string, val string) bool {
	for _, v := range m {
		if v == val {
			return true
		}
	}
	return false
}
