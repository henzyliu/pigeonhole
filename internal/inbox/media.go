package inbox

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/webp"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// MaxUploadSize matches Meta's attachment limit.
const MaxUploadSize = 25 * 1024 * 1024

// SendFile uploads a local file into the store and sends it as a photo,
// video, voice/audio or generic file message. The bridge then reads it back
// from the store and uploads it to the network.
func (s *Service) SendFile(ctx context.Context, roomID, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	} else if info.IsDir() {
		return fmt.Errorf("%s is a folder", filepath.Base(path))
	} else if info.Size() > MaxUploadSize {
		return fmt.Errorf("%s is larger than 25 MB", filepath.Base(path))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	n, err := s.roomNetwork(ctx, roomID)
	if err != nil {
		return err
	}
	name := filepath.Base(path)
	mimeType := detectMime(name, data)
	uri, _, err := n.MX.BotIntent().UploadMedia(ctx, id.RoomID(roomID), data, name, mimeType)
	if err != nil {
		return fmt.Errorf("failed to store %s: %w", name, err)
	}
	content := &event.MessageEventContent{
		MsgType:  msgTypeFor(mimeType),
		Body:     name,
		FileName: name,
		URL:      uri,
		Info:     &event.FileInfo{MimeType: mimeType, Size: len(data)},
	}
	if content.MsgType == event.MsgImage {
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
			content.Info.Width, content.Info.Height = cfg.Width, cfg.Height
		}
	}
	return s.sendLocal(ctx, roomID, event.EventMessage, content)
}

func detectMime(name string, data []byte) string {
	if byExt := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); byExt != "" {
		mimeType, _, _ := strings.Cut(byExt, ";")
		return mimeType
	}
	mimeType, _, _ := strings.Cut(http.DetectContentType(data), ";")
	return mimeType
}

func msgTypeFor(mimeType string) event.MessageType {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return event.MsgImage
	case strings.HasPrefix(mimeType, "video/"):
		return event.MsgVideo
	case strings.HasPrefix(mimeType, "audio/"):
		return event.MsgAudio
	default:
		return event.MsgFile
	}
}
