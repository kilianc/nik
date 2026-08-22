package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// the fixtures in testdata/ are byte-identical copies of the platform's. this
// test and its twin on the gateway side are the only thing keeping two hand-maintained
// copies of the wire format in agreement, so it asserts field values rather
// than just decoding without error — a struct tag typo that silently drops a
// field would sail past an err == nil check.

func loadEnvelope(t *testing.T, name string) envelope {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}

	var env envelope
	err = json.Unmarshal(raw, &env)
	if err != nil {
		t.Fatalf("decode envelope %s: %v", name, err)
	}

	return env
}

func decodeFixture[P any](t *testing.T, name string, want envelopeType) P {
	t.Helper()

	env := loadEnvelope(t, name)
	if env.Type != want {
		t.Fatalf("%s: type = %q, want %q", name, env.Type, want)
	}
	if env.ID == "" {
		t.Errorf("%s: empty envelope id", name)
	}
	if env.TS.IsZero() {
		t.Errorf("%s: zero envelope timestamp", name)
	}

	p, err := decodePayload[P](env)
	if err != nil {
		t.Fatalf("%s: decode payload: %v", name, err)
	}

	return p
}

// a verb without a fixture is a field that can reach the platform with nothing
// forcing the two copies to agree
func TestFixturesCoverEveryVerb(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.json"))
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}

	seen := map[envelopeType]bool{}
	for _, f := range files {
		seen[loadEnvelope(t, filepath.Base(f)).Type] = true
	}

	want := []envelopeType{
		typeHello, typeHelloAck, typeMsgIn, typeConvIn,
		typeMsgOut, typeMediaOut, typeReactOut, typeTypingOut, typeReadOut,
		typeAck, typeError,
		typeAPIReq, typeAPIRes, typeAPIEvt,
	}
	for _, w := range want {
		if !seen[w] {
			t.Errorf("no fixture for verb %q", w)
		}
	}
}

func TestDecodeHello(t *testing.T) {
	p := decodeFixture[hello](t, "hello.json", typeHello)

	if p.Version != protocolVersion {
		t.Errorf("version = %d, want %d", p.Version, protocolVersion)
	}
	if p.NikName != "kitchen-mac" {
		t.Errorf("nik_name = %q", p.NikName)
	}
	if p.PublicKey == "" {
		t.Error("public_key is required")
	}
	if p.ClientVersion != "v0.2.0" {
		t.Errorf("client_version = %q", p.ClientVersion)
	}
}

func TestDecodeHelloAck(t *testing.T) {
	p := decodeFixture[helloAck](t, "hello.ack.json", typeHelloAck)

	if p.Number != "+16502811468" {
		t.Errorf("number = %q", p.Number)
	}
	if p.SelfJID != "16502811468@s.whatsapp.net" {
		t.Errorf("self_jid = %q — nik has no other source for its own jid", p.SelfJID)
	}
	if p.Token != "nik_rotated-example-token" {
		t.Errorf("token = %q — every connect hands nik its next token", p.Token)
	}
}

func TestDecodeMsgIn(t *testing.T) {
	dm := decodeFixture[msgIn](t, "msg.in.text.json", typeMsgIn)

	if dm.Chat != dm.Sender {
		t.Errorf("dm fixture: chat %q != sender %q", dm.Chat, dm.Sender)
	}
	if dm.IsGroup {
		t.Error("is_group = true on a dm fixture")
	}
	if dm.WaID != "3EB0C767D82B0F3C1A2B" {
		t.Errorf("wa_id = %q", dm.WaID)
	}
	if dm.Sealed == "" {
		t.Error("sealed is required — content never travels in the clear")
	}

	wantSent := time.Date(2026, 8, 13, 17, 1, 58, 0, time.UTC)
	if !dm.SentAt.Equal(wantSent) {
		t.Errorf("sent_at = %v, want %v", dm.SentAt, wantSent)
	}

	group := decodeFixture[msgIn](t, "msg.in.group-media.json", typeMsgIn)

	if !group.IsGroup {
		t.Error("is_group = false on a group fixture")
	}
	if group.Chat == group.Sender {
		t.Error("group fixture: chat should differ from sender")
	}
}

func TestDecodeConvIn(t *testing.T) {
	p := decodeFixture[convIn](t, "conv.in.json", typeConvIn)

	if !p.IsGroup {
		t.Error("is_group = false on a group fixture")
	}
	if p.Sealed == "" {
		t.Error("sealed is required — a group title is content")
	}
}

func TestDecodeMsgOut(t *testing.T) {
	p := decodeFixture[msgOut](t, "msg.out.json", typeMsgOut)

	if p.Text == "" {
		t.Error("empty text")
	}
	if p.Quote == nil {
		t.Fatal("quote missing")
	}
	if p.Quote.StanzaID != "3EB0AA11BB22CC33DD44" {
		t.Errorf("quote.stanza_id = %q", p.Quote.StanzaID)
	}
}

func TestDecodeMediaOut(t *testing.T) {
	p := decodeFixture[mediaOut](t, "media.out.json", typeMediaOut)

	if p.Handle == "" {
		t.Error("handle is required — bytes never ride the websocket")
	}
	if p.Kind != mediaVoice {
		t.Errorf("kind = %q, want %q", p.Kind, mediaVoice)
	}
}

func TestDecodeTransientVerbs(t *testing.T) {
	react := decodeFixture[reactOut](t, "react.out.json", typeReactOut)
	if react.Emoji != "🦊" {
		t.Errorf("emoji = %q — multi-byte emoji must survive the round trip", react.Emoji)
	}
	if react.WaID == "" {
		t.Error("wa_id is required")
	}

	typing := decodeFixture[typingOut](t, "typing.out.json", typeTypingOut)
	if typing.State != "start" {
		t.Errorf("state = %q", typing.State)
	}

	read := decodeFixture[readOut](t, "read.out.json", typeReadOut)
	if len(read.WaIDs) != 1 || read.WaIDs[0] != "3EB0C767D82B0F3C1A2B" {
		t.Errorf("wa_ids = %v", read.WaIDs)
	}
}

func TestDecodeAckAndError(t *testing.T) {
	a := decodeFixture[ack](t, "ack.json", typeAck)
	if len(a.IDs) != 1 {
		t.Errorf("ids = %v", a.IDs)
	}

	e := decodeFixture[protocolError](t, "error.json", typeError)
	if e.Ref == "" || e.Reason == "" {
		t.Errorf("ref = %q, reason = %q", e.Ref, e.Reason)
	}
}

// the sealed payloads have no envelope fixture — the wire only ever shows
// ciphertext — but both sides must map them identically
func TestSealedPayloadRoundTrip(t *testing.T) {
	msg := msgContent{
		Kind:     kindImage,
		Body:     "look at this",
		MimeType: "image/jpeg",
		Mentions: []string{"14155551234@s.whatsapp.net"},
		Quote:    &quoteRef{StanzaID: "3EB0", Body: "what's for dinner?"},
		Media:    &mediaRef{DownloadID: "nik.9f8e", MMSType: "image", SizeBytes: 84213},
	}

	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}

	var got msgContent
	err = json.Unmarshal(raw, &got)
	if err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}

	if got.Media == nil || got.Media.DownloadID != "nik.9f8e" {
		t.Error("attachment reference lost in round trip")
	}
	if got.Quote == nil || got.Quote.Body != "what's for dinner?" {
		t.Error("quote lost in round trip")
	}

	// the platform strips whatsapp's own descriptor before sealing; if these
	// ever appear, nik is being handed keys it has no business holding
	for _, leaked := range []string{"media_key", "direct_path", "file_enc_sha256", "cdn_host"} {
		if strings.Contains(string(raw), leaked) {
			t.Errorf("content carries %q", leaked)
		}
	}

	conv := convContent{
		Kind:         convGroup,
		Title:        "Familia 🇮🇹",
		Participants: []string{"14155551234@s.whatsapp.net"},
	}

	raw, err = json.Marshal(conv)
	if err != nil {
		t.Fatalf("marshal conv: %v", err)
	}

	var gotConv convContent
	err = json.Unmarshal(raw, &gotConv)
	if err != nil {
		t.Fatalf("unmarshal conv: %v", err)
	}

	if gotConv.Title != conv.Title {
		t.Errorf("title = %q, want %q", gotConv.Title, conv.Title)
	}
}

// v3: the API tunnel. Three verbs rather than one per console feature, so this
// is the last console-shaped bump — see the note in protocol.go. Their
// fixtures live under the same rule as every other verb: byte-identical to the
// platform's, and asserted field by field, since a struct tag typo that
// silently drops session_key is a response nobody can open.

func TestAPIReqFixture(t *testing.T) {
	req := decodeFixture[apiReq](t, "api.req.json", typeAPIReq)

	if req.Method != "GET" {
		t.Errorf("method = %q, want GET", req.Method)
	}
	if req.Path != "/v1/conversations/local/messages" {
		t.Errorf("path = %q", req.Path)
	}
	if req.Query != "limit=50" {
		t.Errorf("query = %q", req.Query)
	}
	if req.SessionKey == "" {
		t.Error("empty session_key — the response would have nowhere to be sealed to")
	}

	// The session key has to be a usable X25519 public key, not merely a
	// non-empty string: this is what proves the fixture's shape is real.
	if _, err := decodeSessionKey(req.SessionKey); err != nil {
		t.Errorf("session_key does not decode: %v", err)
	}
}

func TestAPIResFixture(t *testing.T) {
	res := decodeFixture[apiRes](t, "api.res.json", typeAPIRes)

	if res.Ref == "" {
		t.Error("empty ref — nothing could match this to its request")
	}
	if res.Status != 200 {
		t.Errorf("status = %d, want 200", res.Status)
	}
	if res.Sealed == "" {
		t.Error("empty sealed body")
	}
}

func TestAPIEvtFixture(t *testing.T) {
	evt := decodeFixture[apiEvt](t, "api.evt.json", typeAPIEvt)

	if evt.Ref == "" {
		t.Error("empty ref — nothing could match this to the stream it belongs to")
	}
	if evt.Event != "message" {
		t.Errorf("event = %q, want message", evt.Event)
	}
	if evt.Sealed == "" {
		t.Error("empty sealed payload")
	}
}

// The version nik announces is not the highest it can speak, and that gap is
// deliberate: today's gateway accepts exactly 2 and refuses anything else, so
// announcing 3 before the platform accepts a range would take every household
// offline. This test is here so the two constants cannot drift apart by
// accident — when the gateway accepts a range, this is what gets updated
// alongside the bump.
func TestAnnouncedVersionTrailsWhatWeCanSpeak(t *testing.T) {
	if protocolVersion > maxProtocolVersion {
		t.Fatalf("announcing v%d but can only speak v%d", protocolVersion, maxProtocolVersion)
	}
	if protocolVersion != 2 {
		t.Fatalf("announced version is %d; bumping it requires the gateway to accept a range first (see protocol.go)", protocolVersion)
	}
}
