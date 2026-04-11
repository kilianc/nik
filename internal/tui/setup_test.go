package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kciuffolo/nik/internal/config"
)

func TestSetupInitialStep(t *testing.T) {
	w := newTestSetup(t)

	if w.step != stepWelcome {
		t.Errorf("expected initial step stepWelcome, got %d", w.step)
	}
}

func TestSetupWelcomeToAuthChoice(t *testing.T) {
	w := newTestSetup(t)

	w, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if w.step != stepAuthChoice {
		t.Errorf("expected step stepAuthChoice, got %d", w.step)
	}
}

func TestSetupAuthChoiceSubscription(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepAuthChoice
	w.authCursor = 0

	w, cmd := w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if w.step != stepCodexLogin {
		t.Errorf("expected step stepCodexLogin, got %d", w.step)
	}
	if cmd == nil {
		t.Error("expected cmd for codex login")
	}
}

func TestSetupAuthChoiceAPIKey(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepAuthChoice
	w.authCursor = 1

	w, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if w.step != stepAPIKey {
		t.Errorf("expected step stepAPIKey, got %d", w.step)
	}
}

func TestSetupAuthChoiceNavigation(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepAuthChoice

	w, _ = w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if w.authCursor != 1 {
		t.Errorf("expected cursor 1 after j, got %d", w.authCursor)
	}

	w, _ = w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if w.authCursor != 0 {
		t.Errorf("expected cursor 0 after k, got %d", w.authCursor)
	}
}

func TestSetupCodexLoginSuccess(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepCodexLogin

	w, _ = w.Update(codexLoginMsg{})

	if w.step != stepCodexDone {
		t.Errorf("expected step stepCodexDone, got %d", w.step)
	}
	if !w.hasSubscription {
		t.Error("expected hasSubscription to be true")
	}
}

func TestSetupCodexLoginFail(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepCodexLogin

	w, _ = w.Update(codexLoginMsg{err: errTest})

	if w.step != stepCodexLogin {
		t.Errorf("expected step to stay at stepCodexLogin, got %d", w.step)
	}
	if w.err == nil {
		t.Error("expected error to be set")
	}
}

func TestSetupCodexLoginRetry(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepCodexLogin
	w.err = errTest

	w, cmd := w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if w.err != nil {
		t.Error("expected error to be cleared")
	}
	if cmd == nil {
		t.Error("expected cmd for retry")
	}
}

func TestSetupCodexDoneToAPIKey(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepCodexDone

	w, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if w.step != stepAPIKey {
		t.Errorf("expected step stepAPIKey, got %d", w.step)
	}
}

func TestSetupAPIKeyRequired(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepAPIKey

	w, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if w.step != stepAPIKey {
		t.Errorf("expected step to stay at stepAPIKey, got %d", w.step)
	}
	if w.err == nil {
		t.Error("expected error for empty API key")
	}
}

func TestSetupAPIKeySubmit(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepAPIKey
	w.apiKeyIn.SetValue("sk-test")
	w.apiKeyIn.Focus()

	w, cmd := w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if w.step != stepAPIKeyValidating {
		t.Errorf("expected step stepAPIKeyValidating, got %d", w.step)
	}
	if cmd == nil {
		t.Error("expected validation cmd")
	}
}

func TestSetupAPIKeyValidationFail(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepAPIKeyValidating

	w, _ = w.Update(apiKeyValidatedMsg{err: errTest})

	if w.step != stepAPIKey {
		t.Errorf("expected step to revert to stepAPIKey, got %d", w.step)
	}
	if w.err == nil {
		t.Error("expected error to be set")
	}
}

func TestSetupAPIKeyValidationSuccess(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepAPIKeyValidating

	w, _ = w.Update(apiKeyValidatedMsg{})

	if w.step != stepModel {
		t.Errorf("expected step to advance to stepModel, got %d", w.step)
	}
	if w.err != nil {
		t.Errorf("expected no error, got %v", w.err)
	}
}

func TestSetupModelSelection(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepModel

	w, _ = w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if w.modelCursor != 1 {
		t.Errorf("expected cursor 1, got %d", w.modelCursor)
	}

	w, _ = w.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if w.modelCursor != 0 {
		t.Errorf("expected cursor 0, got %d", w.modelCursor)
	}
}

func TestSetupModelSelectAdvances(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepModel

	w, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if w.step != stepDocker {
		t.Errorf("expected step stepDocker, got %d", w.step)
	}
	if w.cfg.Models.Main.Model != defaultModels[0] {
		t.Errorf("expected model %q, got %q", defaultModels[0], w.cfg.Models.Main.Model)
	}
}

func TestSetupDockerSelectDocker(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepDocker
	w.dockerCursor = 0

	w, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if w.step != stepTimezone {
		t.Errorf("expected step stepTimezone, got %d", w.step)
	}
	if w.cfg.Shell.DockerImage == "" {
		t.Error("expected docker image to be set")
	}
	if !strings.HasPrefix(w.cfg.Shell.DockerImage, "nik-shell-") {
		t.Errorf("expected docker image to start with nik-shell-, got %q", w.cfg.Shell.DockerImage)
	}
}

func TestSetupDockerSelectHost(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepDocker
	w.dockerCursor = 1

	w, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if w.step != stepTimezone {
		t.Errorf("expected step stepTimezone, got %d", w.step)
	}
	if w.cfg.Shell.DockerImage != "" {
		t.Errorf("expected empty docker image for host, got %q", w.cfg.Shell.DockerImage)
	}
}

func TestSetupDockerPreservesExisting(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepDocker
	w.dockerCursor = 0
	w.cfg.Shell.DockerImage = "custom-image"

	w, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if w.cfg.Shell.DockerImage != "custom-image" {
		t.Errorf("expected existing docker image preserved, got %q", w.cfg.Shell.DockerImage)
	}
}

func TestSetupTimezoneValidIANA(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepTimezone
	w.timezoneIn.SetValue("America/New_York")
	w.apiKeyIn.SetValue("sk-test")

	w, cmd := w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if w.step != stepWriting {
		t.Errorf("expected step stepWriting, got %d", w.step)
	}
	if w.cfg.Timezone != "America/New_York" {
		t.Errorf("expected timezone America/New_York, got %q", w.cfg.Timezone)
	}
	if cmd == nil {
		t.Error("expected config write cmd")
	}
}

func TestSetupTimezoneResolvesCity(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepTimezone
	w.timezoneIn.SetValue("Rome")
	w.apiKeyIn.SetValue("sk-test")

	w, cmd := w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if w.step != stepTZResolving {
		t.Errorf("expected step stepTZResolving, got %d", w.step)
	}
	if cmd == nil {
		t.Error("expected resolve cmd")
	}
}

func TestSetupTZResolvedSuccess(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepTZResolving

	w, _ = w.Update(tzResolvedMsg{timezone: "Europe/Rome"})

	if w.step != stepTimezone {
		t.Errorf("expected step stepTimezone, got %d", w.step)
	}
	if w.resolvedTZ != "Europe/Rome" {
		t.Errorf("expected resolvedTZ Europe/Rome, got %q", w.resolvedTZ)
	}
	if w.timezoneIn.Value() != "Europe/Rome" {
		t.Errorf("expected input to be Europe/Rome, got %q", w.timezoneIn.Value())
	}
}

func TestSetupTZResolvedFail(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepTZResolving

	w, _ = w.Update(tzResolvedMsg{err: errTest})

	if w.step != stepTimezone {
		t.Errorf("expected step stepTimezone, got %d", w.step)
	}
	if w.err == nil {
		t.Error("expected error to be set")
	}
}

func TestSetupConfigWriteSuccess(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepWriting

	w, _ = w.Update(configWrittenMsg{})

	if w.step != stepDone {
		t.Errorf("expected step stepDone, got %d", w.step)
	}
}

func TestSetupConfigWriteFail(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepWriting

	w, _ = w.Update(configWrittenMsg{err: errTest})

	if w.err == nil {
		t.Error("expected error to be set")
	}
}

func TestSetupDaemonPollComplete(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepWaitDaemon
	w.daemonOldPID = 100

	w, _ = w.Update(daemonPollMsg{pid: 200, alive: true})

	if !w.completed {
		t.Error("expected completed to be true")
	}
}

func TestSetupDaemonPollSamePID(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepWaitDaemon
	w.daemonOldPID = 100

	w, cmd := w.Update(daemonPollMsg{pid: 100, alive: true})

	if w.completed {
		t.Error("expected completed to be false for same PID")
	}
	if cmd == nil {
		t.Error("expected retry poll cmd")
	}
}

func TestSetupDaemonPollNotAlive(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepWaitDaemon
	w.daemonOldPID = 100

	w, cmd := w.Update(daemonPollMsg{alive: false})

	if w.completed {
		t.Error("expected completed to be false")
	}
	if cmd == nil {
		t.Error("expected retry poll cmd")
	}
}

func TestSetupIsDone(t *testing.T) {
	w := newTestSetup(t)

	if w.isDone() {
		t.Error("setup should not be done initially")
	}

	w.completed = true
	if !w.isDone() {
		t.Error("setup should be done when completed")
	}
}

func TestSetupViewWelcome(t *testing.T) {
	w := newTestSetup(t)

	view := w.View()

	if !strings.Contains(view, "Welcome") {
		t.Error("expected Welcome in view")
	}
	if !strings.Contains(view, "press enter to begin") {
		t.Error("expected press enter prompt in view")
	}
}

func TestSetupViewAuthChoice(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepAuthChoice

	view := w.View()

	if !strings.Contains(view, "OpenAI") {
		t.Error("expected OpenAI in view")
	}
	if !strings.Contains(view, "subscription") {
		t.Error("expected subscription in view")
	}
	if !strings.Contains(view, "API key") {
		t.Error("expected API key in view")
	}
}

func TestSetupViewAPIKeyWithSubscription(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepAPIKey
	w.hasSubscription = true

	view := w.View()

	if !strings.Contains(view, "doesn't apply to you") {
		t.Error("expected subscription note in view")
	}
	if !strings.Contains(view, "recall") {
		t.Error("expected recall mention in view")
	}
}

func TestSetupViewAPIKeyWithoutSubscription(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepAPIKey
	w.hasSubscription = false

	view := w.View()

	if !strings.Contains(view, "this applies to you") {
		t.Error("expected applies note in view")
	}
}

func TestSetupViewModelRecommended(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepModel

	view := w.View()

	if !strings.Contains(view, "(recommended)") {
		t.Error("expected (recommended) label in model view")
	}
}

func TestSetupViewDoneSummary(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepDone
	w.hasSubscription = true
	w.cfg.Models.Main.Model = "gpt-5.4"
	w.cfg.Timezone = "America/New_York"

	view := w.View()

	if !strings.Contains(view, "all set") {
		t.Error("expected completion message in view")
	}
	if !strings.Contains(view, "connected") {
		t.Error("expected OpenAI account connected in view")
	}
	if !strings.Contains(view, "gpt-5.4") {
		t.Error("expected model in view")
	}
	if !strings.Contains(view, "America/New_York") {
		t.Error("expected timezone in view")
	}
}

func newTestSetup(t *testing.T) setupModel {
	t.Helper()
	cfg := config.Default(t.TempDir())
	return newSetupModel(cfg)
}

var errTest = &testError{"test error"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
