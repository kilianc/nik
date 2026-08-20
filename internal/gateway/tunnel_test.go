package gateway

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"
)

// A tunnel test is a round trip: seal a request the way the gateway would,
// run it, and open the response the way nik-web would. Anything less proves
// the envelope decodes, which the fixtures already say.

type sentEnvelope struct {
	typ     envelopeType
	payload any
}

func newTestTunnel(t *testing.T, handler http.Handler) (*Tunnel, *[]sentEnvelope, *[keySize]byte, *[keySize]byte) {
	t.Helper()

	agentPub, agentPriv, err := generateKey()
	if err != nil {
		t.Fatalf("generate agent key: %v", err)
	}

	var sent []sentEnvelope
	send := func(_ context.Context, typ envelopeType, payload any) error {
		sent = append(sent, sentEnvelope{typ: typ, payload: payload})

		return nil
	}

	return newTunnel(handler, agentPriv, agentPub, send), &sent, agentPub, agentPriv
}

// sessionKeys is what the far side would hold: a keypair whose public half
// rides in the request so a reply has somewhere to be sealed to.
func sessionKeys(t *testing.T) (hexPub string, pub, priv *[keySize]byte) {
	t.Helper()

	pub, priv, err := generateKey()
	if err != nil {
		t.Fatalf("generate session key: %v", err)
	}

	return hex.EncodeToString(pub[:]), pub, priv
}

func lastResponse(t *testing.T, sent []sentEnvelope) apiRes {
	t.Helper()

	if len(sent) == 0 {
		t.Fatal("nothing was sent back")
	}

	last := sent[len(sent)-1]
	if last.typ != typeAPIRes {
		t.Fatalf("sent a %q, want %q", last.typ, typeAPIRes)
	}

	res, ok := last.payload.(apiRes)
	if !ok {
		t.Fatalf("payload is %T, want apiRes", last.payload)
	}

	return res
}

func openResponse(t *testing.T, res apiRes, pub, priv *[keySize]byte) []byte {
	t.Helper()

	sealed, err := base64.RawURLEncoding.DecodeString(res.Sealed)
	if err != nil {
		t.Fatalf("decode sealed response: %v", err)
	}

	opened, err := openSealed(sealed, pub, priv)
	if err != nil {
		t.Fatalf("open sealed response: %v", err)
	}

	return opened
}

func TestTunnelRunsAGetAndSealsTheResponse(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/health" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("verbose") != "true" {
			t.Errorf("query was not carried: %q", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ready":true}`))
	})

	tunnel, sent, _, _ := newTestTunnel(t, handler)
	hexPub, pub, priv := sessionKeys(t)

	err := tunnel.handle(context.Background(), "env-1", apiReq{
		Method:     http.MethodGet,
		Path:       "/v1/health",
		Query:      "verbose=true",
		SessionKey: hexPub,
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	res := lastResponse(t, *sent)
	if res.Ref != "env-1" {
		t.Errorf("ref = %q, want env-1", res.Ref)
	}
	if res.Status != http.StatusOK {
		t.Errorf("status = %d", res.Status)
	}

	body := openResponse(t, res, pub, priv)
	if string(body) != `{"ready":true}` {
		t.Errorf("body = %q", body)
	}
}

func TestTunnelCarriesASealedRequestBody(t *testing.T) {
	var got map[string]string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusAccepted)
	})

	tunnel, sent, agentPub, _ := newTestTunnel(t, handler)
	hexPub, pub, priv := sessionKeys(t)

	// The gateway seals to the agent's public key — the same anonymous sealed
	// box every other inbound verb uses.
	sealed, err := sealTo([]byte(`{"body":"dinner's ready"}`), agentPub)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	err = tunnel.handle(context.Background(), "env-2", apiReq{
		Method:     http.MethodPost,
		Path:       "/v1/conversations/local/messages",
		SessionKey: hexPub,
		Sealed:     base64.RawURLEncoding.EncodeToString(sealed),
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if got["body"] != "dinner's ready" {
		t.Fatalf("handler received %v", got)
	}

	res := lastResponse(t, *sent)
	if res.Status != http.StatusAccepted {
		t.Errorf("status = %d, want 202", res.Status)
	}
	openResponse(t, res, pub, priv)
}

// The gateway cannot open what nik seals, and nik-web's session key is not
// nik's. A response sealed to the wrong recipient is one nobody can read, so
// this is worth asserting rather than assuming.
func TestTunnelResponseIsNotReadableByTheAgentKey(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"secret":"for the session only"}`))
	})

	tunnel, sent, agentPub, agentPriv := newTestTunnel(t, handler)
	hexPub, _, _ := sessionKeys(t)

	err := tunnel.handle(context.Background(), "env-3", apiReq{
		Method: http.MethodGet, Path: "/v1/health", SessionKey: hexPub,
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	res := lastResponse(t, *sent)

	sealed, err := base64.RawURLEncoding.DecodeString(res.Sealed)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	_, err = openSealed(sealed, agentPub, agentPriv)
	if err == nil {
		t.Fatal("the agent's own key opened a response sealed to the session")
	}
}

// A body nik cannot open is the caller's problem, and it gets an answer
// saying so. A request that dies quietly is a console that hangs.
func TestTunnelAnswers400ForAnUnopenableBody(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("the handler should not have run")
	})

	tunnel, sent, _, _ := newTestTunnel(t, handler)
	hexPub, pub, priv := sessionKeys(t)

	err := tunnel.handle(context.Background(), "env-4", apiReq{
		Method:     http.MethodPost,
		Path:       "/v1/conversations/local/messages",
		SessionKey: hexPub,
		Sealed:     base64.RawURLEncoding.EncodeToString([]byte("not a sealed box")),
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	res := lastResponse(t, *sent)
	if res.Status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.Status)
	}
	openResponse(t, res, pub, priv)
}

// Without a usable session key there is nowhere to seal a reply, so there is
// no reply — the one failure the far side learns about by timing out.
func TestTunnelRefusesAnUnusableSessionKey(t *testing.T) {
	tunnel, sent, _, _ := newTestTunnel(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the handler should not have run")
	}))

	err := tunnel.handle(context.Background(), "env-5", apiReq{
		Method: http.MethodGet, Path: "/v1/health", SessionKey: "not-hex",
	})
	if err == nil {
		t.Fatal("want an error for an unusable session key")
	}
	if len(*sent) != 0 {
		t.Fatalf("sent %d envelopes, want none", len(*sent))
	}
}

// A handler's own error statuses ride back untouched. The tunnel is a pipe,
// not a policy layer — the scope check already happened inside the handler.
func TestTunnelCarriesHandlerErrorStatuses(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"not available on this socket"}`))
	})

	tunnel, sent, _, _ := newTestTunnel(t, handler)
	hexPub, pub, priv := sessionKeys(t)

	err := tunnel.handle(context.Background(), "env-6", apiReq{
		Method: http.MethodGet, Path: "/v1/secrets", SessionKey: hexPub,
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	res := lastResponse(t, *sent)
	if res.Status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", res.Status)
	}
	if string(openResponse(t, res, pub, priv)) != `{"error":"not available on this socket"}` {
		t.Error("the handler's body did not survive the tunnel")
	}
}
