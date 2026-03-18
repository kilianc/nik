package whatsapp

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"time"

	"github.com/kciuffolo/nik/internal/messaging"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	_ "github.com/mattn/go-sqlite3"
)

type Client struct {
	wm        *whatsmeow.Client
	container *sqlstore.Container
	mediaPath string
	handlers  []func(any)
}

func NewClient(sessionPath string, mediaPath string) (*Client, error) {
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on", sessionPath)
	container, err := sqlstore.New(context.Background(), "sqlite3", dsn, newWaLogger("whatsmeow/db"))
	if err != nil {
		return nil, fmt.Errorf("open whatsmeow store: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return nil, fmt.Errorf("get device: %w", err)
	}

	wm := whatsmeow.NewClient(deviceStore, newWaLogger("whatsmeow"))
	if wm.Store.PushName == "" {
		wm.Store.PushName = "Nik"
	}

	return &Client{
		wm:        wm,
		container: container,
		mediaPath: mediaPath,
	}, nil
}

func (c *Client) AddEventHandler(handler func(any)) {
	c.handlers = append(c.handlers, handler)
	c.wm.AddEventHandler(handler)
}

func (c *Client) Connect(ctx context.Context, forceLink bool) error {
	if forceLink {
		_ = c.wm.Store.Delete(ctx)
		c.wm.Store.ID = nil
	}

	if c.wm.Store.ID == nil {
		qrChan, err := c.wm.GetQRChannel(ctx)
		if err != nil {
			return fmt.Errorf("get qr channel: %w", err)
		}
		if err := c.wm.ConnectContext(ctx); err != nil {
			return fmt.Errorf("connect: %w", err)
		}

		for item := range qrChan {
			if item.Event == "code" {
				fmt.Println()
				qrterminal.Generate(item.Code, qrterminal.L, os.Stdout)
				fmt.Println()
				continue
			}
			if item.Event == "success" {
				break
			}
			if item.Error != nil {
				return fmt.Errorf("qr pairing: %w", item.Error)
			}
		}
	} else {
		if err := c.wm.ConnectContext(ctx); err != nil {
			return fmt.Errorf("connect: %w", err)
		}
	}

	return nil
}

func (c *Client) ReplayHistorySync(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open sync recording: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		raw, decodeErr := base64.StdEncoding.DecodeString(scanner.Text())
		if decodeErr != nil {
			return fmt.Errorf("decode history chunk: %w", decodeErr)
		}

		data := &waHistorySync.HistorySync{}
		err = proto.Unmarshal(raw, data)
		if err != nil {
			return fmt.Errorf("unmarshal history chunk: %w", err)
		}

		evt := &events.HistorySync{Data: data}
		for _, handler := range c.handlers {
			handler(evt)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan history sync file: %w", err)
	}

	return nil
}

func (c *Client) SetPresence(ctx context.Context, available bool) error {
	p := types.PresenceUnavailable
	if available {
		p = types.PresenceAvailable
	}
	return c.wm.SendPresence(ctx, p)
}

func (c *Client) Reply(ctx context.Context, conversationJID, text string) (messaging.OutboundMessage, error) {
	jid, err := types.ParseJID(conversationJID)
	if err != nil {
		return messaging.OutboundMessage{}, fmt.Errorf("parse conversation jid: %w", err)
	}

	msg := &waProto.Message{Conversation: &text}
	resp, err := c.wm.SendMessage(ctx, jid, msg)
	if err != nil {
		return messaging.OutboundMessage{}, fmt.Errorf("send message: %w", err)
	}

	externalSenderID := resp.Sender.String()
	if externalSenderID == "" {
		externalSenderID = c.SelfJID()
	}

	return messaging.OutboundMessage{
		ExternalMessageID: string(resp.ID),
		ExternalSenderID:  externalSenderID,
		SentAt:            resp.Timestamp,
		Kind:              "text",
		Body:              text,
	}, nil
}

func (c *Client) SendImage(ctx context.Context, conversationJID, imagePath, caption string) (messaging.OutboundMessage, error) {
	jid, err := types.ParseJID(conversationJID)
	if err != nil {
		return messaging.OutboundMessage{}, fmt.Errorf("parse conversation jid: %w", err)
	}

	data, err := os.ReadFile(imagePath)
	if err != nil {
		return messaging.OutboundMessage{}, fmt.Errorf("read image file: %w", err)
	}

	mimeType := mime.TypeByExtension(filepath.Ext(imagePath))
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	resp, err := c.wm.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return messaging.OutboundMessage{}, fmt.Errorf("upload image: %w", err)
	}

	imageMsg := &waProto.ImageMessage{
		Caption:       proto.String(caption),
		Mimetype:      proto.String(mimeType),
		URL:           &resp.URL,
		DirectPath:    &resp.DirectPath,
		MediaKey:      resp.MediaKey,
		FileEncSHA256: resp.FileEncSHA256,
		FileSHA256:    resp.FileSHA256,
		FileLength:    &resp.FileLength,
	}

	sendResp, err := c.wm.SendMessage(ctx, jid, &waProto.Message{ImageMessage: imageMsg})
	if err != nil {
		return messaging.OutboundMessage{}, fmt.Errorf("send image: %w", err)
	}

	externalSenderID := sendResp.Sender.String()
	if externalSenderID == "" {
		externalSenderID = c.SelfJID()
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	return messaging.OutboundMessage{
		ExternalMessageID: string(sendResp.ID),
		ExternalSenderID:  externalSenderID,
		SentAt:            sendResp.Timestamp,
		Kind:              "image",
		Body:              caption,
		MimeType:          mimeType,
		LocalPath:         hash + filepath.Ext(imagePath),
	}, nil
}

func (c *Client) SendAudio(ctx context.Context, conversationJID, audioPath string, voiceNote bool) (messaging.OutboundMessage, error) {
	jid, err := types.ParseJID(conversationJID)
	if err != nil {
		return messaging.OutboundMessage{}, fmt.Errorf("parse conversation jid: %w", err)
	}

	data, err := os.ReadFile(audioPath)
	if err != nil {
		return messaging.OutboundMessage{}, fmt.Errorf("read audio file: %w", err)
	}

	resp, err := c.wm.Upload(ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		return messaging.OutboundMessage{}, fmt.Errorf("upload audio: %w", err)
	}

	audioMsg := &waProto.AudioMessage{
		Mimetype:      proto.String("audio/ogg; codecs=opus"),
		URL:           &resp.URL,
		DirectPath:    &resp.DirectPath,
		MediaKey:      resp.MediaKey,
		FileEncSHA256: resp.FileEncSHA256,
		FileSHA256:    resp.FileSHA256,
		FileLength:    &resp.FileLength,
		PTT:           proto.Bool(voiceNote),
	}

	sendResp, err := c.wm.SendMessage(ctx, jid, &waProto.Message{AudioMessage: audioMsg})
	if err != nil {
		return messaging.OutboundMessage{}, fmt.Errorf("send audio: %w", err)
	}

	externalSenderID := sendResp.Sender.String()
	if externalSenderID == "" {
		externalSenderID = c.SelfJID()
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	return messaging.OutboundMessage{
		ExternalMessageID: string(sendResp.ID),
		ExternalSenderID:  externalSenderID,
		SentAt:            sendResp.Timestamp,
		Kind:              "audio",
		MimeType:          "audio/ogg; codecs=opus",
		LocalPath:         hash + ".ogg",
	}, nil
}

func (c *Client) React(ctx context.Context, conversationJID, msgID, senderJID, emoji string) (messaging.OutboundMessage, error) {
	conversation, err := types.ParseJID(conversationJID)
	if err != nil {
		return messaging.OutboundMessage{}, fmt.Errorf("parse conversation jid: %w", err)
	}

	sender, _ := types.ParseJID(senderJID)
	msg := c.wm.BuildReaction(conversation, sender, types.MessageID(msgID), emoji)
	resp, err := c.wm.SendMessage(ctx, conversation, msg)
	if err != nil {
		return messaging.OutboundMessage{}, fmt.Errorf("send reaction: %w", err)
	}

	externalSenderID := resp.Sender.String()
	if externalSenderID == "" {
		externalSenderID = c.SelfJID()
	}

	return messaging.OutboundMessage{
		ExternalMessageID: string(resp.ID),
		ExternalSenderID:  externalSenderID,
		SentAt:            resp.Timestamp,
		Kind:              "reaction",
		Body:              emoji,
	}, nil
}

func (c *Client) StartTyping(ctx context.Context, conversationJID string) error {
	jid, err := types.ParseJID(conversationJID)
	if err != nil {
		return fmt.Errorf("parse conversation jid: %w", err)
	}

	return c.wm.SendChatPresence(ctx, jid, types.ChatPresenceComposing, types.ChatPresenceMediaText)
}

func (c *Client) StopTyping(ctx context.Context, conversationJID string) error {
	jid, err := types.ParseJID(conversationJID)
	if err != nil {
		return fmt.Errorf("parse conversation jid: %w", err)
	}

	return c.wm.SendChatPresence(ctx, jid, types.ChatPresencePaused, types.ChatPresenceMediaText)
}

func (c *Client) MarkRead(ctx context.Context, conversationJID string, msgIDs []string, senderJID string, ts time.Time) error {
	if len(msgIDs) == 0 {
		return nil
	}

	conversation, err := types.ParseJID(conversationJID)
	if err != nil {
		return fmt.Errorf("parse conversation jid: %w", err)
	}

	var sender types.JID
	if senderJID != "" {
		sender, _ = types.ParseJID(senderJID)
	}

	return c.wm.MarkRead(ctx, msgIDs, ts, conversation, sender)
}

func (c *Client) Close() {
	if c.wm != nil {
		c.wm.Disconnect()
	}
}

func (c *Client) SelfJID() string {
	if c.wm == nil || c.wm.Store == nil || c.wm.Store.ID == nil {
		return ""
	}

	return c.wm.Store.ID.String()
}
