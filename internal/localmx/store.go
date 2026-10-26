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
	"regexp"
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
	muted_until INTEGER NOT NULL DEFAULT 0,
	-- hidden_ts is last_ts when the user hid the chat (0 = not hidden).
	hidden_ts   INTEGER NOT NULL DEFAULT 0,
	-- local_muted is a mute set in Pigeonhole, kept apart from muted_until,
	-- which bridges overwrite with the network's own mute state.
	local_muted INTEGER NOT NULL DEFAULT 0,
	-- draft is an unsent message typed in Pigeonhole, and draft_reply the
	-- event it replies to, if any.
	draft       TEXT NOT NULL DEFAULT '',
	draft_reply TEXT NOT NULL DEFAULT ''
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
-- Where each other person has read up to in a room (one row per reader).
CREATE TABLE IF NOT EXISTS receipts (
	room_id  TEXT NOT NULL,
	user_id  TEXT NOT NULL,
	event_id TEXT NOT NULL,
	ts       INTEGER NOT NULL,
	PRIMARY KEY (room_id, user_id)
);
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
	// OnIncoming is called for each new message from someone else that counts
	// towards the unread total (i.e. not history backfill).
	OnIncoming func(roomID id.RoomID, sender id.UserID, ts int64, preview string)

	lock sync.Mutex

	typingLock sync.Mutex
	typing     map[id.RoomID]map[id.UserID]time.Time
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
	if err = addColumn(db, "rooms", "hidden_ts", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return nil, fmt.Errorf("failed to add hidden chats: %w", err)
	}
	if err = addColumn(db, "rooms", "local_muted", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return nil, fmt.Errorf("failed to add muted chats: %w", err)
	}
	for _, col := range []string{"draft", "draft_reply"} {
		if err = addColumn(db, "rooms", col, "TEXT NOT NULL DEFAULT ''"); err != nil {
			return nil, fmt.Errorf("failed to add drafts: %w", err)
		}
	}
	if err = refreshEditedPreviews(db); err != nil {
		return nil, fmt.Errorf("failed to fix chat previews: %w", err)
	}
	if err = relativizeMediaPaths(db); err != nil {
		return nil, fmt.Errorf("failed to fix media paths: %w", err)
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

// mediaURIPattern finds this store's media URIs inside stored event JSON.
var mediaURIPattern = regexp.MustCompile(`mxc://` + ServerName + `/[A-Za-z0-9_-]+`)

// DeleteRoom removes a chat and everything stored for it: messages,
// reactions, receipts, and media files no longer used anywhere else.
func (s *Store) DeleteRoom(ctx context.Context, roomID id.RoomID) error {
	candidates, err := s.roomMedia(ctx, roomID)
	if err != nil {
		return err
	}
	for _, q := range []string{
		"DELETE FROM rooms WHERE room_id=?", "DELETE FROM members WHERE room_id=?",
		"DELETE FROM events WHERE room_id=?", "DELETE FROM reactions WHERE room_id=?",
		"DELETE FROM receipts WHERE room_id=?",
	} {
		if _, err = s.DB.ExecContext(ctx, q, roomID); err != nil {
			return err
		}
	}
	// Only media this room referenced is considered, so files that are still
	// being uploaded for other chats (not referenced by anything yet) are safe.
	for mxc := range candidates {
		if err = s.deleteMediaIfUnused(ctx, mxc); err != nil {
			return err
		}
	}
	s.notify("rooms", roomID)
	return nil
}

// roomMedia lists the media URIs a room's avatar and messages refer to.
func (s *Store) roomMedia(ctx context.Context, roomID id.RoomID) (map[string]struct{}, error) {
	found := make(map[string]struct{})
	rows, err := s.DB.QueryContext(ctx, `
		SELECT content FROM events WHERE room_id=?
		UNION ALL SELECT avatar FROM rooms WHERE room_id=?`, roomID, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var text string
		if err = rows.Scan(&text); err != nil {
			return nil, err
		}
		for _, mxc := range mediaURIPattern.FindAllString(text, -1) {
			found[mxc] = struct{}{}
		}
	}
	return found, rows.Err()
}

func (s *Store) deleteMediaIfUnused(ctx context.Context, mxc string) error {
	var used bool
	err := s.DB.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM rooms WHERE avatar=?)
		    OR EXISTS (SELECT 1 FROM profiles WHERE avatar=?)
		    OR EXISTS (SELECT 1 FROM events WHERE instr(content, ?) > 0)`, mxc, mxc, mxc).Scan(&used)
	if err != nil || used {
		return err
	}
	path, _, err := s.MediaPath(ctx, id.ContentURIString(mxc))
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err = s.DB.ExecContext(ctx, "DELETE FROM media WHERE mxc=?", mxc)
	return err
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
		if err == nil {
			err = refreshPreview(ctx, s.DB, target)
		}
	case event.EventReaction:
		rel := evt.Content.AsReaction().RelatesTo
		_, err = s.DB.ExecContext(ctx, `
			INSERT INTO reactions (event_id, room_id, target, sender, key) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (event_id) DO NOTHING`, evt.ID, evt.RoomID, rel.EventID, evt.Sender, rel.Key)
	case event.EventMessage, event.EventSticker:
		msg := evt.Content.AsMessage()
		if msg.RelatesTo != nil && msg.RelatesTo.Type == event.RelReplace && msg.NewContent != nil {
			// Bridges mark edits that only fill in content (e.g. a reel fetched
			// to replace its thumbnail) as not to be shown as edited.
			hidden, _ := evt.Content.Raw["com.beeper.dont_render_edited"].(bool)
			var newContent []byte
			if newContent, err = json.Marshal(msg.NewContent); err == nil {
				_, err = s.DB.ExecContext(ctx, "UPDATE events SET content=?, edited=MAX(edited, ?) WHERE event_id=?",
					newContent, !hidden, msg.RelatesTo.EventID)
			}
			if err == nil {
				err = refreshPreview(ctx, s.DB, msg.RelatesTo.EventID)
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
	// Someone who has just sent a message has stopped typing it.
	s.SetTyping(evt.RoomID, evt.Sender, 0)
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
	if err != nil {
		return err
	}
	s.notify("rooms", evt.RoomID)
	if unread > 0 && s.OnIncoming != nil {
		go s.OnIncoming(evt.RoomID, evt.Sender, evt.Timestamp, Preview(evt.Type, msg))
	}
	return nil
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

// DeletedPreview is the chat-list preview for a deleted latest message.
const DeletedPreview = "Message deleted"

// refreshPreview redoes a chat's preview from an edited or deleted message,
// if it's the chat's latest one; the preview is otherwise only set when a
// message arrives.
func refreshPreview(ctx context.Context, db dbExec, eventID id.EventID) error {
	var roomID, evtType, raw string
	var redacted bool
	err := db.QueryRowContext(ctx, `
		SELECT e.room_id, e.type, e.content, e.redacted
		FROM events e JOIN rooms r ON r.room_id = e.room_id AND r.last_ts = e.ts
		WHERE e.event_id = ?`, eventID).Scan(&roomID, &evtType, &raw, &redacted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // not the latest message, or not stored
	} else if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "UPDATE rooms SET preview=? WHERE room_id=?", storedPreview(evtType, raw, redacted), roomID)
	return err
}

// storedPreview is Preview for a message as stored in the events table.
func storedPreview(evtType, raw string, redacted bool) string {
	if redacted {
		return DeletedPreview
	}
	var content event.Content
	if json.Unmarshal([]byte(raw), &content) != nil {
		return ""
	}
	t := event.Type{Type: evtType, Class: event.MessageEventType}
	_ = content.ParseRaw(t)
	return Preview(t, content.AsMessage())
}

type dbExec interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// refreshEditedPreviews fixes previews saved before edits and deletions
// updated them: chats whose latest message was edited or deleted since.
func refreshEditedPreviews(db *sql.DB) error {
	rows, err := db.Query(`
		SELECT e.event_id FROM rooms r JOIN events e ON e.room_id = r.room_id AND e.ts = r.last_ts
		WHERE e.edited = 1 OR e.redacted = 1`)
	if err != nil {
		return err
	}
	var ids []id.EventID
	for rows.Next() {
		var eventID id.EventID
		if err = rows.Scan(&eventID); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, eventID)
	}
	rows.Close()
	for _, eventID := range ids {
		if err = refreshPreview(context.Background(), db, eventID); err != nil {
			return err
		}
	}
	return rows.Err()
}

// Message statuses, as stored in events.status. "pending", "fail" and
// "retriable" come straight from the bridge's message status too.
const (
	StatusSuccess   = "success"
	StatusDelivered = "delivered"
)

// SetEventStatus records a send status. A late "success" never downgrades a
// message that's already been delivered.
func (s *Store) SetEventStatus(ctx context.Context, eventID id.EventID, roomID id.RoomID, status string) {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE events SET status=? WHERE event_id=? AND NOT (status=? AND ?=?)`,
		status, eventID, StatusDelivered, status, StatusSuccess)
	if err == nil {
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

// SetReceipt records that another person has read up to eventID.
func (s *Store) SetReceipt(ctx context.Context, roomID id.RoomID, userID id.UserID, eventID id.EventID, ts time.Time) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO receipts (room_id, user_id, event_id, ts) VALUES (?, ?, ?, ?)
		ON CONFLICT (room_id, user_id) DO UPDATE SET event_id=excluded.event_id, ts=excluded.ts`,
		roomID, userID, eventID, ts.UnixMilli())
	if err == nil {
		s.notify("messages", roomID)
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

// SetHidden hides a chat from the main list, or brings it back. A hidden
// chat only stays hidden until a newer message arrives (see HiddenClause).
func (s *Store) SetHidden(ctx context.Context, roomID id.RoomID, hidden bool) error {
	q := "UPDATE rooms SET hidden_ts=0 WHERE room_id=?"
	if hidden {
		// MAX(…, 1) so a chat with no messages yet still counts as hidden.
		q = "UPDATE rooms SET hidden_ts=MAX(last_ts, 1) WHERE room_id=?"
	}
	_, err := s.DB.ExecContext(ctx, q, roomID)
	if err == nil {
		s.notify("rooms", roomID)
	}
	return err
}

// HiddenClause is the SQL condition for a hidden chat: hidden, and nothing
// newer than when it was hidden. A new message raises last_ts past
// hidden_ts, which brings the chat back without any extra bookkeeping.
const HiddenClause = "(hidden_ts > 0 AND last_ts <= hidden_ts)"

// SetDraft saves a chat's unsent message (and the event it replies to);
// empty text clears it. Only an actual change tells the UI.
func (s *Store) SetDraft(ctx context.Context, roomID id.RoomID, text string, replyTo id.EventID) error {
	if text == "" {
		replyTo = ""
	}
	res, err := s.DB.ExecContext(ctx, "UPDATE rooms SET draft=?, draft_reply=? WHERE room_id=? AND (draft<>? OR draft_reply<>?)",
		text, replyTo, roomID, text, replyTo)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		s.notify("rooms", roomID)
	}
	return nil
}

// SetLocalMuted mutes a chat in Pigeonhole only, or unmutes it.
func (s *Store) SetLocalMuted(ctx context.Context, roomID id.RoomID, muted bool) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE rooms SET local_muted=? WHERE room_id=?", muted, roomID)
	if err == nil {
		s.notify("rooms", roomID)
	}
	return err
}

// NetworkMutedClause is the SQL condition for a chat muted in the
// network's own app (muted_until is in milliseconds).
const NetworkMutedClause = "(muted_until > CAST(strftime('%s', 'now') AS INTEGER) * 1000)"

// MutedClause is the SQL condition for a chat that's muted either way:
// it doesn't alert or count toward the app's unread badge.
const MutedClause = "(local_muted = 1 OR " + NetworkMutedClause + ")"

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

// SaveMedia records a file in MediaDir. Only the file name is stored, so the
// data folder can be moved (as when the app was renamed) without breaking it.
func (s *Store) SaveMedia(ctx context.Context, mxc id.ContentURIString, path, mime string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO media (mxc, path, mime) VALUES (?, ?, ?)
		ON CONFLICT (mxc) DO UPDATE SET path=excluded.path, mime=excluded.mime`, mxc, filepath.Base(path), mime)
	return err
}

func (s *Store) MediaPath(ctx context.Context, mxc id.ContentURIString) (path, mime string, err error) {
	var name string
	err = s.DB.QueryRowContext(ctx, "SELECT path, mime FROM media WHERE mxc=?", mxc).Scan(&name, &mime)
	return filepath.Join(s.MediaDir, filepath.Base(name)), mime, err
}

// addColumn adds a column to a table created by an older version of the app.
func addColumn(db *sql.DB, table, column, definition string) error {
	rows, err := db.Query("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return err
		} else if name == column {
			return nil
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	_, err = db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err
}

// relativizeMediaPaths rewrites media rows saved with absolute paths (before
// paths were stored relative) to bare file names.
func relativizeMediaPaths(db *sql.DB) error {
	rows, err := db.Query("SELECT mxc, path FROM media WHERE path LIKE '%/%' OR path LIKE '%\\%'")
	if err != nil {
		return err
	}
	fixes := map[string]string{}
	for rows.Next() {
		var mxc, path string
		if err = rows.Scan(&mxc, &path); err != nil {
			rows.Close()
			return err
		}
		fixes[mxc] = filepath.Base(path)
	}
	rows.Close()
	for mxc, name := range fixes {
		if _, err = db.Exec("UPDATE media SET path=? WHERE mxc=?", name, mxc); err != nil {
			return err
		}
	}
	return nil
}
