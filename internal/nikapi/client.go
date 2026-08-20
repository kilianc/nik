// Package nikapi is the typed client for nikd's API.
//
// It is the only way anything that is not nikd reaches NIK_HOME, and it
// deliberately holds no fallback: there is no "open the database if the
// socket is missing" path here, because a fallback is the boundary not
// existing. When nikd is not running, the answer is to say so.
package nikapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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

// New dials the owner socket in the given home.
func New(home string) *Client {
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
