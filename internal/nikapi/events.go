package nikapi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/kciuffolo/nik/internal/api"
)

// Event is one thing that happened, as a client sees it. Data is left raw so
// a caller decodes only the types it cares about — a TUI that ignores
// workload should not pay to parse it.
type Event struct {
	Type string
	Data json.RawMessage
}

// Message decodes a message event.
func (e Event) Message() (Message, error) {
	var out Message
	err := json.Unmarshal(e.Data, &out)

	return out, err
}

// Activity decodes an activity event.
func (e Event) Activity() (api.ActivityEvent, error) {
	var out api.ActivityEvent
	err := json.Unmarshal(e.Data, &out)

	return out, err
}

// Workload decodes a workload event.
func (e Event) Workload() (api.WorkloadEvent, error) {
	var out api.WorkloadEvent
	err := json.Unmarshal(e.Data, &out)

	return out, err
}

// Event type names, re-exported so callers switch on a constant.
const (
	EventMessage  = api.EventMessage
	EventActivity = api.EventActivity
	EventWorkload = api.EventWorkload
	EventResync   = api.EventResync
)

// Events streams until ctx is cancelled or the connection drops, calling fn
// for each event. It does not reconnect: a caller that wants to survive a
// daemon restart owns that loop, because only it knows whether to refetch
// history first.
//
// An EventResync means this client fell behind and what it holds may have
// gaps — refetch rather than trust it.
func (c *Client) Events(ctx context.Context, fn func(Event)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://nikd/v1/events", nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	// The shared client has a 30s timeout, which is exactly wrong for a
	// stream meant to stay open for hours. This one is bounded by ctx alone.
	streamer := &http.Client{Transport: c.http.Transport}

	resp, err := streamer.Do(req)
	if err != nil {
		if isNotListening(err) {
			return ErrNoDaemon
		}

		return fmt.Errorf("/v1/events: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return apiError("/v1/events", resp)
	}

	return scanSSE(resp.Body, fn)
}

// scanSSE reads the frame format: `event:` and `data:` lines, a blank line
// ending each frame, and `:` comment lines that are heartbeats and nothing
// more.
func scanSSE(body interface{ Read([]byte) (int, error) }, fn func(Event)) error {
	scanner := bufio.NewScanner(body)
	// Room for a long message body; the server caps what it sends well below
	// this, so hitting it means something is wrong rather than large.
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)

	var current Event

	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case line == "":
			if current.Type != "" {
				fn(current)
			}
			current = Event{}

		case strings.HasPrefix(line, ":"):
			// heartbeat

		case strings.HasPrefix(line, "event:"):
			current.Type = strings.TrimSpace(strings.TrimPrefix(line, "event:"))

		case strings.HasPrefix(line, "data:"):
			current.Data = json.RawMessage(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}

	err := scanner.Err()
	if err != nil {
		return fmt.Errorf("read event stream: %w", err)
	}

	return nil
}
