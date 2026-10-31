package inbox

import (
	"context"
	"time"

	whatsapp "go.mau.fi/mautrix-whatsapp/pkg/connector"
	"go.mau.fi/mautrix-whatsapp/pkg/waid"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/id"

	"unified-inbox/internal/localmx"
)

// WhatsApp shows whether the person in a chat is online, or when they were
// last seen, while you have that chat open, and you show as online to
// others while you do. Pigeonhole does the same: opening a WhatsApp DM
// tells the bridge you're viewing it (which marks you online, as the
// WhatsApp app does) and asks WhatsApp for that person's presence. People
// who hide their last seen, or if you hide yours, only ever show "online".

// Seen is a WhatsApp contact's presence.
type Seen struct {
	Online bool `json:"online"`
	// LastSeen is when they were last online (Unix ms), or 0 if hidden.
	LastSeen int64 `json:"lastSeen"`
}

// ViewingChat is called with the chat on screen, or "" when none is (the
// window is in the background, say). WhatsApp then shows you as online or
// not, and the open chat's contact's presence starts arriving.
func (s *Service) ViewingChat(ctx context.Context, roomID string) {
	n, ok := s.networks["whatsapp"]
	if !ok || !n.started.Load() {
		return
	}
	user, err := n.Bridge.GetUserByMXID(ctx, Me)
	if err != nil || user == nil {
		return
	}
	var portal *bridgev2.Portal
	if roomID != "" {
		portal, _ = n.Bridge.GetPortalByMXID(ctx, id.RoomID(roomID))
	}
	for _, login := range user.GetUserLogins() {
		wa, ok := login.Client.(*whatsapp.WhatsAppClient)
		if !ok || wa.Client == nil || !wa.IsLoggedIn() {
			continue
		}
		viewing := portal
		if viewing != nil && viewing.Receiver != "" && viewing.Receiver != login.ID {
			viewing = nil
		}
		if err = wa.HandleMatrixViewingChat(ctx, &bridgev2.MatrixViewingChat{Portal: viewing}); err != nil {
			s.Log.Debug().Err(err).Msg("Failed to tell WhatsApp which chat is open")
		}
		if viewing == nil || viewing.OtherUserID == "" {
			continue
		}
		s.watchWhatsAppPresence(wa.Client)
		contact := waid.ParseUserID(viewing.OtherUserID)
		s.presenceLock.Lock()
		if s.waRooms == nil {
			s.waRooms = make(map[string]string)
		}
		s.waRooms[roomID] = contact.ToNonAD().String()
		s.presenceLock.Unlock()
		go func(cli *whatsmeow.Client) {
			if err := cli.SubscribePresence(s.ctx, contact); err != nil {
				s.Log.Debug().Err(err).Msg("Failed to subscribe to WhatsApp presence")
			}
		}(wa.Client)
	}
}

// watchWhatsAppPresence adds a presence handler to a WhatsApp connection,
// once per connection.
func (s *Service) watchWhatsAppPresence(cli *whatsmeow.Client) {
	s.presenceLock.Lock()
	defer s.presenceLock.Unlock()
	if s.waWatched == nil {
		s.waWatched = make(map[*whatsmeow.Client]bool)
	}
	if s.waWatched[cli] {
		return
	}
	s.waWatched[cli] = true
	cli.AddEventHandler(func(evt any) {
		if p, ok := evt.(*events.Presence); ok {
			s.onWhatsAppPresence(cli, p)
		}
	})
}

// onWhatsAppPresence records a contact's presence. WhatsApp may send it
// under their phone number or their hidden ID ("LID"), and chats may use
// either, so it's kept under both.
func (s *Service) onWhatsAppPresence(cli *whatsmeow.Client, p *events.Presence) {
	seen := Seen{Online: !p.Unavailable}
	if !p.LastSeen.IsZero() {
		seen.LastSeen = p.LastSeen.UnixMilli()
	}
	from := p.From.ToNonAD()
	keys := []string{from.String()}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	var alt types.JID
	var err error
	if from.Server == types.HiddenUserServer {
		alt, err = cli.Store.LIDs.GetPNForLID(ctx, from)
	} else {
		alt, err = cli.Store.LIDs.GetLIDForPN(ctx, from)
	}
	if err == nil && !alt.IsEmpty() {
		keys = append(keys, alt.ToNonAD().String())
	}
	s.presenceLock.Lock()
	if s.waSeen == nil {
		s.waSeen = make(map[string]Seen)
	}
	changed := false
	for _, key := range keys {
		// Going offline without a time (hidden last seen) keeps an earlier
		// one rather than forgetting it.
		if !seen.Online && seen.LastSeen == 0 {
			seen.LastSeen = s.waSeen[key].LastSeen
		}
		if s.waSeen[key] != seen {
			s.waSeen[key] = seen
			changed = true
		}
	}
	s.presenceLock.Unlock()
	if changed {
		s.emit("inbox:change", localmx.Change{Kind: "presence"})
	}
}

// whatsAppSeen gives the presence known for WhatsApp chats that have been
// opened, by room ID.
func (s *Service) whatsAppSeen() map[string]Seen {
	s.presenceLock.Lock()
	defer s.presenceLock.Unlock()
	out := map[string]Seen{}
	for roomID, contact := range s.waRooms {
		if seen, ok := s.waSeen[contact]; ok && (seen.Online || seen.LastSeen > 0) {
			out[roomID] = seen
		}
	}
	return out
}
