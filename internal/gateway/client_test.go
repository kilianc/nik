package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// fakeGateway is the platform side of the protocol: accepts the websocket,
// answers hello, and lets a test push envelopes down and observe what comes up
type fakeGateway struct {
	// conns is every agent socket ever accepted, so a "gateway restart" can
	// drop all of them at once.
	conns []*websocket.Conn
	// token is the one bearer the gateway currently accepts; it rotates on
	// every hello, like the real one.
	token string
	// rotations counts hellos, so tests can assert rotation happened.
	rotations int
	t         *testing.T
	srv       *httptest.Server

	mu       sync.Mutex
	conn     *websocket.Conn
	received []envelope
	agentPub *[keySize]byte

	selfJID string
	uploads map[string][]byte
	blobs   map[string][]byte
}

func newFakeGateway(t *testing.T) *fakeGateway {
	t.Helper()

	g := &fakeGateway{
		t:       t,
		token:   "test-token",
		selfJID: "16502811468@s.whatsapp.net",
		uploads: map[string][]byte{},
		blobs:   map[string][]byte{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/agent", g.handleAgent)
	mux.HandleFunc("POST /v1/media", g.handleUpload)
	mux.HandleFunc("GET /v1/media/{id}", g.handleDownload)

	g.srv = httptest.NewServer(mux)
	t.Cleanup(g.srv.Close)

	return g
}

// dropAll closes every agent socket server-side, the way a gateway restart
// does; each client's session ends and its run loop reconnects.
func (g *fakeGateway) dropAll() {
	g.mu.Lock()
	conns := append([]*websocket.Conn(nil), g.conns...)
	g.conns = nil
	g.mu.Unlock()
	for _, c := range conns {
		_ = c.CloseNow()
	}
}

func (g *fakeGateway) url() string {
	return "ws" + strings.TrimPrefix(g.srv.URL, "http") + "/v1/agent"
}

func (g *fakeGateway) handleAgent(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	want := g.token
	g.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+want {
		http.Error(w, "bad token", http.StatusUnauthorized)

		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}

	g.mu.Lock()
	g.conn = conn
	g.conns = append(g.conns, conn)
	g.mu.Unlock()

	for {
		var env envelope

		err := wsjson.Read(r.Context(), conn, &env)
		if err != nil {
			return
		}

		if env.Type == typeHello {
			h, err := decodePayload[hello](env)
			if err != nil {
				g.t.Errorf("decode hello: %v", err)

				continue
			}

			raw, err := base64.RawURLEncoding.DecodeString(h.PublicKey)
			if err != nil || len(raw) != keySize {
				g.t.Errorf("bad public key in hello: %v", err)

				continue
			}

			var pub [keySize]byte
			copy(pub[:], raw)

			g.mu.Lock()
			g.agentPub = &pub
			g.rotations++
			g.token = fmt.Sprintf("nik_rotated-%d", g.rotations)
			fresh := g.token
			g.mu.Unlock()

			ackEnv, _ := newEnvelope(typeHelloAck, helloAck{
				Token:   fresh,
				Number:  "+16502811468",
				SelfJID: g.selfJID,
			})
			_ = wsjson.Write(r.Context(), conn, ackEnv)

			continue
		}

		g.mu.Lock()
		g.received = append(g.received, env)
		g.mu.Unlock()
	}
}

func (g *fakeGateway) handleUpload(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	want := g.token
	g.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+want {
		http.Error(w, "bad token", http.StatusUnauthorized)

		return
	}

	data := make([]byte, r.ContentLength)
	_, _ = io.ReadFull(r.Body, data)

	g.mu.Lock()
	g.uploads["mh_test"] = data
	g.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"handle": "mh_test"})
}

func (g *fakeGateway) handleDownload(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	sealed, ok := g.blobs[r.PathValue("id")]
	g.mu.Unlock()

	if !ok {
		http.Error(w, "unknown attachment", http.StatusNotFound)

		return
	}

	_, _ = w.Write(sealed)
}

// push seals content and delivers it as a msg.in, the way the platform does
func (g *fakeGateway) push(t *testing.T, msg msgIn, content msgContent) {
	t.Helper()

	g.mu.Lock()
	pub := g.agentPub
	conn := g.conn
	g.mu.Unlock()

	if pub == nil || conn == nil {
		t.Fatal("push before the agent said hello")
	}

	raw, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}

	sealed, err := sealTo(raw, pub)
	if err != nil {
		t.Fatalf("seal content: %v", err)
	}

	msg.Sealed = base64.RawURLEncoding.EncodeToString(sealed)

	env, err := newEnvelope(typeMsgIn, msg)
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}

	err = wsjson.Write(context.Background(), conn, env)
	if err != nil {
		t.Fatalf("write msg.in: %v", err)
	}
}

func (g *fakeGateway) sent() []envelope {
	g.mu.Lock()
	defer g.mu.Unlock()

	return append([]envelope(nil), g.received...)
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timeout waiting for %s", what)
}

func testKey(t *testing.T) *[keySize]byte {
	t.Helper()

	_, priv, err := generateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	return priv
}

func TestClientHandshakeAndSelfJID(t *testing.T) {
	gw := newFakeGateway(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := newClient(gw.url(), "test-token", "test", testKey(t))

	go func() { _ = c.run(ctx) }()

	waitUntil(t, "self jid from hello.ack", func() bool {
		return c.SelfJID() == "16502811468@s.whatsapp.net"
	})
}

func TestClientDecryptsAndAcks(t *testing.T) {
	gw := newFakeGateway(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := newClient(gw.url(), "test-token", "test", testKey(t))

	var mu sync.Mutex
	var got []msgContent

	c.onMessage = func(_ context.Context, _ msgIn, content msgContent) error {
		mu.Lock()
		got = append(got, content)
		mu.Unlock()

		return nil
	}

	go func() { _ = c.run(ctx) }()
	waitUntil(t, "handshake", func() bool { return c.SelfJID() != "" })

	gw.push(t, msgIn{
		Chat:   "14155551234@s.whatsapp.net",
		Sender: "14155551234@s.whatsapp.net",
		WaID:   "wa-1",
	}, msgContent{Kind: kindText, Body: "hey nik"})

	waitUntil(t, "message delivered", func() bool {
		mu.Lock()
		defer mu.Unlock()

		return len(got) == 1
	})

	mu.Lock()
	if got[0].Body != "hey nik" || got[0].Kind != kindText {
		t.Errorf("content = %+v", got[0])
	}
	mu.Unlock()

	// the ack names the envelope id, which is the gateway's queue row
	waitUntil(t, "ack", func() bool {
		for _, env := range gw.sent() {
			if env.Type == typeAck {
				return true
			}
		}

		return false
	})
}

func TestClientAcksUndecryptableRows(t *testing.T) {
	gw := newFakeGateway(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := newClient(gw.url(), "test-token", "test", testKey(t))

	delivered := false
	c.onMessage = func(_ context.Context, _ msgIn, _ msgContent) error {
		delivered = true

		return nil
	}

	go func() { _ = c.run(ctx) }()
	waitUntil(t, "handshake", func() bool { return c.SelfJID() != "" })

	// seal to a key the client does not hold: a reinstalled agent's backlog
	strangerPub, _, err := generateKey()
	if err != nil {
		t.Fatalf("generate stranger key: %v", err)
	}

	raw, _ := json.Marshal(msgContent{Kind: kindText, Body: "locked away"})
	sealed, err := sealTo(raw, strangerPub)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	env, _ := newEnvelope(typeMsgIn, msgIn{
		Chat:   "1@s.whatsapp.net",
		Sender: "1@s.whatsapp.net",
		WaID:   "wa-locked",
		Sealed: base64.RawURLEncoding.EncodeToString(sealed),
	})

	gw.mu.Lock()
	conn := gw.conn
	gw.mu.Unlock()

	err = wsjson.Write(ctx, conn, env)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	// the row must be acked — otherwise it replays on every reconnect until
	// the ttl — and must not reach the handler
	waitUntil(t, "ack for undecryptable row", func() bool {
		for _, e := range gw.sent() {
			if e.Type != typeAck {
				continue
			}

			a, err := decodePayload[ack](e)
			if err == nil && len(a.IDs) == 1 && a.IDs[0] == env.ID {
				return true
			}
		}

		return false
	})

	if delivered {
		t.Error("undecryptable content reached the handler")
	}
}

func TestClientFetchAttachment(t *testing.T) {
	gw := newFakeGateway(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	priv := testKey(t)
	c := newClient(gw.url(), "test-token", "test", priv)

	go func() { _ = c.run(ctx) }()
	waitUntil(t, "handshake", func() bool { return c.SelfJID() != "" })

	want := []byte("jpeg-bytes")

	sealed, err := sealTo(want, publicKeyOf(priv))
	if err != nil {
		t.Fatalf("seal blob: %v", err)
	}

	gw.mu.Lock()
	gw.blobs["blob-1"] = sealed
	gw.mu.Unlock()

	got, err := c.fetchAttachment(ctx, "blob-1")
	if err != nil {
		t.Fatalf("fetch attachment: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("attachment = %q, want %q", got, want)
	}

	_, err = c.fetchAttachment(ctx, "no-such-blob")
	if err == nil {
		t.Error("fetching an unknown blob succeeded")
	}
}

func TestClientHTTPURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"wss", "wss://nik-gw.example.com/v1/agent", "https://nik-gw.example.com/v1/media"},
		{"ws", "ws://127.0.0.1:8080/v1/agent", "http://127.0.0.1:8080/v1/media"},
		// The platform is renaming this path and serves both, so a config
		// written either side of the rename has to work here. Media is the
		// only thing that reads the path, and it fails long after connect.
		{"wss, renamed path", "wss://nik-gw.example.com/v1/nik", "https://nik-gw.example.com/v1/media"},
		{"ws, renamed path", "ws://127.0.0.1:8080/v1/nik", "http://127.0.0.1:8080/v1/media"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newClient(tt.in, "t", "n", testKey(t))

			got, err := c.httpURL("/v1/media")
			if err != nil {
				t.Fatalf("httpURL: %v", err)
			}
			if got != tt.want {
				t.Errorf("httpURL = %q, want %q", got, tt.want)
			}
		})
	}

	c := newClient("wss://bad.example.com/other", "t", "n", testKey(t))

	_, err := c.httpURL("/v1/media")
	if err == nil {
		t.Error("derived an http url from a websocket url with neither path")
	}
}

// A 401 at the handshake is a rejected token, not a bad network moment:
// run must return the terminal error instead of retrying forever.
func TestRunStopsOnRejectedToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad token", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := newClient("ws"+strings.TrimPrefix(srv.URL, "http"), "nik_bad", "test", testKey(t))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := c.run(ctx)
	if !errors.Is(err, errAuthRejected) {
		t.Fatalf("run returned %v, want errAuthRejected", err)
	}
	if ctx.Err() != nil {
		t.Fatal("run only ended because the context expired — it retried a rejected token")
	}
}
