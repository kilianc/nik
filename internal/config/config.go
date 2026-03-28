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
	Backend         string `yaml:"backend"`
}

func (m ModelConfig) IsSubscription() bool {
	return m.Backend == "subscription"
}

type CriticConfig struct {
	Enabled         bool   `yaml:"enabled"`
	Model           string `yaml:"model"`
	ReasoningEffort string `yaml:"reasoning_effort"`
	Verbosity       string `yaml:"verbosity"`
	Backend         string `yaml:"backend"`
}

func (c CriticConfig) IsSubscription() bool {
	return c.Backend == "subscription"
}

type ModelsConfig struct {
	Main   ModelConfig  `yaml:"main"`
	Task   ModelConfig  `yaml:"task"`
	Recall ModelConfig  `yaml:"recall"`
	Critic CriticConfig `yaml:"critic"`
	TTS    TTSConfig    `yaml:"tts"`
}

func (m ModelsConfig) AnySubscription() bool {
	return m.Main.IsSubscription() || m.Task.IsSubscription() || m.Recall.IsSubscription() || m.Critic.IsSubscription()
}

type TTSConfig struct {
	Model string  `yaml:"model"`
	Voice string  `yaml:"voice"`
	Speed float64 `yaml:"speed"`
}

type ShellConfig struct {
	DockerImage string `yaml:"docker_image"`
}

type TaskConfig struct {
	MaxRounds int           `yaml:"max_rounds"`
	Timeout   time.Duration `yaml:"timeout"`
}

func (t TaskConfig) MaxRoundsOrDefault() int {
	if t.MaxRounds > 0 {
		return t.MaxRounds
	}
	return 200
}

func (t TaskConfig) TimeoutOrDefault() time.Duration {
	if t.Timeout > 0 {
		return t.Timeout
	}
	return 60 * time.Minute
}

type ConversationEntry struct {
	Label string
	ID    string
}

type ConversationList []ConversationEntry

func (cl ConversationList) IDs() []string {
	ids := make([]string, 0, len(cl))
	for _, e := range cl {
		ids = append(ids, e.ID)
	}
	return ids
}

func (cl ConversationList) ContainsID(id string) bool {
	for _, e := range cl {
		if e.ID == id {
			return true
		}
	}
	return false
}

func (cl ConversationList) LabelFor(id string) string {
	for _, e := range cl {
		if e.ID == id {
			return e.Label
		}
	}
	return ""
}

func (cl *ConversationList) Append(label, id string) {
	*cl = append(*cl, ConversationEntry{Label: label, ID: id})
}

func (cl *ConversationList) Remove(id string) bool {
	for i, e := range *cl {
		if e.ID == id {
			*cl = slices.Delete(*cl, i, i+1)
			return true
		}
	}
	return false
}

func (cl ConversationList) toMap() map[string]string {
	m := make(map[string]string, len(cl))
	for _, e := range cl {
		m[e.Label] = e.ID
	}
	return m
}

func (cl *ConversationList) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping, got %d", node.Kind)
	}

	*cl = make(ConversationList, 0, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		*cl = append(*cl, ConversationEntry{
			Label: node.Content[i].Value,
			ID:    node.Content[i+1].Value,
		})
	}
	return nil
}

func (cl ConversationList) MarshalYAML() (interface{}, error) {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, e := range cl {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: e.Label},
			&yaml.Node{Kind: yaml.ScalarNode, Value: e.ID},
		)
	}
	return node, nil
}

type Config struct {
	Home        string    `yaml:"-"`
	lastModTime time.Time `yaml:"-"`

	OpenAIKey       string       `yaml:"openai_key"`
	AnthropicKey    string       `yaml:"anthropic_key"`
	Models          ModelsConfig `yaml:"models"`
	Task            TaskConfig   `yaml:"task"`
	Shell           ShellConfig  `yaml:"shell"`
	PromptsDirValue string       `yaml:"prompts_dir"`
	SkillsDirValue  string       `yaml:"skills_dir"`

	AllowConversationIDs      ConversationList `yaml:"allow_conversation_ids"`
	PrivilegedConversationIDs ConversationList `yaml:"privileged_conversation_ids"`

	MaxHistory          int           `yaml:"max_history"`
	SystemMessageMaxAge time.Duration `yaml:"system_message_max_age"`
	Timezone            string        `yaml:"timezone"`
	Location            string        `yaml:"location"`
	JournalTime         string        `yaml:"journal_time"`
	DreamTime           string        `yaml:"dream_time"`
	BriefingTime        string        `yaml:"briefing_time"`
	DiagnosticTime      string        `yaml:"diagnostic_time"`

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

func (c Config) ErrLogPath() string {
	return filepath.Join(c.Home, "nik.err.log")
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

func (c Config) SystemMessageMaxAgeOrDefault() time.Duration {
	if c.SystemMessageMaxAge > 0 {
		return c.SystemMessageMaxAge
	}
	return 2 * time.Hour
}

func (c Config) MemoriesPath() string {
	return filepath.Join(c.Home, "memories", "latest.md")
}

func (c Config) AllowedIDs() []string {
	return c.AllowConversationIDs.IDs()
}

func (c Config) PrivilegedIDs() []string {
	return c.PrivilegedConversationIDs.IDs()
}

func (c Config) IsAllowed(id string) bool {
	return c.AllowConversationIDs.ContainsID(id)
}

func (c Config) IsPrivileged(id string) bool {
	return c.PrivilegedConversationIDs.ContainsID(id)
}

func (c Config) ConversationLabel(id string) string {
	if label := c.AllowConversationIDs.LabelFor(id); label != "" {
		return label
	}
	return c.PrivilegedConversationIDs.LabelFor(id)
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
	for _, e := range cfg.PrivilegedConversationIDs {
		if !cfg.AllowConversationIDs.ContainsID(e.ID) {
			cfg.AllowConversationIDs.Append(e.Label, e.ID)
		}
	}
}

func validateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.OpenAIKey) == "" && !cfg.Models.AnySubscription() && strings.TrimSpace(cfg.AnthropicKey) == "" {
		return fmt.Errorf("missing required config key openai_key, anthropic_key, or set backend: subscription on a model")
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
		Backend:         cfg.Models.Critic.Backend,
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

	if !isValidBackend(modelCfg.Backend) {
		return fmt.Errorf("invalid models.%s.backend %q (api, subscription, or empty)", purpose, modelCfg.Backend)
	}

	return nil
}

func isValidBackend(value string) bool {
	switch value {
	case "", "api", "subscription":
		return true
	default:
		return false
	}
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
