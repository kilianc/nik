package whatsapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"mime"
	"os"
	"path/filepath"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
)

type mediaResult struct {
	path      string
	filename  string
	hash      string
	mimeType  string
	sizeBytes int64
}

// downloadMedia downloads a message's media attachment to disk.
// returns nil if the message has no downloadable media or the download fails.
// on success, all fields in the result are guaranteed non-empty.
func (c *Client) downloadMedia(ctx context.Context, msg *waProto.Message, messageID, kind string) *mediaResult {
	if c.mediaPath == "" {
		return nil
	}

	downloadable, rawMime := extractDownloadable(msg, kind)
	if downloadable == nil {
		return nil
	}

	mimeType := normalizeMime(rawMime)

	data, err := c.wm.Download(ctx, downloadable)
	if err != nil {
		slog.Warn("download media failed", "pkg", "whatsapp", "msg_id", messageID, "kind", kind, "error", err)
		return nil
	}

	err = os.MkdirAll(c.mediaPath, 0o755)
	if err != nil {
		slog.Warn("create media dir failed", "pkg", "whatsapp", "dir", c.mediaPath, "error", err)
		return nil
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	filename := hash + extensionFromMime(mimeType)
	absPath := filepath.Join(c.mediaPath, filename)

	_, err = os.Stat(absPath)
	if err == nil {
		slog.Info("media reused", "pkg", "whatsapp", "msg_id", messageID, "kind", kind, "path", absPath, "bytes", len(data))
		return &mediaResult{path: absPath, filename: filename, hash: hash, mimeType: mimeType, sizeBytes: int64(len(data))}
	}

	err = os.WriteFile(absPath, data, 0o644)
	if err != nil {
		slog.Warn("write media file failed", "pkg", "whatsapp", "path", absPath, "error", err)
		return nil
	}

	slog.Info("media downloaded", "pkg", "whatsapp", "msg_id", messageID, "kind", kind, "path", absPath, "bytes", len(data))
	return &mediaResult{path: absPath, filename: filename, hash: hash, mimeType: mimeType, sizeBytes: int64(len(data))}
}

func extractDownloadable(msg *waProto.Message, kind string) (whatsmeow.DownloadableMessage, string) {
	if msg == nil {
		return nil, ""
	}

	switch kind {
	case "image":
		if img := msg.GetImageMessage(); img != nil {
			return img, img.GetMimetype()
		}
	case "audio":
		if audio := msg.GetAudioMessage(); audio != nil {
			return audio, audio.GetMimetype()
		}
	case "video":
		if video := msg.GetVideoMessage(); video != nil {
			return video, video.GetMimetype()
		}
	case "ptv":
		if ptv := msg.GetPtvMessage(); ptv != nil {
			return ptv, ptv.GetMimetype()
		}
	case "document":
		if doc := msg.GetDocumentMessage(); doc != nil {
			return doc, doc.GetMimetype()
		}
	case "sticker":
		if sticker := msg.GetStickerMessage(); sticker != nil {
			return sticker, sticker.GetMimetype()
		}
	}

	return nil, ""
}

// normalizeMime strips parameters and defaults empty to application/octet-stream.
func normalizeMime(raw string) string {
	mediaType, _, _ := mime.ParseMediaType(raw)
	if mediaType == "" {
		return "application/octet-stream"
	}
	return mediaType
}

func extensionFromMime(mimeType string) string {
	exts, _ := mime.ExtensionsByType(mimeType)
	if len(exts) > 0 {
		return exts[0]
	}
	return ".bin"
}
