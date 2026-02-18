package whatsapp

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"os"
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
	mediaDir  string
	handlers  []func(any)
}

func NewClient(sessionPath string, mediaPath string, mediaDir string) (*Client, error) {
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
		mediaDir:  mediaDir,
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

func (c *Client) React(ctx context.Context, conversationJID, msgID, senderJID, emoji string) error {
	conversation, err := types.ParseJID(conversationJID)
	if err != nil {
		return fmt.Errorf("parse conversation jid: %w", err)
	}

	sender, _ := types.ParseJID(senderJID)
	msg := c.wm.BuildReaction(conversation, sender, types.MessageID(msgID), emoji)
	_, err = c.wm.SendMessage(ctx, conversation, msg)
	if err != nil {
		return fmt.Errorf("send reaction: %w", err)
	}

	return nil
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
