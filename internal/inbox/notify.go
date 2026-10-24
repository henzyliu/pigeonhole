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
	// Network and LoginID say which account the chat belongs to, so alerts
	// can be silenced per account. LoginID is empty if it can't be told.
	Network string
	LoginID string
}

// maxNotifyAge stops a burst of alerts for old messages that arrive while
// catching up after the app was closed.
const maxNotifyAge = 2 * time.Minute

func (s *Service) onIncoming(roomID id.RoomID, sender id.UserID, ts int64, preview string) {
	if s.OnNotify == nil {
		return
	} else if time.Since(time.UnixMilli(ts)) > maxNotifyAge {
		s.Log.Debug().Str("room_id", string(roomID)).Msg("Not notifying: message is old (catching up)")
		return
	}
	ctx := context.Background()
	var name, network string
	var isDM bool
	var mutedUntil int64
	err := s.Store.DB.QueryRowContext(ctx, "SELECT name, network, is_dm, muted_until FROM rooms WHERE room_id=? AND is_portal=1", roomID).
		Scan(&name, &network, &isDM, &mutedUntil)
	if err != nil {
		return
	} else if mutedUntil > time.Now().UnixMilli() {
		s.Log.Debug().Str("room_id", string(roomID)).Msg("Not notifying: chat is muted")
		return
	}
	// Like the official apps, message requests arrive silently.
	for netID := range s.networks {
		if portal := s.portalFor(ctx, netID, string(roomID)); portal != nil && portal.MessageRequest {
			s.Log.Debug().Str("room_id", string(roomID)).Msg("Not notifying: message request")
			return
		}
	}
	var senderName string
	_ = s.Store.DB.QueryRowContext(ctx, "SELECT name FROM profiles WHERE user_id=?", sender).Scan(&senderName)
	n := Notification{RoomID: string(roomID), Title: name, Body: preview, Network: network}
	if net, ok := s.networks[network]; ok {
		if login := s.loginForRoom(ctx, net, string(roomID)); login != nil {
			n.LoginID = string(login.ID)
		}
	}
	if isDM || name == "" {
		n.Title = senderName
	} else {
		n.Sender = senderName
	}
	s.OnNotify(n)
}
