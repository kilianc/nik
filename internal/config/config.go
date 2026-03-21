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

type ModelConfig struct {
	Model           string `yaml:"model"`
	ReasoningEffort string `yaml:"reasoning_effort"`
	Verbosity       string `yaml:"verbosity"`
}

type CriticConfig struct {
	Enabled         bool   `yaml:"enabled"`
	Model           string `yaml:"model"`
	ReasoningEffort string `yaml:"reasoning_effort"`
	Verbosity       string `yaml:"verbosity"`
}

type ModelsConfig struct {
	Main   ModelConfig  `yaml:"main"`
	Task   ModelConfig  `yaml:"task"`
	Recall ModelConfig  `yaml:"recall"`
	Critic CriticConfig `yaml:"critic"`
	TTS    TTSConfig    `yaml:"tts"`
}

type TTSConfig struct {
	Model string  `yaml:"model"`
	Voice string  `yaml:"voice"`
	Speed float64 `yaml:"speed"`
}

type ShellConfig struct {
	DockerImage string `yaml:"docker_image"`
}

type Config struct {
	Home        string    `yaml:"-"`
	lastModTime time.Time `yaml:"-"`

	OpenAIKey       string       `yaml:"openai_key"`
	UseCodex        bool         `yaml:"use_codex"`
	Models          ModelsConfig `yaml:"models"`
	Shell           ShellConfig  `yaml:"shell"`
	PromptsDirValue string       `yaml:"prompts_dir"`
	SkillsDirValue  string       `yaml:"skills_dir"`

	AllowConversationIDs      map[string]string `yaml:"allow_conversation_ids"`
	PrivilegedConversationIDs map[string]string `yaml:"privileged_conversation_ids"`

	MaxHistory     int    `yaml:"max_history"`
	Timezone       string `yaml:"timezone"`
	Location       string `yaml:"location"`
	JournalTime    string `yaml:"journal_time"`
	DreamTime      string `yaml:"dream_time"`
	BriefingTime   string `yaml:"briefing_time"`
	DiagnosticTime string `yaml:"diagnostic_time"`

	BannedWords []string `yaml:"banned_words"`
}

func (c Config) ShellHome() string {
	if c.Shell.DockerImage != "" {
		return "/workspace"
	}
	return c.Home
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

func (c Config) TmpPath() string {
	return filepath.Join(c.Home, "tmp")
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

func (c Config) MediaPath() string {
	return filepath.Join(c.Home, "media")
}

func (c Config) DownloadsPath() string {
	return filepath.Join(c.Home, "downloads")
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
	if c.Models.TTS.Voice == "" {
		return "ash"
	}
	return c.Models.TTS.Voice
}

func (c Config) TTSSpeedOrDefault() float64 {
	if c.Models.TTS.Speed == 0 {
		return 1.0
	}
	return c.Models.TTS.Speed
}

func (c Config) TTSModelOrDefault() string {
	if c.Models.TTS.Model == "" {
		return "gpt-4o-mini-tts"
	}
	return c.Models.TTS.Model
}

func (c Config) TTSInstructionsPath() string {
	override := filepath.Join(c.Home, "prompts", "tts-00.md")
	if _, err := os.Stat(override); err == nil {
		return override
	}

	return filepath.Join(c.PromptsPath(), "tts-00.md")
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

func (c Config) IsAllowed(id string) bool {
	for _, v := range c.AllowConversationIDs {
		if v == id {
			return true
		}
	}
	return false
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

	normalizeConfig(&cfg)
	err = validateConfig(cfg)
	if err != nil {
		return nil, err
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

	normalizeConfig(&fresh)
	err = validateConfig(fresh)
	if err != nil {
		return err
	}

	home := c.Home
	modTime := c.lastModTime
	*c = fresh
	c.Home = home
	c.lastModTime = modTime

	return nil
}

func normalizeConfig(cfg *Config) {
	if cfg.AllowConversationIDs == nil {
		cfg.AllowConversationIDs = make(map[string]string)
	}
	for label, pid := range cfg.PrivilegedConversationIDs {
		if !mapContainsValue(cfg.AllowConversationIDs, pid) {
			cfg.AllowConversationIDs[label] = pid
		}
	}
}

func validateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.OpenAIKey) == "" && !cfg.UseCodex {
		return fmt.Errorf("missing required config key openai_key (or set use_codex: true)")
	}

	if strings.TrimSpace(cfg.Models.Main.Model) == "" {
		return fmt.Errorf("missing required config key models.main.model")
	}

	err := validatePurposeModel("main", cfg.Models.Main)
	if err != nil {
		return err
	}

	err = validatePurposeModel("task", cfg.Models.Task)
	if err != nil {
		return err
	}

	err = validatePurposeModel("recall", cfg.Models.Recall)
	if err != nil {
		return err
	}

	criticModel := ModelConfig{
		Model:           cfg.Models.Critic.Model,
		ReasoningEffort: cfg.Models.Critic.ReasoningEffort,
		Verbosity:       cfg.Models.Critic.Verbosity,
	}
	err = validatePurposeModel("critic", criticModel)
	if err != nil {
		return err
	}

	if cfg.Models.Critic.Enabled && strings.TrimSpace(cfg.Models.Critic.Model) == "" {
		return fmt.Errorf("missing required config key models.critic.model when models.critic.enabled is true")
	}

	return nil
}

func validatePurposeModel(purpose string, modelCfg ModelConfig) error {
	if !isValidReasoningEffort(modelCfg.ReasoningEffort) {
		return fmt.Errorf("invalid models.%s.reasoning_effort %q (none, minimal, low, medium, high, xhigh, or empty)", purpose, modelCfg.ReasoningEffort)
	}

	if !isValidVerbosity(modelCfg.Verbosity) {
		return fmt.Errorf("invalid models.%s.verbosity %q (low, medium, high, or empty)", purpose, modelCfg.Verbosity)
	}

	return nil
}

func isValidReasoningEffort(value string) bool {
	switch value {
	case "", "none", "minimal", "low", "medium", "high", "xhigh":
		return true
	default:
		return false
	}
}

func isValidVerbosity(value string) bool {
	switch value {
	case "", "low", "medium", "high":
		return true
	default:
		return false
	}
}

func mapContainsValue(m map[string]string, val string) bool {
	for _, v := range m {
		if v == val {
			return true
		}
	}
	return false
}
