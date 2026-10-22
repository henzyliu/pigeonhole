// Package localmx is an in-process stand-in for a Matrix homeserver. The
// mautrix bridgev2 network connectors (Messenger, Instagram, ...) believe they
// are talking to Matrix, but every room, event and media file lands in a local
// SQLite database that the desktop UI reads directly.
package localmx

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

const schema = `
CREATE TABLE IF NOT EXISTS rooms (
	room_id     TEXT PRIMARY KEY,
	network     TEXT NOT NULL,
	is_portal   INTEGER NOT NULL DEFAULT 0,
	is_dm       INTEGER NOT NULL DEFAULT 0,
	name        TEXT NOT NULL DEFAULT '',
	avatar      TEXT NOT NULL DEFAULT '',
	topic       TEXT NOT NULL DEFAULT '',
	last_ts     INTEGER NOT NULL DEFAULT 0,
	preview     TEXT NOT NULL DEFAULT '',
	unread      INTEGER NOT NULL DEFAULT 0,
	muted_until INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS members (
	room_id    TEXT NOT NULL,
	user_id    TEXT NOT NULL,
	membership TEXT NOT NULL,
	PRIMARY KEY (room_id, user_id)
);
CREATE TABLE IF NOT EXISTS profiles (
	user_id TEXT PRIMARY KEY,
	name    TEXT NOT NULL DEFAULT '',
	avatar  TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS events (
	event_id TEXT PRIMARY KEY,
	room_id  TEXT NOT NULL,
	sender   TEXT NOT NULL,
	type     TEXT NOT NULL,
	content  TEXT NOT NULL,
	ts       INTEGER NOT NULL,
	status   TEXT NOT NULL DEFAULT '',
	edited   INTEGER NOT NULL DEFAULT 0,
	redacted INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS events_room_ts ON events (room_id, ts);
CREATE TABLE IF NOT EXISTS reactions (
	event_id TEXT PRIMARY KEY,
	room_id  TEXT NOT NULL,
	target   TEXT NOT NULL,
	sender   TEXT NOT NULL,
	key      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS reactions_target ON reactions (target);
CREATE TABLE IF NOT EXISTS media (
	mxc  TEXT PRIMARY KEY,
	path TEXT NOT NULL,
	mime TEXT NOT NULL DEFAULT ''
);
`

// Change describes what the UI should refresh.
type Change struct {
	Kind   string    `json:"kind"` // "rooms", "messages", "typing", "status"
	RoomID id.RoomID `json:"roomId,omitempty"`
}

// Store holds everything the "homeserver" knows. It is shared by all bridges.
type Store struct {
	DB       *sql.DB
	MediaDir string
	Me       id.UserID

	// IsSelf reports whether a sender is the local user (directly or as their
	// own ghost on a network). Set by the inbox service.
	IsSelf func(id.UserID) bool
	Notify func(Change)

	lock sync.Mutex
}

func OpenStore(dataDir string, me id.UserID) (*Store, error) {
	mediaDir := filepath.Join(dataDir, "media")
	if err := os.MkdirAll(mediaDir, 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(dataDir, "inbox.db")+"?_journal_mode=WAL&_busy_timeout=5000&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	if _, err = db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}
	return &Store{DB: db, MediaDir: mediaDir, Me: me}, nil
}

func (s *Store) notify(kind string, roomID id.RoomID) {
	if s.Notify != nil {
		s.Notify(Change{Kind: kind, RoomID: roomID})
	}
}

func (s *Store) isSelf(sender id.UserID) bool {
	return sender == s.Me || (s.IsSelf != nil && s.IsSelf(sender))
}

func (s *Store) CreateRoom(ctx context.Context, roomID id.RoomID, network string, isDM bool, name, topic string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO rooms (room_id, network, is_dm, name, topic) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (room_id) DO NOTHING`, roomID, network, isDM, name, topic)
	return err
}

func (s *Store) DeleteRoom(ctx context.Context, roomID id.RoomID) error {
	for _, q := range []string{
		"DELETE FROM rooms WHERE room_id=?", "DELETE FROM members WHERE room_id=?",
		"DELETE FROM events WHERE room_id=?", "DELETE FROM reactions WHERE room_id=?",
	} {
		if _, err := s.DB.ExecContext(ctx, q, roomID); err != nil {
			return err
		}
	}
	s.notify("rooms", roomID)
	return nil
}

func (s *Store) SetMembership(ctx context.Context, roomID id.RoomID, userID id.UserID, membership event.Membership) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO members (room_id, user_id, membership) VALUES (?, ?, ?)
		ON CONFLICT (room_id, user_id) DO UPDATE SET membership=excluded.membership`, roomID, userID, membership)
	return err
}

func (s *Store) Members(ctx context.Context, roomID id.RoomID) (map[id.UserID]event.Membership, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT user_id, membership FROM members WHERE room_id=?", roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[id.UserID]event.Membership)
	for rows.Next() {
		var userID, membership string
		if err = rows.Scan(&userID, &membership); err != nil {
			return nil, err
		}
		out[id.UserID(userID)] = event.Membership(membership)
	}
	return out, rows.Err()
}

func (s *Store) SetProfile(ctx context.Context, userID id.UserID, name *string, avatar *string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO profiles (user_id, name, avatar) VALUES (?, COALESCE(?, ''), COALESCE(?, ''))
		ON CONFLICT (user_id) DO UPDATE SET
			name=COALESCE(?, profiles.name), avatar=COALESCE(?, profiles.avatar)`,
		userID, name, avatar, name, avatar)
	if err == nil {
		s.notify("rooms", "")
	}
	return err
}

// HandleState applies a state event to the room row.
func (s *Store) HandleState(ctx context.Context, roomID id.RoomID, evtType event.Type, stateKey string, content *event.Content) error {
	_ = content.ParseRaw(evtType)
	var err error
	switch evtType {
	case event.StateRoomName:
		_, err = s.DB.ExecContext(ctx, "UPDATE rooms SET name=? WHERE room_id=?", content.AsRoomName().Name, roomID)
	case event.StateRoomAvatar:
		_, err = s.DB.ExecContext(ctx, "UPDATE rooms SET avatar=? WHERE room_id=?", string(content.AsRoomAvatar().URL), roomID)
	case event.StateTopic:
		_, err = s.DB.ExecContext(ctx, "UPDATE rooms SET topic=? WHERE room_id=?", content.AsTopic().Topic, roomID)
	case event.StateMember:
		err = s.SetMembership(ctx, roomID, id.UserID(stateKey), content.AsMember().Membership)
	case event.StateBridge, event.StateHalfShotBridge:
		_, err = s.DB.ExecContext(ctx, "UPDATE rooms SET is_portal=1 WHERE room_id=?", roomID)
	default:
		return nil
	}
	if err == nil {
		s.notify("rooms", roomID)
	}
	return err
}

// HandleEvent stores a timeline event. countUnread controls whether messages
// from other people bump the room's unread counter.
func (s *Store) HandleEvent(ctx context.Context, evt *event.Event, countUnread bool) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	if evt.Content.Parsed == nil {
		_ = evt.Content.ParseRaw(evt.Type)
	}
	var err error
	switch evt.Type {
	case event.EventRedaction:
		target := evt.Redacts
		if red, ok := evt.Content.Parsed.(*event.RedactionEventContent); ok && red.Redacts != "" {
			target = red.Redacts
		}
		if _, err = s.DB.ExecContext(ctx, "DELETE FROM reactions WHERE event_id=?", target); err == nil {
			_, err = s.DB.ExecContext(ctx, "UPDATE events SET redacted=1, content='{}' WHERE event_id=?", target)
		}
	case event.EventReaction:
		rel := evt.Content.AsReaction().RelatesTo
		_, err = s.DB.ExecContext(ctx, `
			INSERT INTO reactions (event_id, room_id, target, sender, key) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (event_id) DO NOTHING`, evt.ID, evt.RoomID, rel.EventID, evt.Sender, rel.Key)
	case event.EventMessage, event.EventSticker:
		msg := evt.Content.AsMessage()
		if msg.RelatesTo != nil && msg.RelatesTo.Type == event.RelReplace && msg.NewContent != nil {
			var newContent []byte
			if newContent, err = json.Marshal(msg.NewContent); err == nil {
				_, err = s.DB.ExecContext(ctx, "UPDATE events SET content=?, edited=1 WHERE event_id=?", newContent, msg.RelatesTo.EventID)
			}
			break
		}
		err = s.insertMessage(ctx, evt, msg, countUnread)
	default:
		return nil
	}
	if err == nil {
		s.notify("messages", evt.RoomID)
	}
	return err
}

func (s *Store) insertMessage(ctx context.Context, evt *event.Event, msg *event.MessageEventContent, countUnread bool) error {
	raw, err := json.Marshal(&evt.Content)
	if err != nil {
		return err
	}
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO events (event_id, room_id, sender, type, content, ts) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (event_id) DO NOTHING`, evt.ID, evt.RoomID, evt.Sender, evt.Type.Type, raw, evt.Timestamp)
	if err != nil {
		return err
	} else if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	unread := 0
	if countUnread && !s.isSelf(evt.Sender) {
		unread = 1
	}
	_, err = s.DB.ExecContext(ctx, `
		UPDATE rooms SET
			preview=CASE WHEN ? >= last_ts THEN ? ELSE preview END,
			last_ts=MAX(last_ts, ?),
			unread=unread + ?
		WHERE room_id=?`, evt.Timestamp, Preview(evt.Type, msg), evt.Timestamp, unread, evt.RoomID)
	if err == nil {
		s.notify("rooms", evt.RoomID)
	}
	return err
}

// Preview renders a one-line summary of a message for the chat list.
func Preview(evtType event.Type, msg *event.MessageEventContent) string {
	if evtType == event.EventSticker {
		return "Sticker"
	}
	switch msg.MsgType {
	case event.MsgImage:
		return "📷 Photo"
	case event.MsgVideo:
		return "🎥 Video"
	case event.MsgAudio:
		return "🎤 Voice message"
	case event.MsgFile:
		return "📎 " + msg.GetFileName()
	}
	body := strings.TrimSpace(msg.Body)
	if i := strings.IndexByte(body, '\n'); i >= 0 {
		body = body[:i]
	}
	return body
}

func (s *Store) SetEventStatus(ctx context.Context, eventID id.EventID, roomID id.RoomID, status string) {
	if _, err := s.DB.ExecContext(ctx, "UPDATE events SET status=? WHERE event_id=?", status, eventID); err == nil {
		s.notify("messages", roomID)
	}
}

func (s *Store) MarkRead(ctx context.Context, roomID id.RoomID) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE rooms SET unread=0 WHERE room_id=?", roomID)
	if err == nil {
		s.notify("rooms", roomID)
	}
	return err
}

func (s *Store) MarkUnread(ctx context.Context, roomID id.RoomID, unread bool) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE rooms SET unread=MAX(unread, ?) WHERE room_id=?", unread, roomID)
	if err == nil && !unread {
		return s.MarkRead(ctx, roomID)
	}
	s.notify("rooms", roomID)
	return err
}

func (s *Store) Mute(ctx context.Context, roomID id.RoomID, until time.Time) error {
	ts := int64(0)
	if !until.IsZero() {
		ts = until.UnixMilli()
	}
	_, err := s.DB.ExecContext(ctx, "UPDATE rooms SET muted_until=? WHERE room_id=?", ts, roomID)
	return err
}

func (s *Store) GetEvent(ctx context.Context, eventID id.EventID) (*event.Event, error) {
	var roomID, sender, evtType, raw string
	var ts int64
	err := s.DB.QueryRowContext(ctx, "SELECT room_id, sender, type, content, ts FROM events WHERE event_id=?", eventID).
		Scan(&roomID, &sender, &evtType, &raw, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("event %s not found", eventID)
	} else if err != nil {
		return nil, err
	}
	evt := &event.Event{
		ID: eventID, RoomID: id.RoomID(roomID), Sender: id.UserID(sender), Timestamp: ts,
		Type: event.Type{Type: evtType, Class: event.MessageEventType},
	}
	if err = json.Unmarshal([]byte(raw), &evt.Content); err != nil {
		return nil, err
	}
	_ = evt.Content.ParseRaw(evt.Type)
	return evt, nil
}

func (s *Store) SaveMedia(ctx context.Context, mxc id.ContentURIString, path, mime string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO media (mxc, path, mime) VALUES (?, ?, ?)
		ON CONFLICT (mxc) DO UPDATE SET path=excluded.path, mime=excluded.mime`, mxc, path, mime)
	return err
}

func (s *Store) MediaPath(ctx context.Context, mxc id.ContentURIString) (path, mime string, err error) {
	err = s.DB.QueryRowContext(ctx, "SELECT path, mime FROM media WHERE mxc=?", mxc).Scan(&path, &mime)
	return
}
