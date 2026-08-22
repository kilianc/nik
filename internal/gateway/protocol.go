package gateway

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
)

// wire format for the gateway (its own PROTOCOL doc). these types are
// a hand-maintained copy of the platform's: the gateway keeps its packages
// internal and builds without cgo, so importing across the two was never an
// option.
//
// the copies are held together by the fixtures in testdata/, which are
// byte-identical to the platform's and decoded by a test on each side. change a
// struct tag here and the twin test over there fails, which is the point.

// protocolVersion is what this nik sends in hello.
//
// Still 2, deliberately, even though everything v3 needs is implemented
// below. Today's gateway accepts exactly 2 and rejects anything else, so a nik
// that announced 3 would be refused at hello — every household offline until
// the platform caught up. The ordering that avoids that:
//
//  1. nik learns to speak v3 and ships it (this change). A v2 gateway never
//     sends an api.req, so the code is simply unreached.
//  2. the gateway accepts a range, MinVersion 2 through Version 3, and
//     records what it negotiated per connection.
//  3. this constant becomes 3, and the tunnel is live.
//
// Step 3 is one line and belongs in the release after step 2, not before it.
const protocolVersion = 2

// maxProtocolVersion is what this nik can speak, as opposed to what it
// announces. It is what step 3 above promotes.
const maxProtocolVersion = 3

type envelopeType string

const (
	typeHello     envelopeType = "hello"
	typeHelloAck  envelopeType = "hello.ack"
	typeMsgIn     envelopeType = "msg.in"
	typeConvIn    envelopeType = "conv.in"
	typeMsgOut    envelopeType = "msg.out"
	typeMediaOut  envelopeType = "media.out"
	typeReactOut  envelopeType = "react.out"
	typeTypingOut envelopeType = "typing.out"
	typeReadOut   envelopeType = "read.out"
	typeAck       envelopeType = "ack"
	typeError     envelopeType = "error"

	// v3. The API tunnel: nikd's own HTTP API, carried over the socket it
	// already holds.
	typeAPIReq envelopeType = "api.req"
	typeAPIRes envelopeType = "api.res"
	typeAPIEvt envelopeType = "api.evt"
)

// The tunnel is three envelope types rather than one per console feature, and
// that is the whole design decision.
//
// New envelope types have to be gated on a negotiated version, so enumerating
// console verbs — console.in, console.out, history, activity — makes every
// later console feature a protocol bump: two repos, two fixture sets, and a
// compatibility window against households nobody can redeploy. Carrying
// {method, path, body} instead means this is the last console-shaped bump
// there ever needs to be, and nik-web becomes an HTTP client whose transport
// happens to be a websocket.
//
// What the gateway keeps: method and path ride in the clear, so it can apply
// route policy, rate-limit, meter and write an audit line that says what was
// asked. What it does not get: the body, which is sealed. That is the same
// bargain every other verb already makes.

// apiReq is one request, tunnelled. G→A.
type apiReq struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	// Query is the raw query string, without the leading "?".
	Query string `json:"query,omitempty"`
	// SessionKey is the hex X25519 public key the response is sealed to.
	// Anonymous sealed boxes only run one way — the gateway seals to this
	// agent — so a reply needs somewhere of its own to be sealed to.
	SessionKey string `json:"session_key"`
	// Sealed is the request body, sealed to this agent's key. Empty for
	// requests without one.
	Sealed string `json:"sealed,omitempty"`
}

// apiRes is one response, tunnelled. A→G. Ref names the request.
type apiRes struct {
	Ref    string `json:"ref"`
	Status int    `json:"status"`
	// Sealed is the response body, sealed to the request's session key.
	Sealed string `json:"sealed,omitempty"`
}

// apiEvt is one server-sent event, tunnelled. A→G.
//
// Separate from apiRes because an event stream is not a reply: it has no
// request to reference beyond the one that opened it, and it arrives whenever
// nik has something to say.
type apiEvt struct {
	// Ref names the api.req that opened the stream.
	Ref string `json:"ref"`
	// Event is the SSE event name, in the clear — the same bargain as path.
	Event string `json:"event"`
	// Sealed is the event payload, sealed to the stream's session key.
	Sealed string `json:"sealed"`
}

// message kinds, matching messaging.InboundMessage.Kind
const (
	kindText     = "text"
	kindImage    = "image"
	kindAudio    = "audio"
	kindVideo    = "video"
	kindDocument = "document"
	kindSticker  = "sticker"
	kindReaction = "reaction"
)

const (
	convDM    = "dm"
	convGroup = "group"
)

const (
	mediaDocument = "document"
	mediaVoice    = "voice"
)

type envelope struct {
	ID      string          `json:"id"`
	Type    envelopeType    `json:"type"`
	TS      time.Time       `json:"ts"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type hello struct {
	Version   int    `json:"version"`
	AgentName string `json:"agent_name,omitempty"`
	PublicKey string `json:"public_key"`
	// ClientVersion is nik's own release, stamped at build time. optional:
	// every nik released before this field existed omits it, and the
	// platform cannot ask them to say it. a build that never went through a
	// release says "dev".
	ClientVersion string `json:"client_version,omitempty"`
}

// helloAck carries the central number's identity. There is no claim state:
// the account was born from a DM, so its JID is bound before any agent runs.
type helloAck struct {
	// Token is a fresh long-lived agent token, rotated on every connect. nik
	// stores it and dials with it next time; the one it connected with — on
	// first run, the short-lived install code from the dashboard — is dead.
	Token  string `json:"token,omitempty"`
	Number string `json:"number,omitempty"`
	// SelfJID is the central number's JID — the only source nik has for its
	// own whatsapp identity in gateway mode
	SelfJID string `json:"self_jid,omitempty"`
}

// msgIn carries routing metadata in the clear; Sealed holds a msgContent only
// this agent's key can open
type msgIn struct {
	Chat    string    `json:"chat"`
	Sender  string    `json:"sender"`
	WaID    string    `json:"wa_id"`
	IsGroup bool      `json:"is_group,omitempty"`
	SentAt  time.Time `json:"sent_at,omitzero"`
	Sealed  string    `json:"sealed"`
}

type msgContent struct {
	Kind            string    `json:"kind"`
	Body            string    `json:"body,omitempty"`
	MimeType        string    `json:"mime_type,omitempty"`
	IsEdit          bool      `json:"is_edit,omitempty"`
	EditTarget      string    `json:"edit_target,omitempty"`
	Mentions        []string  `json:"mentions,omitempty"`
	Quote           *quoteRef `json:"quote,omitempty"`
	Forwarded       bool      `json:"forwarded,omitempty"`
	ForwardingScore int32     `json:"forwarding_score,omitempty"`
	Ephemeral       bool      `json:"ephemeral,omitempty"`
	ViewOnce        bool      `json:"view_once,omitempty"`
	Media           *mediaRef `json:"media,omitempty"`
}

// quoteRef carries the quoted body because history never syncs in gateway
// mode — nik may have no record of the message being replied to
type quoteRef struct {
	StanzaID    string `json:"stanza_id"`
	Participant string `json:"participant,omitempty"`
	Body        string `json:"body,omitempty"`
	Kind        string `json:"kind,omitempty"`
}

// mediaRef names an attachment the gateway already fetched and sealed. it
// carries no whatsapp media key: the gateway downloads, and a key that opens a
// photo is the photo.
type mediaRef struct {
	DownloadID string `json:"download_id,omitempty"`
	MMSType    string `json:"mms_type,omitempty"`
	SizeBytes  int64  `json:"size_bytes,omitempty"`
}

type convIn struct {
	Chat    string `json:"chat"`
	IsGroup bool   `json:"is_group,omitempty"`
	Sealed  string `json:"sealed"`
}

type convContent struct {
	Kind         string   `json:"kind"`
	Title        string   `json:"title,omitempty"`
	Topic        string   `json:"topic,omitempty"`
	Owner        string   `json:"owner,omitempty"`
	Participants []string `json:"participants,omitempty"`
	IsAnnounce   bool     `json:"is_announce,omitempty"`
	IsLocked     bool     `json:"is_locked,omitempty"`
}

type msgOut struct {
	Chat  string    `json:"chat"`
	Text  string    `json:"text"`
	Quote *quoteRef `json:"quote,omitempty"`
}

// mediaOut names bytes already uploaded to POST /v1/media; attachments never
// ride the websocket
type mediaOut struct {
	Chat    string `json:"chat"`
	Handle  string `json:"handle"`
	Kind    string `json:"kind"`
	Caption string `json:"caption,omitempty"`
}

type reactOut struct {
	Chat        string `json:"chat"`
	WaID        string `json:"wa_id"`
	Participant string `json:"participant,omitempty"`
	Emoji       string `json:"emoji"`
}

type typingOut struct {
	Chat  string `json:"chat"`
	State string `json:"state"`
}

type readOut struct {
	Chat        string   `json:"chat"`
	WaIDs       []string `json:"wa_ids"`
	Participant string   `json:"participant,omitempty"`
}

type ack struct {
	IDs []string `json:"ids"`
}

type protocolError struct {
	Ref    string `json:"ref,omitempty"`
	Reason string `json:"reason"`
}

func newEnvelope(t envelopeType, payload any) (envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return envelope{}, fmt.Errorf("marshal %s: %w", t, err)
	}

	return envelope{ID: id.V7(), Type: t, TS: time.Now().UTC(), Payload: raw}, nil
}

func decodePayload[P any](e envelope) (P, error) {
	var p P

	err := json.Unmarshal(e.Payload, &p)
	if err != nil {
		return p, fmt.Errorf("decode %s: %w", e.Type, err)
	}

	return p, nil
}
