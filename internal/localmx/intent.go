package localmx

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Intent acts as a single Matrix user: the bridge bot, a ghost for a remote
// contact, or the local user's double puppet.
type Intent struct {
	c    *Connector
	mxid id.UserID
}

var _ bridgev2.MatrixAPI = (*Intent)(nil)

func (in *Intent) GetMXID() id.UserID   { return in.mxid }
func (in *Intent) IsDoublePuppet() bool { return in.mxid == in.c.Store.Me }

func (in *Intent) SendMessage(ctx context.Context, roomID id.RoomID, eventType event.Type, content *event.Content, extra *bridgev2.MatrixSendExtra) (*mautrix.RespSendEvent, error) {
	ts := time.Now()
	if extra != nil && !extra.Timestamp.IsZero() {
		ts = extra.Timestamp
	}
	evt := &event.Event{
		ID:        newEventID(),
		RoomID:    roomID,
		Sender:    in.mxid,
		Type:      eventType,
		Content:   *content,
		Timestamp: ts.UnixMilli(),
	}
	if err := in.c.Store.HandleEvent(ctx, evt, true); err != nil {
		return nil, err
	}
	return &mautrix.RespSendEvent{EventID: evt.ID}, nil
}

func (in *Intent) SendState(ctx context.Context, roomID id.RoomID, eventType event.Type, stateKey string, content *event.Content, ts time.Time) (*mautrix.RespSendEvent, error) {
	if err := in.c.Store.HandleState(ctx, roomID, eventType, stateKey, content); err != nil {
		return nil, err
	}
	return &mautrix.RespSendEvent{EventID: newEventID()}, nil
}

func (in *Intent) MarkRead(ctx context.Context, roomID id.RoomID, eventID id.EventID, ts time.Time) error {
	if in.mxid != in.c.Store.Me {
		return nil
	}
	return in.c.Store.MarkRead(ctx, roomID)
}

func (in *Intent) MarkUnread(ctx context.Context, roomID id.RoomID, unread bool) error {
	return in.c.Store.MarkUnread(ctx, roomID, unread)
}

func (in *Intent) MarkTyping(ctx context.Context, roomID id.RoomID, typingType bridgev2.TypingType, timeout time.Duration) error {
	if timeout > 0 {
		in.c.Store.notify("typing", roomID)
	}
	return nil
}

func (in *Intent) DownloadMedia(ctx context.Context, uri id.ContentURIString, file *event.EncryptedFileInfo) ([]byte, error) {
	if file != nil {
		uri = file.URL
	}
	path, _, err := in.c.Store.MediaPath(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("unknown media %s: %w", uri, err)
	}
	return os.ReadFile(path)
}

func (in *Intent) DownloadMediaToFile(ctx context.Context, uri id.ContentURIString, file *event.EncryptedFileInfo, writable bool, callback func(*os.File) error) error {
	if file != nil {
		uri = file.URL
	}
	path, _, err := in.c.Store.MediaPath(ctx, uri)
	if err != nil {
		return fmt.Errorf("unknown media %s: %w", uri, err)
	}
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	if !writable {
		return callback(src)
	}
	tmp, err := os.CreateTemp("", "pigeonhole-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	if _, err = io.Copy(tmp, src); err != nil {
		return err
	}
	if _, err = tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return callback(tmp)
}

func (in *Intent) newMediaFile() (id.ContentURIString, string) {
	mediaID := randomID()
	return id.ContentURIString("mxc://" + ServerName + "/" + mediaID), filepath.Join(in.c.Store.MediaDir, mediaID)
}

func (in *Intent) UploadMedia(ctx context.Context, roomID id.RoomID, data []byte, fileName, mimeType string) (id.ContentURIString, *event.EncryptedFileInfo, error) {
	mxc, path := in.newMediaFile()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", nil, err
	}
	return mxc, nil, in.c.Store.SaveMedia(ctx, mxc, path, mimeType)
}

func (in *Intent) UploadMediaStream(ctx context.Context, roomID id.RoomID, size int64, requireFile bool, cb bridgev2.FileStreamCallback) (id.ContentURIString, *event.EncryptedFileInfo, error) {
	mxc, path := in.newMediaFile()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
	if err != nil {
		return "", nil, err
	}
	res, err := cb(f)
	_ = f.Close()
	if err != nil {
		_ = os.Remove(path)
		return "", nil, bridgev2.CallbackError{Type: "write", Wrapped: err}
	}
	if res.ReplacementFile != "" {
		if err = moveFile(res.ReplacementFile, path); err != nil {
			return "", nil, err
		}
	}
	return mxc, nil, in.c.Store.SaveMedia(ctx, mxc, path, res.MimeType)
}

func moveFile(from, to string) error {
	if os.Rename(from, to) == nil {
		return nil
	}
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(to)
	if err != nil {
		return err
	}
	defer dst.Close()
	if _, err = io.Copy(dst, src); err != nil {
		return err
	}
	return os.Remove(from)
}

func (in *Intent) SetDisplayName(ctx context.Context, name string) error {
	return in.c.Store.SetProfile(ctx, in.mxid, &name, nil)
}

func (in *Intent) SetAvatarURL(ctx context.Context, avatarURL id.ContentURIString) error {
	avatar := string(avatarURL)
	return in.c.Store.SetProfile(ctx, in.mxid, nil, &avatar)
}

func (in *Intent) SetExtraProfileMeta(ctx context.Context, data any) error { return nil }
func (in *Intent) SetProfile(ctx context.Context, data any) error          { return nil }

func (in *Intent) CreateRoom(ctx context.Context, req *mautrix.ReqCreateRoom) (id.RoomID, error) {
	roomID := req.BeeperLocalRoomID
	if roomID == "" {
		roomID = id.RoomID("!" + randomID() + ":" + ServerName)
	}
	store := in.c.Store
	if err := store.CreateRoom(ctx, roomID, in.c.Network, req.IsDirect, req.Name, req.Topic); err != nil {
		return "", err
	}
	if err := store.SetMembership(ctx, roomID, in.mxid, event.MembershipJoin); err != nil {
		return "", err
	}
	for _, userID := range req.Invite {
		if err := store.SetMembership(ctx, roomID, userID, event.MembershipJoin); err != nil {
			return "", err
		}
	}
	for _, evt := range req.InitialState {
		stateKey := ""
		if evt.StateKey != nil {
			stateKey = *evt.StateKey
		}
		if err := store.HandleState(ctx, roomID, evt.Type, stateKey, &evt.Content); err != nil {
			return "", err
		}
	}
	return roomID, nil
}

func (in *Intent) DeleteRoom(ctx context.Context, roomID id.RoomID, puppetsOnly bool) error {
	return in.c.Store.DeleteRoom(ctx, roomID)
}

func (in *Intent) EnsureJoined(ctx context.Context, roomID id.RoomID, params ...bridgev2.EnsureJoinedParams) error {
	return in.c.Store.SetMembership(ctx, roomID, in.mxid, event.MembershipJoin)
}

func (in *Intent) EnsureInvited(ctx context.Context, roomID id.RoomID, userID id.UserID) error {
	return in.c.Store.SetMembership(ctx, roomID, userID, event.MembershipJoin)
}

func (in *Intent) TagRoom(ctx context.Context, roomID id.RoomID, tag event.RoomTag, isTagged bool) error {
	return nil
}

func (in *Intent) MuteRoom(ctx context.Context, roomID id.RoomID, until time.Time) error {
	return in.c.Store.Mute(ctx, roomID, until)
}

func (in *Intent) GetEvent(ctx context.Context, roomID id.RoomID, eventID id.EventID) (*event.Event, error) {
	return in.c.Store.GetEvent(ctx, eventID)
}
