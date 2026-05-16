package tui

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kciuffolo/nik/internal/codex"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

func TestSetupWelcomeToAuthChoice(t *testing.T) {
	w := newTestSetup(t)

	w, _ = w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if w.step != stepAuthChoice {
		t.Errorf("expected step stepAuthChoice, got %d", w.step)
	}
}

func TestSetupAuthChoiceSubscription(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepAuthChoice
	w.authCursor = 0

	w, cmd := w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

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

	w, _ = w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if w.step != stepAPIKey {
		t.Errorf("expected step stepAPIKey, got %d", w.step)
	}
}

func TestSetupAuthChoiceNavigation(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepAuthChoice

	w, _ = w.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if w.authCursor != 1 {
		t.Errorf("expected cursor 1 after j, got %d", w.authCursor)
	}

	w, _ = w.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
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

	w, cmd := w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

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

	w, cmd := w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

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

	w, _ = w.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

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

func TestSetupCodexLoginRetry(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepCodexLogin
	w.err = errTest

	w, cmd := w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

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

	w, _ = w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if w.step != stepAPIKey {
		t.Errorf("expected step stepAPIKey, got %d", w.step)
	}
}

func TestSetupRequiredFieldsBlock(t *testing.T) {
	cases := []struct {
		name string
		step setupStep
	}{
		{"api key", stepAPIKey},
		{"exa key", stepExaKey},
		{"timezone", stepTimezone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newTestSetup(t)
			w.step = tc.step

			w, _ = w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

			if w.step != tc.step {
				t.Errorf("step = %d, want %d (no advance on empty input)", w.step, tc.step)
			}
			if w.err == nil {
				t.Error("expected error for empty input")
			}
		})
	}
}

func TestSetupAPIKeySubmit(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepAPIKey
	w.apiKeyIn.SetValue("sk-test")
	w.apiKeyIn.Focus()

	w, cmd := w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if w.step != stepAPIKeyValidating {
		t.Errorf("expected step stepAPIKeyValidating, got %d", w.step)
	}
	if cmd == nil {
		t.Error("expected validation cmd")
	}
}

func TestSetupExaKeySubmit(t *testing.T) {
	w := newTestSetup(t)
	w.step = stepExaKey
	w.exaKeyIn.SetValue("exa-test")
	w.exaKeyIn.Focus()

	w, cmd := w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if w.step != stepExaKeyValidating {
		t.Errorf("expected step stepExaKeyValidating, got %d", w.step)
	}
	if cmd == nil {
		t.Error("expected validation cmd")
	}
}

func TestSetupStepTransitions(t *testing.T) {
	cases := []struct {
		name      string
		startStep setupStep
		msg       tea.Msg
		wantStep  setupStep
		wantErr   bool
	}{
		{"codex login fail stays", stepCodexLogin, codexLoginMsg{err: errTest}, stepCodexLogin, true},
		{"api key validation success advances", stepAPIKeyValidating, apiKeyValidatedMsg{}, stepExaKey, false},
		{"api key validation fail reverts", stepAPIKeyValidating, apiKeyValidatedMsg{err: errTest}, stepAPIKey, true},
		{"exa key validation success advances", stepExaKeyValidating, exaKeyValidatedMsg{}, stepModel, false},
		{"exa key validation fail reverts", stepExaKeyValidating, exaKeyValidatedMsg{err: errTest}, stepExaKey, true},
		{"location resolved fail reverts", stepLocationResolving, locationResolvedMsg{err: errTest}, stepTimezone, true},
		{"config write success advances", stepWriting, configWrittenMsg{}, stepDone, false},
		{"config write fail keeps step", stepWriting, configWrittenMsg{err: errTest}, stepWriting, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newTestSetup(t)
			w.step = tc.startStep

			w, _ = w.Update(tc.msg)

			if w.step != tc.wantStep {
				t.Errorf("step = %d, want %d", w.step, tc.wantStep)
			}
			if (w.err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr = %v", w.err, tc.wantErr)
			}
		})
	}
}

func TestSetupModelSelection(t *testing.T) {
	w := newTestSetup(t)
	w.models = apiModels
	w.step = stepModel

	w, _ = w.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if w.modelCursor != 1 {
		t.Errorf("expected cursor 1, got %d", w.modelCursor)
	}

	w, _ = w.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if w.modelCursor != 0 {
		t.Errorf("expected cursor 0, got %d", w.modelCursor)
	}
}

func TestSetupModelSelectAdvances(t *testing.T) {
	w := newTestSetup(t)
	w.models = apiModels
	w.step = stepModel

	w, _ = w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

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

	w, _ = w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

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

	w, _ = w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

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

	w, _ = w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if w.cfg.Shell.DockerImage != "custom-image" {
		t.Errorf("expected existing docker image preserved, got %q", w.cfg.Shell.DockerImage)
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

			w, cmd := w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

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

func TestSetupDaemonPoll(t *testing.T) {
	cases := []struct {
		name          string
		msg           daemonPollMsg
		wantCompleted bool
		wantRetry     bool
	}{
		{"new pid completes", daemonPollMsg{pid: 200, alive: true}, true, false},
		{"same pid retries", daemonPollMsg{pid: 100, alive: true}, false, true},
		{"not alive retries", daemonPollMsg{alive: false}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newTestSetup(t)
			w.step = stepWaitDaemon
			w.daemonOldPID = 100

			w, cmd := w.Update(tc.msg)

			if w.completed != tc.wantCompleted {
				t.Errorf("completed = %v, want %v", w.completed, tc.wantCompleted)
			}
			if tc.wantRetry && cmd == nil {
				t.Error("expected retry poll cmd")
			}
		})
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

func TestSetupView(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*setupModel)
		wants []string
	}{
		{
			name:  "welcome",
			setup: func(w *setupModel) {},
			wants: []string{"Welcome", "press enter to begin"},
		},
		{
			name:  "auth choice",
			setup: func(w *setupModel) { w.step = stepAuthChoice },
			wants: []string{"OpenAI", "subscription", "API key"},
		},
		{
			name:  "api key with subscription",
			setup: func(w *setupModel) { w.step = stepAPIKey; w.hasSubscription = true },
			wants: []string{"doesn't apply to you", "recall"},
		},
		{
			name:  "api key without subscription",
			setup: func(w *setupModel) { w.step = stepAPIKey; w.hasSubscription = false },
			wants: []string{"this applies to you"},
		},
		{
			name:  "exa key",
			setup: func(w *setupModel) { w.step = stepExaKey },
			wants: []string{"Exa API Key", "free tier", "dashboard.exa.ai"},
		},
		{
			name: "model recommended",
			setup: func(w *setupModel) {
				w.models = apiModels
				w.cfg.Models.Main.Model = apiModels[0]
				w.step = stepModel
			},
			wants: []string{"(recommended)"},
		},
		{
			name: "done summary",
			setup: func(w *setupModel) {
				w.step = stepDone
				w.hasSubscription = true
				w.cfg.Models.Main.Model = "gpt-5.4"
				w.cfg.Timezone = "America/New_York"
				w.cfg.Location = "New York, NY"
			},
			wants: []string{"all set", "connected", "gpt-5.4", "America/New_York", "New York, NY"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newTestSetup(t)
			tc.setup(&w)

			view := w.View()

			for _, want := range tc.wants {
				if !strings.Contains(view, want) {
					t.Errorf("view missing %q", want)
				}
			}
		})
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
