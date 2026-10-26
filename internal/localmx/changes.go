package localmx

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Edits and unsends made in Pigeonhole wait for the network to confirm
// them before the stored message changes. Until then the change is kept
// beside the message (pending_*), so if the network refuses it, or never
// answers, the message is still exactly what everyone else sees.

// Kinds of change, as stored in pending_kind and change_failed.
const (
	ChangeEdit   = "edit"
	ChangeUnsend = "unsend"
)

// ErrChangePending is returned when a message already has a change waiting.
var ErrChangePending = errors.New("that message is still being changed")

// StartChange records an edit (with its new content) or unsend of target,
// sent as the event changeID, until the network confirms or refuses it.
func (s *Store) StartChange(ctx context.Context, roomID id.RoomID, target id.EventID, kind string, changeID id.EventID, content []byte) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE events SET pending_kind=?, pending_id=?, pending_content=?, change_failed=''
		WHERE event_id=? AND room_id=? AND pending_kind='' AND redacted=0`,
		kind, changeID, string(content), target, roomID)
	if err != nil {
		return err
	} else if n, _ := res.RowsAffected(); n == 0 {
		return ErrChangePending
	}
	s.notify("messages", roomID)
	return nil
}

// FailChange gives up on a change that's still waiting, as if the network
// had refused it. It does nothing once the change has been settled.
func (s *Store) FailChange(ctx context.Context, changeID id.EventID) {
	s.finishChange(ctx, changeID, "fail")
}

// finishChange settles a pending change when the bridge reports on its
// event: success applies it to the message, failure drops it and notes
// that it failed. It reports whether changeID was a pending change.
// (Changes aren't stored as events, so any later status for one matches
// nothing.)
func (s *Store) finishChange(ctx context.Context, changeID id.EventID, status string) bool {
	if status == "pending" || changeID == "" {
		return false
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	var target id.EventID
	var roomID id.RoomID
	var kind, content string
	err := s.DB.QueryRowContext(ctx, "SELECT event_id, room_id, pending_kind, pending_content FROM events WHERE pending_id=?",
		changeID).Scan(&target, &roomID, &kind, &content)
	if err != nil {
		return false
	}
	if status == StatusSuccess || status == StatusDelivered {
		switch kind {
		case ChangeEdit:
			err = s.applyEdit(ctx, target, []byte(content), true)
		case ChangeUnsend:
			err = s.applyRedaction(ctx, target)
		}
		if err == nil {
			_, err = s.DB.ExecContext(ctx, "UPDATE events SET pending_kind='', pending_id='', pending_content='' WHERE event_id=?", target)
		}
	} else {
		_, err = s.DB.ExecContext(ctx, `
			UPDATE events SET change_failed=pending_kind, pending_kind='', pending_id='', pending_content=''
			WHERE event_id=?`, target)
	}
	if err == nil {
		s.notify("messages", roomID)
	}
	return true
}

func (s *Store) applyEdit(ctx context.Context, target id.EventID, content []byte, shown bool) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE events SET content=?, edited=MAX(edited, ?) WHERE event_id=?", content, shown, target)
	if err == nil {
		err = refreshPreview(ctx, s.DB, target)
	}
	return err
}

func (s *Store) applyRedaction(ctx context.Context, target id.EventID) error {
	_, err := s.DB.ExecContext(ctx, "DELETE FROM reactions WHERE event_id=?", target)
	if err == nil {
		_, err = s.DB.ExecContext(ctx, `
			UPDATE events SET redacted=1, content='{}', pending_kind='', pending_id='', pending_content=''
			WHERE event_id=?`, target)
	}
	if err == nil {
		err = refreshPreview(ctx, s.DB, target)
	}
	return err
}

// failLeftoverChanges marks changes still waiting from the last run as
// failed, since the bridges' send queues don't survive a restart.
func failLeftoverChanges(db *sql.DB) error {
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS events_pending ON events (pending_id) WHERE pending_id<>''"); err != nil {
		return err
	}
	// Failures are shown until the next restart, then forgotten.
	if _, err := db.Exec("UPDATE events SET change_failed='' WHERE change_failed<>''"); err != nil {
		return err
	}
	_, err := db.Exec(`
		UPDATE events SET change_failed=pending_kind, pending_kind='', pending_id='', pending_content=''
		WHERE pending_kind<>''`)
	return err
}

// editedContent is an edit's new content as stored. Bridges can put fields
// the parsed content doesn't know about (such as Discord's link previews)
// in the raw m.new_content, so those are kept too.
func editedContent(content event.Content, parsed *event.MessageEventContent) ([]byte, error) {
	raw, err := json.Marshal(parsed)
	if err != nil {
		return nil, err
	}
	extra, ok := content.Raw["m.new_content"].(map[string]any)
	if !ok || len(extra) == 0 {
		return raw, nil
	}
	merged := make(map[string]any)
	if err = json.Unmarshal(raw, &merged); err != nil {
		return nil, err
	}
	for key, value := range extra {
		if _, set := merged[key]; !set {
			merged[key] = value
		}
	}
	return json.Marshal(merged)
}

// textChanged reports whether an edit changes a message's text, as opposed
// to only filling in something else, like a link preview.
func (s *Store) textChanged(ctx context.Context, target id.EventID, newBody string) bool {
	var raw string
	if err := s.DB.QueryRowContext(ctx, "SELECT content FROM events WHERE event_id=?", target).Scan(&raw); err != nil {
		return true
	}
	var old struct {
		Body string `json:"body"`
	}
	_ = json.Unmarshal([]byte(raw), &old)
	return strings.TrimSpace(old.Body) != strings.TrimSpace(newBody)
}
