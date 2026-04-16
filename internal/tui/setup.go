package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kciuffolo/nik/internal/codex"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/daemonctl"
	"github.com/kciuffolo/nik/internal/id"
	"github.com/kciuffolo/nik/internal/secrets"
)

type setupStep int

const (
	stepWelcome setupStep = iota
	stepAuthChoice
	stepCodexLogin
	stepCodexDone
	stepAPIKey
	stepAPIKeyValidating
	stepModel
	stepDocker
	stepTimezone
	stepTZResolving
	stepWriting
	stepDone
	stepWaitDaemon
)

type setupModel struct {
	step       setupStep
	apiKeyIn   textinput.Model
	timezoneIn textinput.Model
	spinner    spinner.Model
	err        error
	cfg        *config.Config

	authCursor      int
	hasSubscription bool
	resolvedTZ      string
	models          []string
	modelCursor     int
	dockerCursor    int

	daemonWasAlive   bool
	daemonOldPID     int
	serviceInstalled bool
	completed        bool

	width  int
	height int
}

var defaultModels = []string{
	"gpt-5.3-codex",
	"gpt-5.4",
	"claude-sonnet-4-20250514",
	"claude-opus-4-20250514",
}

func newSetupModel(cfg *config.Config) setupModel {
	apiKey := textinput.New()
	apiKey.Placeholder = "sk-..."
	apiKey.EchoMode = textinput.EchoPassword
	apiKey.Width = 60
	if existing, err := secrets.New(cfg.Home).Get("openai_key"); err == nil {
		apiKey.SetValue(existing)
	}

	defaultCursor := 0
	for i, m := range defaultModels {
		if m == cfg.Models.Main.Model {
			defaultCursor = i
			break
		}
	}

	tz := cfg.Timezone
	if tz == "" {
		tz = time.Now().Location().String()
	}
	if tz == "Local" {
		tz = ""
	}
	tzIn := textinput.New()
	tzIn.SetValue(tz)
	tzIn.Width = 50

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))

	return setupModel{
		step:        stepWelcome,
		apiKeyIn:    apiKey,
		timezoneIn:  tzIn,
		spinner:     sp,
		cfg:         cfg,
		models:      defaultModels,
		modelCursor: defaultCursor,
	}
}

type codexLoginMsg struct {
	err error
}

type apiKeyValidatedMsg struct{ err error }
type configWrittenMsg struct{ err error }

type tzResolvedMsg struct {
	timezone string
	err      error
}

type daemonPollMsg struct {
	pid   int
	alive bool
}

func codexLoginCmd() tea.Cmd {
	return func() tea.Msg {
		_, err := codex.LoadOrLogin("")
		if err != nil {
			return codexLoginMsg{err: err}
		}
		return codexLoginMsg{}
	}
}

func validateAPIKeyCmd(key string) tea.Cmd {
	return func() tea.Msg {
		req, err := http.NewRequest("GET", "https://api.openai.com/v1/models", nil)
		if err != nil {
			return apiKeyValidatedMsg{err: err}
		}

		req.Header.Set("Authorization", "Bearer "+key)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return apiKeyValidatedMsg{err: fmt.Errorf("connect to OpenAI: %w", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized {
			return apiKeyValidatedMsg{err: fmt.Errorf("invalid API key (401)")}
		}

		if resp.StatusCode != http.StatusOK {
			return apiKeyValidatedMsg{err: fmt.Errorf("unexpected status %d", resp.StatusCode)}
		}

		return apiKeyValidatedMsg{}
	}
}

func resolveTimezoneCmd(apiKey, input string) tea.Cmd {
	return func() tea.Msg {
		body := map[string]interface{}{
			"model":             "gpt-4.1-nano",
			"instructions":      "Reply with only the IANA timezone string for the given location. Example: America/New_York. Nothing else.",
			"input":             input,
			"max_output_tokens": 30,
		}

		data, err := json.Marshal(body)
		if err != nil {
			return tzResolvedMsg{err: err}
		}

		req, err := http.NewRequest("POST", "https://api.openai.com/v1/responses", bytes.NewReader(data))
		if err != nil {
			return tzResolvedMsg{err: err}
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return tzResolvedMsg{err: fmt.Errorf("connect to OpenAI: %w", err)}
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return tzResolvedMsg{err: err}
		}

		if resp.StatusCode != http.StatusOK {
			return tzResolvedMsg{err: fmt.Errorf("OpenAI API error (%d): %s", resp.StatusCode, respBody)}
		}

		var result struct {
			Output []struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"output"`
		}

		err = json.Unmarshal(respBody, &result)
		if err != nil {
			return tzResolvedMsg{err: err}
		}

		var tz string
		if len(result.Output) > 0 && len(result.Output[0].Content) > 0 {
			tz = strings.TrimSpace(result.Output[0].Content[0].Text)
		}
		if tz == "" {
			return tzResolvedMsg{err: fmt.Errorf("no response from model")}
		}

		_, err = time.LoadLocation(tz)
		if err != nil {
			return tzResolvedMsg{err: fmt.Errorf("could not determine timezone for %q", input)}
		}

		return tzResolvedMsg{timezone: tz}
	}
}

func writeConfigCmd(cfg *config.Config, apiKey string) tea.Cmd {
	return func() tea.Msg {
		if apiKey != "" {
			err := secrets.New(cfg.Home).Set("openai_key", apiKey)
			if err != nil {
				return configWrittenMsg{err: err}
			}
		}

		cfg.Normalize()
		err := cfg.Save(cfg.ConfigPath())
		return configWrittenMsg{err: err}
	}
}

func daemonPollCmd(home string) tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		pid, alive := daemonctl.CheckPID(home)
		return daemonPollMsg{pid: pid, alive: alive}
	})
}

func (m setupModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m setupModel) Update(msg tea.Msg) (setupModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case codexLoginMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = stepCodexLogin
			return m, nil
		}
		m.hasSubscription = true
		m.err = nil
		m.step = stepCodexDone
		return m, nil

	case apiKeyValidatedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = stepAPIKey
			m.apiKeyIn.Focus()
			return m, textinput.Blink
		}
		m.err = nil
		m.step = stepModel
		return m, nil

	case tzResolvedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.resolvedTZ = ""
			m.step = stepTimezone
			m.timezoneIn.Focus()
			return m, textinput.Blink
		}
		m.err = nil
		m.resolvedTZ = msg.timezone
		m.timezoneIn.SetValue(msg.timezone)
		m.step = stepTimezone
		m.timezoneIn.Focus()
		return m, textinput.Blink

	case configWrittenMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.step = stepDone
		pid, alive := daemonctl.CheckPID(m.cfg.Home)
		m.daemonWasAlive = alive
		m.daemonOldPID = pid
		m.serviceInstalled = daemonctl.IsInstalled()
		if !alive {
			return m, tea.Batch(m.spinner.Tick, daemonPollCmd(m.cfg.Home))
		}
		return m, nil

	case daemonPollMsg:
		if msg.alive && msg.pid != m.daemonOldPID {
			m.completed = true
			return m, nil
		}
		return m, daemonPollCmd(m.cfg.Home)
	}

	switch m.step {
	case stepWelcome:
		return m.updateWelcome(msg)
	case stepAuthChoice:
		return m.updateAuthChoice(msg)
	case stepCodexLogin:
		return m.updateCodexLogin(msg)
	case stepCodexDone:
		return m.updateCodexDone(msg)
	case stepAPIKey:
		return m.updateAPIKey(msg)
	case stepAPIKeyValidating:
		return m.updateSpinner(msg)
	case stepModel:
		return m.updateModel(msg)
	case stepDocker:
		return m.updateDocker(msg)
	case stepTimezone:
		return m.updateTimezone(msg)
	case stepTZResolving:
		return m.updateSpinner(msg)
	case stepDone:
		return m.updateDone(msg)
	case stepWaitDaemon:
		return m.updateSpinner(msg)
	}

	return m, nil
}

func (m setupModel) updateWelcome(msg tea.Msg) (setupModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		m.step = stepAuthChoice
		return m, nil
	}
	return m, nil
}

func (m setupModel) updateAuthChoice(msg tea.Msg) (setupModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			m.authCursor = 0
			return m, nil
		case "down", "j":
			m.authCursor = 1
			return m, nil
		case "enter":
			if m.authCursor == 0 {
				m.step = stepCodexLogin
				return m, codexLoginCmd()
			}
			m.step = stepAPIKey
			m.apiKeyIn.Focus()
			return m, textinput.Blink
		}
	}
	return m, nil
}

func (m setupModel) updateCodexLogin(msg tea.Msg) (setupModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" && m.err != nil {
		m.err = nil
		return m, codexLoginCmd()
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m setupModel) updateCodexDone(msg tea.Msg) (setupModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		m.step = stepAPIKey
		m.apiKeyIn.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m setupModel) updateAPIKey(msg tea.Msg) (setupModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		val := m.apiKeyIn.Value()
		if val == "" {
			m.err = fmt.Errorf("API key is required")
			return m, nil
		}
		m.step = stepAPIKeyValidating
		m.err = nil
		return m, validateAPIKeyCmd(val)
	}

	var cmd tea.Cmd
	m.apiKeyIn, cmd = m.apiKeyIn.Update(msg)
	return m, cmd
}

func (m setupModel) updateSpinner(msg tea.Msg) (setupModel, tea.Cmd) {
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m setupModel) updateModel(msg tea.Msg) (setupModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			if m.modelCursor > 0 {
				m.modelCursor--
			}
			return m, nil
		case "down", "j":
			if m.modelCursor < len(m.models)-1 {
				m.modelCursor++
			}
			return m, nil
		case "enter":
			m.cfg.Models.Main.Model = m.models[m.modelCursor]
			m.step = stepDocker
			return m, nil
		}
	}
	return m, nil
}

func (m setupModel) updateDocker(msg tea.Msg) (setupModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			m.dockerCursor = 0
			return m, nil
		case "down", "j":
			m.dockerCursor = 1
			return m, nil
		case "enter":
			if m.dockerCursor == 0 {
				if m.cfg.Shell.DockerImage == "" {
					m.cfg.Shell.DockerImage = "nik-shell-" + id.Short(4)
				}
			} else {
				m.cfg.Shell.DockerImage = ""
			}
			m.step = stepTimezone
			m.timezoneIn.Focus()
			return m, textinput.Blink
		}
	}
	return m, nil
}

func (m setupModel) updateTimezone(msg tea.Msg) (setupModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		val := strings.TrimSpace(m.timezoneIn.Value())
		if val == "" {
			m.err = fmt.Errorf("timezone is required")
			return m, nil
		}

		_, err := time.LoadLocation(val)
		if err == nil {
			m.cfg.Timezone = val
			m.resolvedTZ = ""
			m.step = stepWriting
			return m, writeConfigCmd(m.cfg, m.apiKeyIn.Value())
		}

		m.step = stepTZResolving
		m.err = nil
		m.resolvedTZ = ""
		return m, resolveTimezoneCmd(m.apiKeyIn.Value(), val)
	}

	var cmd tea.Cmd
	m.timezoneIn, cmd = m.timezoneIn.Update(msg)
	return m, cmd
}

func (m setupModel) updateDone(msg tea.Msg) (setupModel, tea.Cmd) {
	if !m.daemonWasAlive {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		_ = daemonctl.SignalDaemon(m.cfg.Home, syscall.SIGTERM)
		m.step = stepWaitDaemon
		return m, tea.Batch(m.spinner.Tick, daemonPollCmd(m.cfg.Home))
	}

	return m, nil
}

func (m setupModel) View() string {
	banner := titleStyle.Render("nik") + "\n\n"

	switch m.step {
	case stepWelcome:
		return m.viewWelcome(banner)
	case stepAuthChoice:
		return m.viewAuthChoice(banner)
	case stepCodexLogin:
		return m.viewCodexLogin(banner)
	case stepCodexDone:
		return m.viewCodexDone(banner)
	case stepAPIKey:
		return m.viewAPIKey(banner)
	case stepAPIKeyValidating:
		return banner + m.spinner.View() + " validating API key..."
	case stepModel:
		return m.viewModel(banner)
	case stepDocker:
		return m.viewDocker(banner)
	case stepTimezone:
		return m.viewTimezone(banner)
	case stepTZResolving:
		return banner + m.spinner.View() + " finding your timezone..."
	case stepWriting:
		return banner + m.spinner.View() + " writing config..."
	case stepDone:
		return m.viewDone(banner)
	case stepWaitDaemon:
		return m.viewDone(banner)
	}

	return banner
}

func (m setupModel) viewWelcome(banner string) string {
	s := banner
	s += "Welcome to " + titleStyle.Render("nik") + "\n\n"
	s += "He can't wait to meet you, so let's get these\n"
	s += "settings quickly out of the way.\n"
	s += dimStyle.Render("\npress enter to begin")
	return s
}

func (m setupModel) viewAuthChoice(banner string) string {
	s := banner
	s += labelStyle.Render("How do you connect to OpenAI?") + "\n"
	s += hintStyle.Render("nik needs access to OpenAI models to think, remember, and speak.") + "\n\n"

	sub := "  "
	api := "  "
	if m.authCursor == 0 {
		sub = promptStyle.Render("> ")
	} else {
		api = promptStyle.Render("> ")
	}

	s += sub + labelStyle.Render("ChatGPT Plus or Pro subscription") + " " + successStyle.Render("(recommended)") + "\n"
	s += "    " + hintStyle.Render("$20/mo (Plus) or $200/mo (Pro). Sign in with your") + "\n"
	s += "    " + hintStyle.Render("OpenAI account, or subscribe at chatgpt.com/upgrade") + "\n\n"

	s += api + labelStyle.Render("Pay-per-use API key") + "\n"
	s += "    " + hintStyle.Render("Pay only for what nik uses. Create a key at") + "\n"
	s += "    " + hintStyle.Render("platform.openai.com/api-keys — see pricing at openai.com/api/pricing") + "\n"

	s += dimStyle.Render("\nj/k to move, enter to select")
	return s
}

func (m setupModel) viewCodexLogin(banner string) string {
	s := banner
	if m.err != nil {
		s += errorStyle.Render("Sign-in failed: "+m.err.Error()) + "\n\n"
		s += dimStyle.Render("press enter to retry")
		return s
	}
	s += m.spinner.View() + " Opening your browser to sign in with OpenAI...\n\n"
	s += hintStyle.Render("This connects nik to your ChatGPT Plus or Pro subscription.")
	return s
}

func (m setupModel) viewCodexDone(banner string) string {
	s := banner
	s += successStyle.Render("Signed in successfully!") + "\n\n"
	s += hintStyle.Render("Your ChatGPT subscription is active and nik will use it") + "\n"
	s += hintStyle.Render("for its main model. No API credits needed for that part.") + "\n"
	s += dimStyle.Render("\npress enter to continue")
	return s
}

func (m setupModel) viewAPIKey(banner string) string {
	s := banner

	brainNote := "this applies to you"
	if m.hasSubscription {
		brainNote = "doesn't apply to you"
	}

	s += labelStyle.Render("OpenAI API Key") + "\n\n"
	s += hintStyle.Render("The API key is used for:") + "\n"
	s += hintStyle.Render("  - nik's brain when not using a subscription ("+brainNote+")") + "\n"
	s += hintStyle.Render("  - recall — searching long-term memory") + "\n"
	s += hintStyle.Render("  - text-to-speech") + "\n\n"
	s += hintStyle.Render("These features use cheap models and should cost only") + "\n"
	s += hintStyle.Render("a few cents a month unless you configure otherwise.") + "\n\n"
	s += hintStyle.Render("Get one at platform.openai.com/api-keys") + "\n\n"
	s += m.apiKeyIn.View() + "\n"
	if m.err != nil {
		s += errorStyle.Render(m.err.Error()) + "\n"
	}
	s += dimStyle.Render("\npress enter to validate")
	return s
}

func (m setupModel) viewModel(banner string) string {
	s := banner
	s += labelStyle.Render("Choose a model") + "\n"
	s += hintStyle.Render("This is what nik thinks with day-to-day. You can change it later in config.yaml.") + "\n\n"

	recommended := m.cfg.Models.Main.Model
	for i, model := range m.models {
		cursor := "  "
		if i == m.modelCursor {
			cursor = promptStyle.Render("> ")
		}
		label := model
		if model == recommended {
			label += dimStyle.Render(" (recommended)")
		}
		s += cursor + label + "\n"
	}
	s += dimStyle.Render("\nj/k to move, enter to select")
	return s
}

func (m setupModel) viewDocker(banner string) string {
	s := banner
	s += labelStyle.Render("Shell sandbox") + "\n"
	s += hintStyle.Render("nik runs shell commands to search the web, manage files, and run code.") + "\n"
	s += hintStyle.Render("Docker keeps those commands sandboxed so they can't affect your system.") + "\n\n"

	docker := "  "
	host := "  "
	if m.dockerCursor == 0 {
		docker = promptStyle.Render("> ")
	} else {
		host = promptStyle.Render("> ")
	}

	s += docker + labelStyle.Render("Docker container") + " " + successStyle.Render("(recommended)") + "\n"
	s += "    " + hintStyle.Render("Commands run inside a Docker container. Requires Docker installed.") + "\n\n"

	s += host + labelStyle.Render("Run on host") + "\n"
	s += "    " + hintStyle.Render("Commands run directly on your machine via tmux.") + "\n"

	s += dimStyle.Render("\nj/k to move, enter to select")
	return s
}

func (m setupModel) viewTimezone(banner string) string {
	s := banner
	s += labelStyle.Render("Where you live") + "\n"
	s += hintStyle.Render("nik uses this for scheduling, alarms, and understanding") + "\n"
	s += hintStyle.Render("things like \"tomorrow morning\" or \"next Friday\".") + "\n\n"

	s += m.timezoneIn.View()

	if m.timezoneIn.Value() != "" && m.resolvedTZ == "" {
		_, err := time.LoadLocation(m.timezoneIn.Value())
		if err == nil {
			s += dimStyle.Render("  (detected from your system)")
		}
	}

	if m.resolvedTZ != "" {
		s += successStyle.Render("  (resolved)")
	}

	s += "\n"

	if m.err != nil {
		s += errorStyle.Render(m.err.Error()) + "\n"
	}

	s += dimStyle.Render("\npress enter to accept, or type your city and nik will find it")
	return s
}

func (m setupModel) viewDone(banner string) string {
	s := banner
	s += successStyle.Render("You're all set.") + "\n\n"

	if m.hasSubscription {
		s += "  OpenAI account    " + successStyle.Render("connected") + "\n"
	}
	s += "  API key           " + successStyle.Render("validated") + "\n"
	s += "  Model             " + m.cfg.Models.Main.Model + "\n"
	if m.cfg.Shell.DockerImage != "" {
		s += "  Shell sandbox     " + m.cfg.Shell.DockerImage + "\n"
	} else {
		s += "  Shell sandbox     " + dimStyle.Render("host (no Docker)") + "\n"
	}
	s += "  Timezone          " + m.cfg.Timezone + "\n\n"
	s += dimStyle.Render("config.yaml written to "+m.cfg.ConfigPath()) + "\n"
	s += dimStyle.Render("You can always edit it and nik will pick up changes live.") + "\n\n"

	if m.step == stepWaitDaemon {
		if m.daemonWasAlive && m.serviceInstalled {
			s += hintStyle.Render("nik is restarting...") + "\n"
		} else if m.daemonWasAlive {
			s += hintStyle.Render("nik has been stopped. Please restart it in your other terminal.") + "\n"
		}
		s += m.spinner.View() + " waiting for nik to start..."
		return s
	}

	if m.daemonWasAlive {
		s += hintStyle.Render("nik is running and needs to restart to pick up the new config.") + "\n"
		if m.serviceInstalled {
			s += hintStyle.Render("It will restart automatically via your system service.") + "\n"
		} else {
			s += hintStyle.Render("After stopping, please restart it manually in your other terminal.") + "\n"
		}
		s += dimStyle.Render("\npress enter to restart")
		return s
	}

	if m.serviceInstalled {
		s += hintStyle.Render("Start nik with your system service manager.") + "\n"
	} else {
		s += hintStyle.Render("Start nik with: nik daemon") + "\n"
	}
	s += "\n" + m.spinner.View() + " waiting for nik to start..."
	return s
}

func (m setupModel) isDone() bool {
	return m.completed
}
