package gateway

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
)

// The API tunnel: nikd's own HTTP handler, reached over the socket nik already
// dials out on.
//
// This is remote access to somebody's computer, and it is off unless they say
// otherwise — see api.remote in config. For a managed nik the provisioner
// writes that config, which is a family agreeing to be hosted rather than a
// self-hoster being surprised.

// maxTunnelBody bounds a request from the far side. Generous for a chat
// message, small enough that nothing out there can make a cell allocate its
// way past its memory budget.
const maxTunnelBody = 1 << 20

// Tunnel answers api.req envelopes by running them through an http.Handler.
type Tunnel struct {
	handler http.Handler
	// send delivers a response envelope back to the gateway.
	send func(ctx context.Context, t envelopeType, payload any) error
	priv *[keySize]byte
	pub  *[keySize]byte
}

func newTunnel(handler http.Handler, priv, pub *[keySize]byte,
	send func(ctx context.Context, t envelopeType, payload any) error) *Tunnel {
	return &Tunnel{handler: handler, send: send, priv: priv, pub: pub}
}

// handle runs one tunnelled request and sends its response.
//
// Everything that can go wrong here answers with a status rather than an
// error: the far side is waiting for a reply, and a request that dies quietly
// is a console that hangs.
func (t *Tunnel) handle(ctx context.Context, envID string, req apiReq) error {
	sessionKey, err := decodeSessionKey(req.SessionKey)
	if err != nil {
		// Nowhere to seal a reply to, so there is no reply to send. This is
		// the one failure the far side learns about by timing out, and it is
		// a bug on their side rather than a state nik can report.
		return fmt.Errorf("decode session key: %w", err)
	}

	body, err := t.openBody(req)
	if err != nil {
		return t.respond(ctx, envID, sessionKey, http.StatusBadRequest,
			[]byte(`{"error":"could not open sealed request body"}`))
	}

	// The path and method arrive in the clear precisely so this is loggable.
	// We reached into somebody's computer; they should not have to take our
	// word for when.
	slog.Info("tunnelled api request", "pkg", "gateway",
		"method", req.Method, "path", req.Path)

	target := req.Path
	if req.Query != "" {
		target += "?" + req.Query
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, target, bytes.NewReader(body))
	if err != nil {
		return t.respond(ctx, envID, sessionKey, http.StatusBadRequest,
			[]byte(`{"error":"malformed request"}`))
	}
	if len(body) > 0 {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	rec := httptest.NewRecorder()
	t.handler.ServeHTTP(rec, httpReq)

	out, err := io.ReadAll(io.LimitReader(rec.Body, maxTunnelBody))
	if err != nil {
		return t.respond(ctx, envID, sessionKey, http.StatusInternalServerError,
			[]byte(`{"error":"could not read response"}`))
	}

	return t.respond(ctx, envID, sessionKey, rec.Code, out)
}

func (t *Tunnel) openBody(req apiReq) ([]byte, error) {
	if req.Sealed == "" {
		return nil, nil
	}

	sealed, err := base64.RawURLEncoding.DecodeString(req.Sealed)
	if err != nil {
		return nil, fmt.Errorf("decode sealed body: %w", err)
	}

	opened, err := openSealed(sealed, t.pub, t.priv)
	if err != nil {
		return nil, err
	}
	if len(opened) > maxTunnelBody {
		return nil, fmt.Errorf("request body is %d bytes, over the %d limit", len(opened), maxTunnelBody)
	}

	return opened, nil
}

func (t *Tunnel) respond(ctx context.Context, envID string, sessionKey *[keySize]byte, status int, body []byte) error {
	sealed, err := sealTo(body, sessionKey)
	if err != nil {
		return fmt.Errorf("seal response: %w", err)
	}

	return t.send(ctx, typeAPIRes, apiRes{
		Ref:    envID,
		Status: status,
		Sealed: base64.RawURLEncoding.EncodeToString(sealed),
	})
}

// decodeSessionKey reads the hex X25519 public key a response is sealed to.
//
// Anonymous sealed boxes only run one way: the gateway seals to this nik,
// and nothing on the platform can open what this nik seals. A reply
// therefore needs a recipient of its own, which the request carries.
func decodeSessionKey(raw string) (*[keySize]byte, error) {
	decoded, err := hex.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if len(decoded) != keySize {
		return nil, fmt.Errorf("session key is %d bytes, want %d", len(decoded), keySize)
	}

	var key [keySize]byte
	copy(key[:], decoded)

	return &key, nil
}
