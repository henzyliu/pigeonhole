package inbox

import (
	"context"
	"time"

	"maunium.net/go/mautrix/id"
)

// Notification is a new-message alert for the desktop.
type Notification struct {
	RoomID string
	Title  string // chat name
	Sender string // set for group chats
	Body   string
}

// maxNotifyAge stops a burst of alerts for old messages that arrive while
// catching up after the app was closed.
const maxNotifyAge = 2 * time.Minute

func (s *Service) onIncoming(roomID id.RoomID, sender id.UserID, ts int64, preview string) {
	if s.OnNotify == nil || time.Since(time.UnixMilli(ts)) > maxNotifyAge {
		return
	}
	ctx := context.Background()
	var name string
	var isDM bool
	var mutedUntil int64
	err := s.Store.DB.QueryRowContext(ctx, "SELECT name, is_dm, muted_until FROM rooms WHERE room_id=? AND is_portal=1", roomID).
		Scan(&name, &isDM, &mutedUntil)
	if err != nil || mutedUntil > time.Now().UnixMilli() {
		return
	}
	var senderName string
	_ = s.Store.DB.QueryRowContext(ctx, "SELECT name FROM profiles WHERE user_id=?", sender).Scan(&senderName)
	n := Notification{RoomID: string(roomID), Title: name, Body: preview}
	if isDM || name == "" {
		n.Title = senderName
	} else {
		n.Sender = senderName
	}
	s.OnNotify(n)
}
