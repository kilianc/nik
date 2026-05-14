package tui

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kciuffolo/nik/internal/codex"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/daemonctl"
	"github.com/kciuffolo/nik/internal/db"
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
	stepExaKey
	stepExaKeyValidating
	stepModel
	stepDocker
	stepTimezone
	stepLocationResolving
	stepWriting
	stepDone
	stepWaitDaemon
)

type setupModel struct {
	step         setupStep
	apiKeyIn     textinput.Model
	exaKeyIn     textinput.Model
	timezoneIn   textinput.Model
	codexPasteIn textinput.Model
	pulse        *pulse
	anims        animators
	err          error
	cfg          *config.Config
	conn         *sql.DB

	authCursor         int
	hasSubscription    bool
	codexAuthReq       *codex.AuthRequest
	codexBrowserOpened bool
	models             []string
	modelCursor        int
	dockerCursor       int

	daemonWasAlive   bool
	daemonOldPID     int
	serviceInstalled bool
	completed        bool
}

var subscriptionModels = []string{
	"gpt-5.3-codex",
}

var apiModels = []string{
	"gpt-5.4",
	"claude-sonnet-4-20250514",
	"claude-opus-4-20250514",
}

func modelsFor(backend string) []string {
	if backend == "subscription" {
		return subscriptionModels
	}
	return apiModels
}

func cursorFor(models []string, current string) int {
	for i, m := range models {
		if m == current {
			return i
		}
	}
	return 0
}

func newSetupModel(cfg *config.Config, conn *sql.DB) setupModel {
	apiKey := textinput.New()
	apiKey.Placeholder = "sk-..."
	apiKey.EchoMode = textinput.EchoPassword
	apiKey.Width = 60
	if existing, err := secrets.New(cfg.Home).Get("openai_key"); err == nil {
		apiKey.SetValue(existing)
	}

	exaKey := textinput.New()
	exaKey.Placeholder = "exa-..."
	exaKey.EchoMode = textinput.EchoPassword
	exaKey.Width = 60
	if existing, err := secrets.New(cfg.Home).Get("exa_api_key"); err == nil {
		exaKey.SetValue(existing)
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

	codexPasteIn := textinput.New()
	codexPasteIn.Placeholder = "http://localhost:1455/auth/callback?code=..."
	codexPasteIn.Width = 60

	p := newPulse(30 * time.Millisecond)
	return setupModel{
		step:         stepWelcome,
		apiKeyIn:     apiKey,
		exaKeyIn:     exaKey,
		timezoneIn:   tzIn,
		codexPasteIn: codexPasteIn,
		pulse:        p,
		anims:        animators{p},
		cfg:          cfg,
		conn:         conn,
	}
}

type codexLoginMsg struct {
	err error
}

type codexAuthReadyMsg struct {
	req           *codex.AuthRequest
	browserOpened bool
	err           error
}

type apiKeyValidatedMsg struct{ err error }
type exaKeyValidatedMsg struct{ err error }
type configWrittenMsg struct{ err error }

type locationResolvedMsg struct {
	timezone string
	location string
	err      error
}

type daemonPollMsg struct {
	pid   int
	alive bool
}

func codexAuthStartCmd() tea.Cmd {
	return func() tea.Msg {
		if _, err := codex.Load(""); err == nil {
			return codexLoginMsg{}
		}
		req, err := codex.PrepareLogin()
		if err != nil {
			return codexAuthReadyMsg{err: err}
		}
		return codexAuthReadyMsg{req: req, browserOpened: codex.OpenBrowser(req.AuthURL)}
	}
}

func codexCompleteLoginCmd(req *codex.AuthRequest, input string) tea.Cmd {
	return func() tea.Msg {
		_, err := req.Complete(input, "")
		return codexLoginMsg{err: err}
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

func validateExaKeyCmd(key string) tea.Cmd {
	return func() tea.Msg {
		body := `{"query":"test","type":"auto","numResults":1}`
		req, err := http.NewRequest("POST", "https://api.exa.ai/search", strings.NewReader(body))
		if err != nil {
			return exaKeyValidatedMsg{err: err}
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", key)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return exaKeyValidatedMsg{err: fmt.Errorf("connect to Exa: %w", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return exaKeyValidatedMsg{err: fmt.Errorf("invalid Exa API key (%d)", resp.StatusCode)}
		}

		if resp.StatusCode != http.StatusOK {
			return exaKeyValidatedMsg{err: fmt.Errorf("unexpected status %d", resp.StatusCode)}
		}

		return exaKeyValidatedMsg{}
	}
}

func resolveLocationCmd(apiKey, input string) tea.Cmd {
	return func() tea.Msg {
		body := map[string]any{
			"model": "gpt-4.1-nano",
			"instructions": "You map a place description to an IANA timezone and a canonical human-friendly location label. " +
				"Set confident=false if the input isn't a real place you recognize (gibberish, a person's name, 'my house', etc.).",
			"input":             input,
			"max_output_tokens": 80,
			"text": map[string]any{
				"format": map[string]any{
					"type":   "json_schema",
					"name":   "location",
					"strict": true,
					"schema": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"required":             []string{"timezone", "location", "confident"},
						"properties": map[string]any{
							"timezone":  map[string]any{"type": "string", "description": "IANA timezone, e.g. Europe/Rome. Empty string if not confident."},
							"location":  map[string]any{"type": "string", "description": "Canonical 'City, Country' or 'City, State' label. Empty string if not confident."},
							"confident": map[string]any{"type": "boolean"},
						},
					},
				},
			},
		}

		data, err := json.Marshal(body)
		if err != nil {
			return locationResolvedMsg{err: err}
		}

		req, err := http.NewRequest("POST", "https://api.openai.com/v1/responses", bytes.NewReader(data))
		if err != nil {
			return locationResolvedMsg{err: err}
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return locationResolvedMsg{err: fmt.Errorf("connect to OpenAI: %w", err)}
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return locationResolvedMsg{err: err}
		}

		if resp.StatusCode != http.StatusOK {
			return locationResolvedMsg{err: fmt.Errorf("OpenAI API error (%d): %s", resp.StatusCode, respBody)}
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
			return locationResolvedMsg{err: err}
		}

		var payload string
		if len(result.Output) > 0 && len(result.Output[0].Content) > 0 {
			payload = result.Output[0].Content[0].Text
		}
		if strings.TrimSpace(payload) == "" {
			return locationResolvedMsg{err: fmt.Errorf("no response from model")}
		}

		var parsed struct {
			Timezone  string `json:"timezone"`
			Location  string `json:"location"`
			Confident bool   `json:"confident"`
		}
		err = json.Unmarshal([]byte(payload), &parsed)
		if err != nil {
			return locationResolvedMsg{err: fmt.Errorf("parse model response: %w", err)}
		}

		if !parsed.Confident {
			return locationResolvedMsg{err: fmt.Errorf("could not find a place for %q, try a city and country", input)}
		}

		_, err = time.LoadLocation(parsed.Timezone)
		if err != nil {
			return locationResolvedMsg{err: fmt.Errorf("model returned an invalid timezone %q", parsed.Timezone)}
		}

		location := strings.TrimSpace(parsed.Location)
		if location == "" {
			return locationResolvedMsg{err: fmt.Errorf("model returned an empty location")}
		}

		return locationResolvedMsg{timezone: parsed.Timezone, location: location}
	}
}

func writeSetupCmd(cfg *config.Config, conn *sql.DB, apiKey, exaKey string) tea.Cmd {
	return func() tea.Msg {
		store := secrets.New(cfg.Home)

		if apiKey != "" {
			err := store.Set("openai_key", apiKey)
			if err != nil {
				return configWrittenMsg{err: err}
			}
		}

		if exaKey != "" {
			err := store.Set("exa_api_key", exaKey)
			if err != nil {
				return configWrittenMsg{err: err}
			}
		}

		cfg.Normalize()
		err := cfg.Save(cfg.ConfigPath())
		if err != nil {
			return configWrittenMsg{err: err}
		}

		if conn != nil {
			ctx := context.Background()
			for _, cid := range []string{db.OwnerContactID, db.NikContactID} {
				err = db.ContactUpdate(ctx, conn, db.ContactUpdateParams{ID: cid, Field: "timezone", Value: cfg.Timezone})
				if err != nil {
					return configWrittenMsg{err: fmt.Errorf("write timezone to %s: %w", cid, err)}
				}
				err = db.ContactUpdate(ctx, conn, db.ContactUpdateParams{ID: cid, Field: "location", Value: cfg.Location})
				if err != nil {
					return configWrittenMsg{err: fmt.Errorf("write location to %s: %w", cid, err)}
				}
			}
		}

		return configWrittenMsg{}
	}
}

func daemonPollCmd(home string) tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		pid, alive := daemonctl.CheckPID(home)
		return daemonPollMsg{pid: pid, alive: alive}
	})
}

func (m setupModel) Init() tea.Cmd {
	return m.pulse.SetActive(true)
}

func (m setupModel) Update(msg tea.Msg) (setupModel, tea.Cmd) {
	var cmds []tea.Cmd

	// Universal animator fanout. MUST stay at the top of Update so animation
	// messages can never be swallowed by a step handler delegating to
	// textinput.Update — that's how the spinner used to die mid-step.
	if cmd := m.anims.Update(msg); cmd != nil {
		cmds = append(cmds, cmd)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case codexAuthReadyMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = stepCodexLogin
			return m, tea.Batch(cmds...)
		}
		m.codexAuthReq = msg.req
		m.codexBrowserOpened = msg.browserOpened
		m.codexPasteIn.Reset()
		m.codexPasteIn.Focus()
		m.step = stepCodexLogin
		m.err = nil
		cmds = append(cmds, textinput.Blink)
		return m, tea.Batch(cmds...)

	case codexLoginMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = stepCodexLogin
			if m.codexAuthReq != nil {
				m.codexPasteIn.Focus()
				cmds = append(cmds, textinput.Blink)
			}
			return m, tea.Batch(cmds...)
		}
		m.codexAuthReq = nil
		m.hasSubscription = true
		m.err = nil
		m.step = stepCodexDone
		return m, tea.Batch(cmds...)

	case apiKeyValidatedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = stepAPIKey
			m.apiKeyIn.Focus()
			cmds = append(cmds, textinput.Blink)
			return m, tea.Batch(cmds...)
		}
		m.err = nil
		m.step = stepExaKey
		m.exaKeyIn.Focus()
		cmds = append(cmds, textinput.Blink)
		return m, tea.Batch(cmds...)

	case exaKeyValidatedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = stepExaKey
			m.exaKeyIn.Focus()
			cmds = append(cmds, textinput.Blink)
			return m, tea.Batch(cmds...)
		}
		m.err = nil
		m.step = stepModel
		return m, tea.Batch(cmds...)

	case locationResolvedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = stepTimezone
			m.timezoneIn.Focus()
			cmds = append(cmds, textinput.Blink)
			return m, tea.Batch(cmds...)
		}
		m.err = nil
		m.cfg.Timezone = msg.timezone
		m.cfg.Location = msg.location
		m.step = stepWriting
		cmds = append(cmds, writeSetupCmd(m.cfg, m.conn, m.apiKeyIn.Value(), m.exaKeyIn.Value()))
		return m, tea.Batch(cmds...)

	case configWrittenMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Batch(cmds...)
		}
		m.err = nil
		m.step = stepDone
		pid, alive := daemonctl.CheckPID(m.cfg.Home)
		m.daemonWasAlive = alive
		m.daemonOldPID = pid
		m.serviceInstalled = daemonctl.IsInstalled()
		if !alive {
			cmds = append(cmds, daemonPollCmd(m.cfg.Home))
		}
		return m, tea.Batch(cmds...)

	case daemonPollMsg:
		if msg.alive && msg.pid != m.daemonOldPID {
			m.completed = true
			return m, tea.Batch(cmds...)
		}
		cmds = append(cmds, daemonPollCmd(m.cfg.Home))
		return m, tea.Batch(cmds...)
	}

	var stepCmd tea.Cmd
	switch m.step {
	case stepWelcome:
		m, stepCmd = m.updateWelcome(msg)
	case stepAuthChoice:
		m, stepCmd = m.updateAuthChoice(msg)
	case stepCodexLogin:
		m, stepCmd = m.updateCodexLogin(msg)
	case stepCodexDone:
		m, stepCmd = m.updateCodexDone(msg)
	case stepAPIKey:
		m, stepCmd = m.updateAPIKey(msg)
	case stepExaKey:
		m, stepCmd = m.updateExaKey(msg)
	case stepModel:
		m, stepCmd = m.updateModel(msg)
	case stepDocker:
		m, stepCmd = m.updateDocker(msg)
	case stepTimezone:
		m, stepCmd = m.updateTimezone(msg)
	case stepDone:
		m, stepCmd = m.updateDone(msg)
	}
	cmds = append(cmds, stepCmd)
	return m, tea.Batch(cmds...)
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
				m.cfg.Models.Main.Backend = "subscription"
				m.models = modelsFor("subscription")
				m.modelCursor = cursorFor(m.models, m.cfg.Models.Main.Model)
				m.cfg.Models.Main.Model = m.models[m.modelCursor]
				m.step = stepCodexLogin
				return m, codexAuthStartCmd()
			}
			m.cfg.Models.Main.Backend = "api"
			m.models = modelsFor("api")
			m.modelCursor = cursorFor(m.models, m.cfg.Models.Main.Model)
			m.cfg.Models.Main.Model = m.models[m.modelCursor]
			m.step = stepAPIKey
			m.apiKeyIn.Focus()
			return m, textinput.Blink
		}
	}
	return m, nil
}

func (m setupModel) updateCodexLogin(msg tea.Msg) (setupModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.codexAuthReq = nil
			m.codexPasteIn.Blur()
			m.err = nil
			m.step = stepAuthChoice
			return m, nil
		case "enter":
			if m.codexAuthReq == nil {
				m.err = nil
				return m, codexAuthStartCmd()
			}
			val := strings.TrimSpace(m.codexPasteIn.Value())
			if val == "" {
				m.err = fmt.Errorf("paste the URL from your browser after signing in")
				return m, nil
			}
			m.err = nil
			return m, codexCompleteLoginCmd(m.codexAuthReq, val)
		}
	}

	if m.codexAuthReq != nil {
		var cmd tea.Cmd
		m.codexPasteIn, cmd = m.codexPasteIn.Update(msg)
		return m, cmd
	}
	return m, nil
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

func (m setupModel) updateExaKey(msg tea.Msg) (setupModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
		val := m.exaKeyIn.Value()
		if val == "" {
			m.err = fmt.Errorf("Exa API key is required")
			return m, nil
		}
		m.step = stepExaKeyValidating
		m.err = nil
		return m, validateExaKeyCmd(val)
	}

	var cmd tea.Cmd
	m.exaKeyIn, cmd = m.exaKeyIn.Update(msg)
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
			m.err = fmt.Errorf("please tell nik where you live")
			return m, nil
		}

		m.step = stepLocationResolving
		m.err = nil
		return m, resolveLocationCmd(m.apiKeyIn.Value(), val)
	}

	var cmd tea.Cmd
	m.timezoneIn, cmd = m.timezoneIn.Update(msg)
	return m, cmd
}

func (m setupModel) updateDone(msg tea.Msg) (setupModel, tea.Cmd) {
	if m.daemonWasAlive {
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
			_ = daemonctl.SignalDaemon(m.cfg.Home, syscall.SIGTERM)
			m.step = stepWaitDaemon
			return m, daemonPollCmd(m.cfg.Home)
		}
	}
	return m, nil
}

func (m setupModel) View() string {
	switch m.step {
	case stepWelcome:
		return m.viewWelcome()
	case stepAuthChoice:
		return m.viewAuthChoice()
	case stepCodexLogin:
		return m.viewCodexLogin()
	case stepCodexDone:
		return m.viewCodexDone()
	case stepAPIKey:
		return m.viewAPIKey()
	case stepAPIKeyValidating:
		return m.spinnerView() + " validating API key..."
	case stepExaKey:
		return m.viewExaKey()
	case stepExaKeyValidating:
		return m.spinnerView() + " validating Exa key..."
	case stepModel:
		return m.viewModel()
	case stepDocker:
		return m.viewDocker()
	case stepTimezone:
		return m.viewTimezone()
	case stepLocationResolving:
		return m.spinnerView() + " finding where you are..."
	case stepWriting:
		return m.spinnerView() + " writing config..."
	case stepDone:
		return m.viewDone()
	case stepWaitDaemon:
		return m.viewDone()
	}

	return ""
}

func (m setupModel) viewWelcome() string {
	s := "Welcome to " + titleStyle.Render("nik") + "\n\n"
	s += "He can't wait to meet you, so let's get these\n"
	s += "settings quickly out of the way.\n"
	s += dimStyle.Render("\npress enter to begin")
	return s
}

func (m setupModel) viewAuthChoice() string {
	s := labelStyle.Render("How do you connect to OpenAI?") + "\n"
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

func (m setupModel) viewCodexLogin() string {
	if m.codexAuthReq == nil {
		s := m.spinnerView() + " Preparing OpenAI sign-in...\n\n"
		s += hintStyle.Render("This connects nik to your ChatGPT Plus or Pro subscription.") + "\n"
		if m.err != nil {
			s += "\n" + errorStyle.Render(m.err.Error()) + "\n"
			s += dimStyle.Render("\npress enter to retry, esc to go back")
		}
		return s
	}

	s := labelStyle.Render("Sign in with OpenAI") + "\n\n"
	if m.codexBrowserOpened {
		s += hintStyle.Render("1. Your browser should have opened to OpenAI. If it didn't,") + "\n"
		s += hintStyle.Render("   open this URL manually:") + "\n\n"
	} else {
		s += hintStyle.Render("1. Open this URL in your browser and sign in:") + "\n\n"
	}
	s += "   " + m.codexAuthReq.AuthURL + "\n\n"
	s += hintStyle.Render("2. After signing in your browser will fail to load a") + "\n"
	s += hintStyle.Render("   localhost page — that's expected. Copy that URL from") + "\n"
	s += hintStyle.Render("   the address bar and paste it below:") + "\n\n"
	s += m.codexPasteIn.View() + "\n"
	if m.err != nil {
		s += errorStyle.Render(m.err.Error()) + "\n"
	}
	s += dimStyle.Render("\npress enter to continue, esc to go back")
	return s
}

func (m setupModel) viewCodexDone() string {
	s := successStyle.Render("Signed in successfully!") + "\n\n"
	s += hintStyle.Render("Your ChatGPT subscription is active and nik will use it") + "\n"
	s += hintStyle.Render("for its main model. No API credits needed for that part.") + "\n"
	s += dimStyle.Render("\npress enter to continue")
	return s
}

func (m setupModel) viewAPIKey() string {
	brainNote := "this applies to you"
	if m.hasSubscription {
		brainNote = "doesn't apply to you"
	}

	s := labelStyle.Render("OpenAI API Key") + "\n\n"
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

func (m setupModel) viewExaKey() string {
	s := labelStyle.Render("Exa API Key") + "\n\n"
	s += hintStyle.Render("nik uses Exa to search the web. This powers research,") + "\n"
	s += hintStyle.Render("fact-checking, and staying up to date on current events.") + "\n\n"
	s += hintStyle.Render("Exa has a free tier with 1000 searches/month, more than") + "\n"
	s += hintStyle.Render("enough for personal use. Sign up at https://dashboard.exa.ai and") + "\n"
	s += hintStyle.Render("create an API key under your account settings.") + "\n\n"
	s += m.exaKeyIn.View() + "\n"
	if m.err != nil {
		s += errorStyle.Render(m.err.Error()) + "\n"
	}
	s += dimStyle.Render("\npress enter to validate")
	return s
}

func (m setupModel) viewModel() string {
	s := labelStyle.Render("Choose a model") + "\n"
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

func (m setupModel) viewDocker() string {
	s := labelStyle.Render("Shell sandbox") + "\n"
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

func (m setupModel) viewTimezone() string {
	s := labelStyle.Render("Where you live") + "\n"
	s += hintStyle.Render("nik uses this for scheduling, alarms, and understanding") + "\n"
	s += hintStyle.Render("things like \"tomorrow morning\" or \"next Friday\".") + "\n\n"

	s += m.timezoneIn.View() + "\n"

	if m.err != nil {
		s += errorStyle.Render(m.err.Error()) + "\n"
	}

	s += dimStyle.Render("\ntype your city and country (e.g. \"Rome, Italy\"), then press enter")
	return s
}

func (m setupModel) viewDone() string {
	s := successStyle.Render("You're all set.") + "\n\n"

	if m.hasSubscription {
		s += "  OpenAI account    " + successStyle.Render("connected") + "\n"
	}
	s += "  API key           " + successStyle.Render("validated") + "\n"
	s += "  Exa key           " + successStyle.Render("validated") + "\n"
	s += "  Model             " + m.cfg.Models.Main.Model + "\n"
	if m.cfg.Shell.DockerImage != "" {
		s += "  Shell sandbox     " + m.cfg.Shell.DockerImage + "\n"
	} else {
		s += "  Shell sandbox     " + dimStyle.Render("host (no Docker)") + "\n"
	}
	s += "  Timezone          " + m.cfg.Timezone + "\n"
	s += "  Location          " + m.cfg.Location + "\n\n"
	s += dimStyle.Render("config.yaml written to "+m.cfg.ConfigPath()) + "\n"
	s += dimStyle.Render("You can always edit it and nik will pick up changes live.") + "\n\n"

	if m.step == stepWaitDaemon {
		if m.daemonWasAlive && m.serviceInstalled {
			s += hintStyle.Render("nik is restarting...") + "\n"
		} else if m.daemonWasAlive {
			s += hintStyle.Render("nik has been stopped. Please restart it in your other terminal.") + "\n"
		}
		s += m.spinnerView() + " waiting for nik to start..."
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
	s += "\n" + m.spinnerView() + " waiting for nik to start..."
	return s
}

func (m setupModel) spinnerView() string {
	return spinnerColor.Render(spinnerGlyph(m.pulse.Tick()))
}

func (m setupModel) isDone() bool {
	return m.completed
}
