// Package nikapi is the typed client for nikd's API.
//
// It is the only way anything that is not nikd reaches NIK_HOME, and it
// deliberately holds no fallback: there is no "open the database if the
// socket is missing" path here, because a fallback is the boundary not
// existing. When nikd is not running, the answer is to say so.
package nikapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kciuffolo/nik/internal/api"
)

// ErrNoDaemon is what every call returns when nothing is listening. Callers
// check for it to print something a person can act on rather than a connection
// error naming a socket they have never heard of.
var ErrNoDaemon = errors.New("no daemon listening")

type Client struct {
	socket string
	http   *http.Client
}

// New dials nikd for the given home.
//
// NIK_SOCKET overrides the path, which is how nikctl works inside the shell
// container: NIK_HOME points at the mounted workspace, but the socket it
// should use is the narrowed one mounted separately — the owner socket is not
// reachable from in there at all.
func New(home string) *Client {
	if socket := os.Getenv("NIK_SOCKET"); socket != "" {
		return NewAtSocket(socket)
	}

	return NewAtSocket(api.OwnerSocketPath(home))
}

func NewAtSocket(socket string) *Client {
	return &Client{
		socket: socket,
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socket)
				},
			},
			// Long enough for nikd to be busy, short enough that a wedged
			// daemon does not look like a hung terminal. Streaming endpoints
			// set their own.
			Timeout: 30 * time.Second,
		},
	}
}

// Socket is where this client is pointed, for error messages that need to
// name it.
func (c *Client) Socket() string { return c.socket }

func (c *Client) Version(ctx context.Context) (Version, error) {
	var out Version
	err := c.get(ctx, "/v1/version", &out)

	return out, err
}

func (c *Client) Health(ctx context.Context) (Health, error) {
	var out Health
	err := c.get(ctx, "/v1/health", &out)

	return out, err
}

// Conversation reads one thread's metadata, including what nik is visibly
// doing in it right now.
func (c *Client) Conversation(ctx context.Context, convID string) (Conversation, error) {
	var out Conversation
	err := c.get(ctx, "/v1/conversations/"+url.PathEscape(convID), &out)

	return out, err
}

// Messages pages a conversation, oldest first. after is the newest id the
// caller already has; empty asks for the most recent page.
func (c *Client) Messages(ctx context.Context, convID, after string, limit int) ([]Message, error) {
	q := url.Values{}
	if after != "" {
		q.Set("after", after)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}

	path := "/v1/conversations/" + url.PathEscape(convID) + "/messages"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	var out struct {
		Messages []Message `json:"messages"`
	}
	err := c.get(ctx, path, &out)

	return out.Messages, err
}

// Send posts a message as the owner. It returns once nikd has recorded it,
// which is before nik has read it — what happens next is watched on the
// conversation.
func (c *Client) Send(ctx context.Context, convID, body string) error {
	return c.post(ctx,
		"/v1/conversations/"+url.PathEscape(convID)+"/messages",
		api.SendRequest{Body: body})
}

// Config reads nikd's live configuration.
func (c *Client) Config(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.get(ctx, "/v1/config", &out)

	return out, err
}

// SetConfig changes fields and returns the configuration as nikd now holds
// it — including whatever normalization did to what was sent.
func (c *Client) SetConfig(ctx context.Context, fields ...api.ConfigField) (map[string]any, error) {
	var out map[string]any
	err := c.request(ctx, http.MethodPatch, "/v1/config", api.ConfigPatch{Set: fields}, &out)

	return out, err
}

// ErrAuthRejected is a token the gateway refused — expired or revoked. It is
// separated from every other connect failure because it is the one with a
// specific remedy rather than "try again".
var ErrAuthRejected = errors.New("gateway rejected the token")

// Connect links this nik to an account. It returns once the gateway has
// accepted the token, which means a nil error is proof the pair works rather
// than proof it was written down.
func (c *Client) Connect(ctx context.Context, url, token string) error {
	err := c.post(ctx, "/v1/gateway/connect", api.ConnectRequest{URL: url, Token: token})
	if err != nil && strings.Contains(err.Error(), "rejected that token") {
		return ErrAuthRejected
	}

	return err
}

// Onboarding reports how far along genesis is: when this nik was born,
// whether it finished becoming itself, and which scene is playing.
func (c *Client) Onboarding(ctx context.Context) (api.OnboardingState, error) {
	var out api.OnboardingState
	err := c.get(ctx, "/v1/onboarding", &out)

	return out, err
}

// Workload is what nik is carrying right now: pending alarms and active
// tasks. The event stream carries changes after this.
func (c *Client) Workload(ctx context.Context) (api.WorkloadEvent, error) {
	var out api.WorkloadEvent
	err := c.get(ctx, "/v1/workload", &out)

	return out, err
}

// Query runs a read-only statement against nik's database. Writes are
// refused, and sensitive columns come back redacted — the same rules nik's own
// db_query tool follows, because it is the same code.
func (c *Client) Query(ctx context.Context, query string) (map[string]any, error) {
	var out map[string]any
	err := c.request(ctx, http.MethodPost, "/v1/db/query", api.QueryRequest{Query: query}, &out)

	return out, err
}

// Shell runs a command in nik's sandbox. A command that outlives the wait
// keeps running; the result says so rather than killing it.
func (c *Client) Shell(ctx context.Context, command string, timeoutSeconds int) (api.ShellResult, error) {
	var out api.ShellResult
	err := c.request(ctx, http.MethodPost, "/v1/shell",
		api.ShellRequest{Command: command, TimeoutSeconds: timeoutSeconds}, &out)

	return out, err
}

// Logs tails nikd's own log. errorsOnly reads the warnings-and-above file
// instead, which is where a nik that is up and not answering says why.
func (c *Client) Logs(ctx context.Context, errorsOnly bool, lines int) ([]string, error) {
	q := url.Values{}
	if errorsOnly {
		q.Set("errors", "true")
	}
	if lines > 0 {
		q.Set("lines", strconv.Itoa(lines))
	}

	path := "/v1/logs"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	var out struct {
		Lines []string `json:"lines"`
	}
	err := c.get(ctx, path, &out)

	return out.Lines, err
}

// Restart asks nikd to stop and come back. It returns once the request was
// accepted, which is not the same as it having returned — only a fresh
// connection proves that.
func (c *Client) Restart(ctx context.Context) error {
	return c.post(ctx, "/v1/daemon/restart", struct{}{})
}

// Secrets lists the names in the store. Values are never included: a list is
// for knowing what is there, not for reading it.
func (c *Client) Secrets(ctx context.Context) ([]string, error) {
	var out struct {
		Names []string `json:"names"`
	}
	err := c.get(ctx, "/v1/secrets", &out)

	return out.Names, err
}

// Secret reads one value. A name this caller may not have and a name that
// does not exist answer identically, on purpose.
func (c *Client) Secret(ctx context.Context, name string) (string, error) {
	var out struct {
		Value string `json:"value"`
	}
	err := c.get(ctx, "/v1/secrets/"+url.PathEscape(name), &out)

	return out.Value, err
}

func (c *Client) SetSecret(ctx context.Context, name, value string) error {
	return c.request(ctx, http.MethodPut,
		"/v1/secrets/"+url.PathEscape(name), api.SecretRequest{Value: value}, nil)
}

func (c *Client) DeleteSecret(ctx context.Context, name string) error {
	return c.request(ctx, http.MethodDelete,
		"/v1/secrets/"+url.PathEscape(name), nil, nil)
}

func (c *Client) post(ctx context.Context, path string, body any) error {
	return c.request(ctx, http.MethodPost, path, body, nil)
}

func (c *Client) request(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, "http://nikd"+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if isNotListening(err) {
			return ErrNoDaemon
		}

		return fmt.Errorf("%s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return apiError(path, resp)
	}

	if out == nil {
		// Drain so the connection can be reused rather than closed under us.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))

		return nil
	}

	err = json.NewDecoder(resp.Body).Decode(out)
	if err != nil {
		return fmt.Errorf("%s: decode response: %w", path, err)
	}

	return nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	// The host is ignored — the transport dials a socket — but net/http still
	// needs a syntactically valid URL, and "nikd" beats "localhost" in any
	// error that escapes.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://nikd"+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if isNotListening(err) {
			return ErrNoDaemon
		}

		return fmt.Errorf("%s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return apiError(path, resp)
	}

	err = json.NewDecoder(resp.Body).Decode(out)
	if err != nil {
		return fmt.Errorf("%s: decode response: %w", path, err)
	}

	return nil
}

func apiError(path string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))

	var e api.Error
	if json.Unmarshal(body, &e) == nil && e.Error != "" {
		return fmt.Errorf("%s: %s", path, e.Error)
	}

	return fmt.Errorf("%s: %s", path, resp.Status)
}

// isNotListening distinguishes "nikd is not running" from every other
// transport failure. There is no typed error for a missing socket — the
// syscall surfaces as ENOENT or ECONNREFUSED depending on whether the file
// is gone or merely unbound — so this matches on both.
func isNotListening(err error) bool {
	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		return false
	}

	msg := opErr.Err.Error()

	return strings.Contains(msg, "no such file or directory") ||
		strings.Contains(msg, "connection refused")
}
