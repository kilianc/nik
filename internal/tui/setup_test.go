package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kciuffolo/nik/internal/api"
	"github.com/kciuffolo/nik/internal/codex"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/nikapi"
)

// The gateway is the hard gate: welcome leads to it before anything else.
// With no stored token, the step appears and asks; with one, it is probed
// silently (covered by the transition table: validating → auth choice).
func TestSetupWelcomeToGateway(t *testing.T) {
	w := newTestSetup(t)

	w, _ = w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if w.step != stepGatewayToken {
		t.Errorf("expected step stepGatewayToken, got %d", w.step)
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
		{"gateway validation success advances", stepGatewayValidating, gatewayValidatedMsg{}, stepAuthChoice, false},
		{"gateway validation fail reverts", stepGatewayValidating, gatewayValidatedMsg{err: errTest}, stepGatewayToken, true},
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

// The wizard hands what it collected to nikd. Timezone reaching the contact
// cards is nikd's business now — see apisvc.Config.propagate and its test.
//
// What stays this package's problem: the typed install token must never be
// written back as a secret. The gateway step already stored the rotated one,
// and writing the typed value over it would resurrect a dead install code.
func TestSetupWriteSendsKeysButNeverTheTypedToken(t *testing.T) {
	cfg := config.Default(t.TempDir())
	cfg.Timezone = "Europe/Rome"
	cfg.Location = "Rome, Italy"

	secrets := &recordingSecrets{values: map[string]string{}}
	client := serveFakeAPI(t, secrets)

	msg := writeSetupCmd(client, cfg, "sk-openai", "exa-key", "nik_test-token")()

	written, ok := msg.(configWrittenMsg)
	if !ok {
		t.Fatalf("expected configWrittenMsg, got %T", msg)
	}
	if written.err != nil {
		t.Fatalf("write failed: %v", written.err)
	}

	if secrets.values["openai_key"] != "sk-openai" {
		t.Errorf("openai_key = %q", secrets.values["openai_key"])
	}
	if secrets.values["exa_api_key"] != "exa-key" {
		t.Errorf("exa_api_key = %q", secrets.values["exa_api_key"])
	}
	if _, wrote := secrets.values["gateway_token"]; wrote {
		t.Fatal("the wizard wrote the typed install token over the rotated one")
	}
}

func newTestSetup(t *testing.T) setupModel {
	t.Helper()

	// A nil client: these tests drive the wizard's state machine, and every
	// command it can issue guards on a nil client rather than reaching for a
	// daemon that is not there.
	return newSetupModel(config.Default(t.TempDir()), nil)
}

var errTest = &testError{"test error"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

// recordingSecrets is a secret store that remembers what it was told, so a
// test can assert on what the wizard sent rather than on what a real store
// did with it.
type recordingSecrets struct {
	values map[string]string
}

func (r *recordingSecrets) List(context.Context) ([]string, error) {
	names := make([]string, 0, len(r.values))
	for name := range r.values {
		names = append(names, name)
	}

	return names, nil
}

func (r *recordingSecrets) Get(_ context.Context, _ api.Scope, name string) (string, error) {
	value, ok := r.values[name]
	if !ok {
		return "", api.ErrNotFound
	}

	return value, nil
}

func (r *recordingSecrets) Set(_ context.Context, _ api.Scope, name, value string) error {
	r.values[name] = value

	return nil
}

func (r *recordingSecrets) Delete(_ context.Context, name string) error {
	delete(r.values, name)

	return nil
}

type stubConfig struct{ fields map[string]string }

func (s *stubConfig) Get(context.Context) (map[string]any, error) {
	return map[string]any{}, nil
}

func (s *stubConfig) Set(_ context.Context, field, value string) error {
	s.fields[field] = value

	return nil
}

// serveFakeAPI runs a real nikd API over a real socket, backed by stubs. The
// wizard's commands go over the wire the way they will in life; what is faked
// is only what sits behind the handlers.
func serveFakeAPI(t *testing.T, secrets api.Secrets) *nikapi.Client {
	t.Helper()

	dir, err := os.MkdirTemp("", "nik")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	path := filepath.Join(dir, "nikd.sock")
	ln, err := api.Listen(path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := api.New(api.NewState())
	srv.SetSecrets(secrets)
	srv.SetConfig(&stubConfig{fields: map[string]string{}})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Serve(ctx, ln)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	return nikapi.NewAtSocket(path)
}
