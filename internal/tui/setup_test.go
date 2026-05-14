package tui

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kciuffolo/nik/internal/codex"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
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

func TestSetupCodexAuthReadyFocusesPaste(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepCodexLogin
	req := &codex.AuthRequest{AuthURL: "https://auth.openai.com/oauth/authorize?x=1"}

	w, _ = w.Update(codexAuthReadyMsg{req: req, browserOpened: true})

	if w.codexAuthReq != req {
		t.Error("expected codexAuthReq to be stored on the model")
	}
	if !w.codexBrowserOpened {
		t.Error("expected codexBrowserOpened to be true")
	}
	if !w.codexPasteIn.Focused() {
		t.Error("expected paste input to be focused")
	}
	if w.err != nil {
		t.Errorf("expected no error, got %v", w.err)
	}
}

func TestSetupCodexPasteEmptyShowsError(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepCodexLogin
	w.codexAuthReq = &codex.AuthRequest{AuthURL: "https://example.com"}
	w.codexPasteIn.Focus()

	w, cmd := w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if w.err == nil {
		t.Error("expected error for empty paste")
	}
	if cmd != nil {
		t.Error("expected no cmd when paste is empty")
	}
}

func TestSetupCodexPasteFiresComplete(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepCodexLogin
	w.codexAuthReq = &codex.AuthRequest{AuthURL: "https://example.com"}
	w.codexPasteIn.Focus()
	w.codexPasteIn.SetValue("http://localhost:1455/auth/callback?code=abc&state=xyz")

	w, cmd := w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("expected complete-login cmd")
	}
	if w.err != nil {
		t.Errorf("expected no error, got %v", w.err)
	}
}

func TestSetupCodexEscCancels(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepCodexLogin
	w.codexAuthReq = &codex.AuthRequest{AuthURL: "https://example.com"}
	w.err = errTest

	w, _ = w.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if w.step != stepAuthChoice {
		t.Errorf("expected step stepAuthChoice after esc, got %d", w.step)
	}
	if w.codexAuthReq != nil {
		t.Error("expected codexAuthReq to be cleared")
	}
	if w.err != nil {
		t.Error("expected error to be cleared")
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

	if w.step != stepExaKey {
		t.Errorf("expected step to advance to stepExaKey, got %d", w.step)
	}
	if w.err != nil {
		t.Errorf("expected no error, got %v", w.err)
	}
}

func TestSetupExaKeyRequired(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepExaKey

	w, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if w.step != stepExaKey {
		t.Errorf("expected step to stay at stepExaKey, got %d", w.step)
	}
	if w.err == nil {
		t.Error("expected error for empty Exa key")
	}
}

func TestSetupExaKeySubmit(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepExaKey
	w.exaKeyIn.SetValue("exa-test")
	w.exaKeyIn.Focus()

	w, cmd := w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if w.step != stepExaKeyValidating {
		t.Errorf("expected step stepExaKeyValidating, got %d", w.step)
	}
	if cmd == nil {
		t.Error("expected validation cmd")
	}
}

func TestSetupExaKeyValidationSuccess(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepExaKeyValidating

	w, _ = w.Update(exaKeyValidatedMsg{})

	if w.step != stepModel {
		t.Errorf("expected step stepModel, got %d", w.step)
	}
	if w.err != nil {
		t.Errorf("expected no error, got %v", w.err)
	}
}

func TestSetupExaKeyValidationFail(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepExaKeyValidating

	w, _ = w.Update(exaKeyValidatedMsg{err: errTest})

	if w.step != stepExaKey {
		t.Errorf("expected step to revert to stepExaKey, got %d", w.step)
	}
	if w.err == nil {
		t.Error("expected error to be set")
	}
}

func TestSetupViewExaKey(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepExaKey

	view := w.View()

	if !strings.Contains(view, "Exa API Key") {
		t.Error("expected Exa API Key in view")
	}
	if !strings.Contains(view, "free tier") {
		t.Error("expected free tier mention in view")
	}
	if !strings.Contains(view, "dashboard.exa.ai") {
		t.Error("expected signup URL in view")
	}
}

func TestSetupModelSelection(t *testing.T) {
	w := newTestSetup(t)
	w.models = apiModels
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
	w.models = apiModels
	w.step = stepModel

	w, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if w.step != stepDocker {
		t.Errorf("expected step stepDocker, got %d", w.step)
	}
	if w.cfg.Models.Main.Model != apiModels[0] {
		t.Errorf("expected model %q, got %q", apiModels[0], w.cfg.Models.Main.Model)
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

func TestSetupTimezoneRequiresInput(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepTimezone
	w.timezoneIn.SetValue("")

	w, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if w.step != stepTimezone {
		t.Errorf("expected step to stay at stepTimezone, got %d", w.step)
	}
	if w.err == nil {
		t.Error("expected error for empty input")
	}
}

func TestSetupTimezoneAlwaysResolves(t *testing.T) {
	cases := map[string]string{
		"iana":  "America/New_York",
		"city":  "Rome",
		"state": "California, USA",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			w := newTestSetup(t)
			w.step = stepTimezone
			w.timezoneIn.SetValue(input)
			w.apiKeyIn.SetValue("sk-test")

			w, cmd := w.Update(tea.KeyMsg{Type: tea.KeyEnter})

			if w.step != stepLocationResolving {
				t.Errorf("expected step stepLocationResolving, got %d", w.step)
			}
			if cmd == nil {
				t.Error("expected resolve cmd")
			}
		})
	}
}

func TestSetupLocationResolvedSuccess(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepLocationResolving

	w, cmd := w.Update(locationResolvedMsg{timezone: "Europe/Rome", location: "Rome, Italy"})

	if w.step != stepWriting {
		t.Errorf("expected step stepWriting, got %d", w.step)
	}
	if w.cfg.Timezone != "Europe/Rome" {
		t.Errorf("expected timezone Europe/Rome, got %q", w.cfg.Timezone)
	}
	if w.cfg.Location != "Rome, Italy" {
		t.Errorf("expected location Rome, Italy, got %q", w.cfg.Location)
	}
	if cmd == nil {
		t.Error("expected write cmd")
	}
}

func TestSetupLocationResolvedFail(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepLocationResolving

	w, _ = w.Update(locationResolvedMsg{err: errTest})

	if w.step != stepTimezone {
		t.Errorf("expected step stepTimezone, got %d", w.step)
	}
	if w.err == nil {
		t.Error("expected error to be set")
	}
}

func TestSetupWriteFansOutToContacts(t *testing.T) {
	cfg := config.Default(t.TempDir())
	cfg.Timezone = "Europe/Rome"
	cfg.Location = "Rome, Italy"
	conn := newTestDB(t)

	msg := writeSetupCmd(cfg, conn, "", "")()

	written, ok := msg.(configWrittenMsg)
	if !ok {
		t.Fatalf("expected configWrittenMsg, got %T", msg)
	}
	if written.err != nil {
		t.Fatalf("write failed: %v", written.err)
	}

	ctx := context.Background()
	for _, cid := range []string{db.OwnerContactID, db.NikContactID} {
		c, err := db.ContactGet(ctx, conn, cid)
		if err != nil {
			t.Fatalf("get contact %s: %v", cid, err)
		}
		if !c.Timezone.Valid || c.Timezone.String != "Europe/Rome" {
			t.Errorf("%s timezone = %v, want Europe/Rome", cid, c.Timezone)
		}
		if !c.Location.Valid || c.Location.String != "Rome, Italy" {
			t.Errorf("%s location = %v, want Rome, Italy", cid, c.Location)
		}
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
	w.models = apiModels
	w.cfg.Models.Main.Model = apiModels[0]
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
	w.cfg.Location = "New York, NY"

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
	if !strings.Contains(view, "New York, NY") {
		t.Error("expected location in view")
	}
}

func newTestSetup(t *testing.T) setupModel {
	t.Helper()
	cfg := config.Default(t.TempDir())
	conn := newTestDB(t)
	return newSetupModel(cfg, conn)
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	ctx := context.Background()
	if err := db.NikContactEnsure(ctx, conn); err != nil {
		t.Fatalf("ensure nik contact: %v", err)
	}
	if err := db.OwnerContactEnsure(ctx, conn); err != nil {
		t.Fatalf("ensure owner contact: %v", err)
	}
	return conn
}

var errTest = &testError{"test error"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
