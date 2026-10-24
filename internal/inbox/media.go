package inbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "golang.org/x/image/webp"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/provisionutil"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"unified-inbox/internal/localmx"
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

// Common extensions by type; mime.ExtensionsByType returns them sorted, so
// for JPEG it would pick ".jfif" and for plain text ".conf".
var mediaExtensions = map[string]string{
	"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp",
	"image/heic": ".heic", "video/mp4": ".mp4", "video/quicktime": ".mov", "audio/mpeg": ".mp3",
	"audio/ogg": ".ogg", "audio/mp4": ".m4a", "application/pdf": ".pdf", "text/plain": ".txt",
}

// MediaFileName is the name to give a stored file outside the app: the
// original name if the message had one, with an extension that matches its
// type, since stored media files have no extension.
func MediaFileName(fileName, mimeType, fallback string) string {
	name := strings.TrimSpace(filepath.Base(strings.ReplaceAll(fileName, "\\", "/")))
	if name == "." || name == "/" || name == "" {
		name = fallback
	}
	if filepath.Ext(name) != "" {
		return name
	}
	ext := mediaExtensions[strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))]
	if ext == "" {
		if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
			ext = exts[0]
		}
	}
	return name + ext
}

// MediaCopy copies a stored file (by the /media/ URL the UI shows) to dir,
// named by MediaFileName, and returns its path. Opening and saving images
// use it.
func (s *Service) MediaCopy(ctx context.Context, mediaURL, fileName, dir string) (string, error) {
	mediaID, ok := strings.CutPrefix(mediaURL, mediaPrefix)
	if !ok || mediaID == "" || strings.ContainsAny(mediaID, "/\\") {
		return "", fmt.Errorf("not a media link: %q", mediaURL)
	}
	src, mimeType, err := s.Store.MediaPath(ctx, id.ContentURIString("mxc://"+localmx.ServerName+"/"+mediaID))
	if err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, MediaFileName(fileName, mimeType, mediaID))
	return dest, copyFile(src, dest)
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// resolveMediaTimeout bounds fetching one reel, story or post from Instagram.
const resolveMediaTimeout = time.Minute

// ResolveMedia asks the bridge to fetch the real media for a message that
// was stored as a thumbnail and link (Instagram reels, stories and posts from
// history). The bridge replaces the message with the media by sending an
// edit, which the store applies like any other, so the chat just refreshes.
func (s *Service) ResolveMedia(ctx context.Context, roomID, eventID string) error {
	var raw string
	err := s.Store.DB.QueryRowContext(ctx, "SELECT content FROM events WHERE event_id=? AND room_id=?", eventID, roomID).Scan(&raw)
	if err != nil {
		return fmt.Errorf("message not found: %w", err)
	}
	var content map[string]json.RawMessage
	if err = json.Unmarshal([]byte(raw), &content); err != nil {
		return err
	}
	unresolved, ok := content[unresolvedMediaKey]
	if !ok {
		return nil // already fetched
	}
	n, err := s.roomNetwork(ctx, roomID)
	if err != nil {
		return err
	}
	login := s.loginForRoom(ctx, n, roomID)
	if login == nil {
		return fmt.Errorf("not logged in to %s", n.Name)
	}
	ctx, cancel := context.WithTimeout(ctx, resolveMediaTimeout)
	defer cancel()
	_, err = provisionutil.ResolveMedia(ctx, login, id.EventID(eventID), &provisionutil.ReqResolveMedia{UnresolvedMedia: unresolved})
	if err != nil {
		s.Log.Warn().Err(err).Str("network", n.ID).Str("event_id", eventID).Msg("Failed to fetch shared media")
		return fmt.Errorf("couldn't load it from %s. It may have been deleted, or the account is private", n.Name)
	}
	return nil
}

// loginForRoom is the account a chat belongs to: its portal's receiver, or
// the only account on that network.
func (s *Service) loginForRoom(ctx context.Context, n *Network, roomID string) *bridgev2.UserLogin {
	if !n.started.Load() {
		return nil
	}
	if portal := s.portalFor(ctx, n.ID, roomID); portal != nil && portal.Receiver != "" {
		if login := n.Bridge.GetCachedUserLoginByID(portal.Receiver); login != nil {
			return login
		}
	}
	user, err := n.Bridge.GetUserByMXID(ctx, Me)
	if err != nil {
		return nil
	}
	if logins := user.GetUserLogins(); len(logins) > 0 {
		return logins[0]
	}
	return nil
}
