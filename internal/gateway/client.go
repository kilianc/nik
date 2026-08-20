package gateway

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/kciuffolo/nik/internal/version"
)

// client speaks the nik-saas gateway protocol. it always dials out — nik sits
// behind NAT and the gateway never connects in — and reconnects with backoff,
// replaying whatever backlog accumulated while it was gone.

type client struct {
	url   string
	token string
	name  string

	priv *[keySize]byte
	pub  *[keySize]byte

	onMessage      func(context.Context, msgIn, msgContent) error
	onConversation func(context.Context, convIn, convContent) error
	onReady        func(ctx context.Context, ack helloAck)
	// tunnel answers api.req envelopes. Nil means the API tunnel is off,
	// which it is unless the owner turned it on — see api.remote in config.
	tunnel  *Tunnel
	onToken func(token string)
	// reloadToken re-reads the token from wherever it is persisted. A 401
	// mid-run may mean the token rotated under us (another process
	// connected — nik connect, a second daemon) and the store already
	// holds the live one; try that before concluding the token is dead.
	reloadToken func() (string, error)
	ready       chan struct{}
	readyOnce   sync.Once

	mu      sync.Mutex
	conn    *websocket.Conn
	selfJID string
}

func newClient(url, token, name string, priv *[keySize]byte) *client {
	return &client{
		url:   url,
		token: token,
		name:  name,
		priv:  priv,
		pub:   publicKeyOf(priv),
		ready: make(chan struct{}),
	}
}

func (c *client) SelfJID() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.selfJID
}

// errAuthRejected marks a definitive refusal: the gateway saw the token and
// said no. Retrying cannot fix a wrong or revoked token, so this ends the
// run instead of entering the backoff loop — the daemon should die loudly.
var errAuthRejected = errors.New("gateway rejected the install token (401): check gateway_token in the secret store")

func (c *client) run(ctx context.Context) error {
	backoff := time.Second

	for {
		err := c.session(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, errAuthRejected) {
			if !c.adoptStoredToken() {
				return err
			}
			slog.Warn("gateway rejected our token; retrying with the one in the store", "pkg", "gateway")
			continue
		}

		slog.Error("gateway session ended", "pkg", "gateway", "error", err, "retry_in", backoff)

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return ctx.Err()
		}

		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

// adoptStoredToken swaps in the persisted token if it differs from the one
// we just dialed with. False means there is nothing new to try.
func (c *client) adoptStoredToken() bool {
	if c.reloadToken == nil {
		return false
	}
	fresh, err := c.reloadToken()
	if err != nil || fresh == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if fresh == c.token {
		return false
	}
	c.token = fresh
	return true
}

func (c *client) session(ctx context.Context) error {
	c.mu.Lock()
	token := c.token
	c.mu.Unlock()
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	conn, resp, err := websocket.Dial(dialCtx, c.url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + token}},
	})
	cancel()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return errAuthRejected
		}
		return fmt.Errorf("dial gateway: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	defer conn.CloseNow()

	err = c.send(ctx, typeHello, hello{
		Version:       protocolVersion,
		AgentName:     c.name,
		PublicKey:     base64.RawURLEncoding.EncodeToString(c.pub[:]),
		ClientVersion: version.Number,
	})
	if err != nil {
		return err
	}

	for {
		var env envelope

		err = wsjson.Read(ctx, conn, &env)
		if err != nil {
			return fmt.Errorf("read envelope: %w", err)
		}

		err = c.dispatch(ctx, env)
		if err != nil {
			slog.Error("gateway dispatch", "pkg", "gateway", "error", err, "type", env.Type)
		}
	}
}

func (c *client) dispatch(ctx context.Context, env envelope) error {
	switch env.Type {
	case typeHelloAck:
		return c.onHelloAck(ctx, env)

	case typeMsgIn:
		return c.onMsgIn(ctx, env)

	case typeConvIn:
		return c.onConvIn(ctx, env)

	case typeAPIReq:
		return c.onAPIReq(ctx, env)

	case typeAck:
		return nil

	case typeError:
		e, err := decodePayload[protocolError](env)
		if err != nil {
			return err
		}

		return fmt.Errorf("gateway rejected envelope %s: %s", e.Ref, e.Reason)

	default:
		return fmt.Errorf("unsupported envelope type %s", env.Type)
	}
}

func (c *client) onHelloAck(ctx context.Context, env envelope) error {
	ack, err := decodePayload[helloAck](env)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.selfJID = ack.SelfJID
	rotated := ack.Token != "" && ack.Token != c.token
	if rotated {
		c.token = ack.Token
	}
	c.readyOnce.Do(func() { close(c.ready) })
	c.mu.Unlock()

	if rotated && c.onToken != nil {
		c.onToken(ack.Token)
	}
	if c.onReady != nil {
		c.onReady(ctx, ack)
	}

	return nil
}

func (c *client) onMsgIn(ctx context.Context, env envelope) error {
	msg, err := decodePayload[msgIn](env)
	if err != nil {
		return err
	}

	sealed, err := base64.RawURLEncoding.DecodeString(msg.Sealed)
	if err != nil {
		return fmt.Errorf("decode sealed content: %w", err)
	}

	opened, err := openSealed(sealed, c.pub, c.priv)
	if err != nil {
		// sealed to a key we no longer hold, so this row can never be read.
		// ack it or the gateway replays it on every reconnect until its ttl
		_ = c.send(ctx, typeAck, ack{IDs: []string{env.ID}})

		return fmt.Errorf("open sealed content: %w", err)
	}

	var content msgContent

	err = json.Unmarshal(opened, &content)
	if err != nil {
		_ = c.send(ctx, typeAck, ack{IDs: []string{env.ID}})

		return fmt.Errorf("decode sealed content: %w", err)
	}

	// ack before handling: the envelope id is the gateway's queue row, and the
	// reply rides its own envelope
	err = c.send(ctx, typeAck, ack{IDs: []string{env.ID}})
	if err != nil {
		return err
	}

	if c.onMessage == nil {
		return nil
	}

	return c.onMessage(ctx, msg, content)
}

// onAPIReq answers a tunnelled request.
//
// A gateway that sends one to a nik with the tunnel off gets a 403 rather
// than silence: "this household has not turned that on" is a fact the platform
// should be able to see, and a dropped envelope looks like a broken agent.
func (c *client) onAPIReq(ctx context.Context, env envelope) error {
	req, err := decodePayload[apiReq](env)
	if err != nil {
		return err
	}

	// Ack first: the envelope id is the gateway's queue row, and the response
	// rides its own envelope.
	err = c.send(ctx, typeAck, ack{IDs: []string{env.ID}})
	if err != nil {
		return err
	}

	if c.tunnel == nil {
		slog.Warn("tunnelled api request with the tunnel off", "pkg", "gateway",
			"method", req.Method, "path", req.Path)

		key, keyErr := decodeSessionKey(req.SessionKey)
		if keyErr != nil {
			return fmt.Errorf("api tunnel is off, and the session key is unreadable: %w", keyErr)
		}

		return (&Tunnel{send: c.send}).respond(ctx, env.ID, key, http.StatusForbidden,
			[]byte(`{"error":"the api tunnel is off on this nik"}`))
	}

	return c.tunnel.handle(ctx, env.ID, req)
}

func (c *client) onConvIn(ctx context.Context, env envelope) error {
	conv, err := decodePayload[convIn](env)
	if err != nil {
		return err
	}

	if c.onConversation == nil {
		return nil
	}

	sealed, err := base64.RawURLEncoding.DecodeString(conv.Sealed)
	if err != nil {
		return fmt.Errorf("decode sealed conversation: %w", err)
	}

	// never queued, so there is nothing to ack and nothing lost: the next
	// message in this conversation brings the metadata round again
	opened, err := openSealed(sealed, c.pub, c.priv)
	if err != nil {
		return fmt.Errorf("open sealed conversation: %w", err)
	}

	var content convContent

	err = json.Unmarshal(opened, &content)
	if err != nil {
		return fmt.Errorf("decode sealed conversation: %w", err)
	}

	return c.onConversation(ctx, conv, content)
}

func (c *client) send(ctx context.Context, t envelopeType, payload any) error {
	env, err := newEnvelope(t, payload)
	if err != nil {
		return err
	}

	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return errors.New("send envelope: not connected")
	}

	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return wsjson.Write(sendCtx, conn, env)
}

// uploadMedia posts attachment bytes and returns the handle to name in a
// media.out. attachments never ride the websocket: one large frame would stall
// every other envelope on the connection, acks included.
func (c *client) uploadMedia(ctx context.Context, data []byte, mimeType, filename string) (string, error) {
	url, err := c.httpURL("/v1/media")
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", mimeType)
	req.Header.Set("X-Nik-Filename", filename)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload media: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload media: %s", resp.Status)
	}

	var out struct {
		Handle string `json:"handle"`
	}

	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		return "", fmt.Errorf("decode upload response: %w", err)
	}

	return out.Handle, nil
}

// fetchAttachment collects an inbound attachment. what crosses the network is
// sealed to this agent, and the gateway drops its copy once collected, so from
// here the only readable copy is local.
func (c *client) fetchAttachment(ctx context.Context, downloadID string) ([]byte, error) {
	if downloadID == "" {
		return nil, errors.New("fetch attachment: empty id")
	}

	url, err := c.httpURL("/v1/media/" + downloadID)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch attachment: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch attachment: %s", resp.Status)
	}

	sealed, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read attachment: %w", err)
	}

	return openSealed(sealed, c.pub, c.priv)
}

// httpURL turns the agent websocket url into a sibling http endpoint
func (c *client) httpURL(path string) (string, error) {
	base, ok := strings.CutSuffix(c.url, "/v1/agent")
	if !ok {
		return "", fmt.Errorf("derive http url from %s", c.url)
	}

	switch {
	case strings.HasPrefix(base, "wss://"):
		base = "https://" + strings.TrimPrefix(base, "wss://")
	case strings.HasPrefix(base, "ws://"):
		base = "http://" + strings.TrimPrefix(base, "ws://")
	}

	return base + path, nil
}

func (c *client) close() {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn != nil {
		_ = conn.CloseNow()
	}
}
