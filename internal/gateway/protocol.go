package gateway

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/kciuffolo/nik/internal/id"
)

// wire format for the nik-saas gateway (its docs/PROTOCOL.md). these types are
// a hand-maintained copy of the platform's: nik-saas keeps its packages
// internal and builds without cgo, so importing across the two was never an
// option.
//
// the copies are held together by the fixtures in testdata/, which are
// byte-identical to the platform's and decoded by a test on each side. change a
// struct tag here and the twin test over there fails, which is the point.

// protocolVersion is sent in hello; the gateway rejects anything else.
const protocolVersion = 2

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
)

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
