package inbox

import (
	"context"
	"encoding/json"
	"maps"

	"github.com/bwmarrin/discordgo"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"unified-inbox/internal/localmx"
)

// Discord tells each account its friends' status (online, idle, do not
// disturb), which the bridge ignores; Pigeonhole follows it from the raw
// gateway events to show a dot on Discord DMs, and how many friends are
// online in a group. Only friends' statuses are sent, so anyone else has
// none.

// onDiscordEvent sees every Discord gateway event for an account. It runs on
// the gateway's goroutine, so it only reads the two events it needs.
func (s *Service) onDiscordEvent(login *bridgev2.UserLogin, event any) {
	e, ok := event.(*discordgo.Event)
	if !ok {
		return
	}
	switch e.Type {
	case "READY_SUPPLEMENTAL":
		// Every friend's status, on connecting.
		var data struct {
			MergedPresences struct {
				Friends []struct {
					UserID string `json:"user_id"`
					Status string `json:"status"`
				} `json:"friends"`
			} `json:"merged_presences"`
		}
		if json.Unmarshal(e.RawData, &data) != nil {
			return
		}
		statuses := make(map[string]string, len(data.MergedPresences.Friends))
		for _, f := range data.MergedPresences.Friends {
			statuses[f.UserID] = f.Status
		}
		s.setPresence(string(login.ID), statuses, true)
	case "PRESENCE_UPDATE":
		// A friend's status changing. Updates from servers carry a guild ID
		// and are left alone.
		var data struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
			Status  string `json:"status"`
			GuildID string `json:"guild_id"`
		}
		if json.Unmarshal(e.RawData, &data) != nil || data.GuildID != "" || data.User.ID == "" {
			return
		}
		s.setPresence(string(login.ID), map[string]string{data.User.ID: data.Status}, false)
	}
}

// setPresence records statuses for an account's friends, replacing all of
// them if replace is set, and tells the UI if anything changed.
func (s *Service) setPresence(loginID string, statuses map[string]string, replace bool) {
	s.presenceLock.Lock()
	if s.presence == nil {
		s.presence = make(map[string]map[string]string)
	}
	known := s.presence[loginID]
	changed := false
	if replace {
		changed = !maps.Equal(known, statuses)
		s.presence[loginID] = statuses
	} else {
		if known == nil {
			known = make(map[string]string)
			s.presence[loginID] = known
		}
		for userID, st := range statuses {
			if known[userID] != st {
				known[userID] = st
				changed = true
			}
		}
	}
	s.presenceLock.Unlock()
	// Activity updates (a new song playing, say) arrive as presence updates
	// too, so only a change of status refreshes the UI.
	if changed {
		s.emit("inbox:change", localmx.Change{Kind: "presence"})
	}
}

// PresenceInfo is what the UI shows of Discord statuses.
type PresenceInfo struct {
	// DMs gives the other person's status ("online", "idle" or "dnd") in
	// each Discord DM, by room ID.
	DMs map[string]string `json:"dms"`
	// Groups counts the friends online (including idle and do not disturb)
	// in each Discord group chat, by room ID.
	Groups map[string]int `json:"groups"`
}

// Presence gives the Discord statuses to show. Offline people, and chats on
// accounts that aren't connected right now (whose statuses may be out of
// date), are left out.
func (s *Service) Presence(ctx context.Context) (*PresenceInfo, error) {
	out := &PresenceInfo{DMs: map[string]string{}, Groups: map[string]int{}}
	n, ok := s.networks["discord"]
	if !ok || !n.started.Load() {
		return out, nil
	}
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT room_id, is_dm FROM rooms WHERE is_portal=1 AND network='discord'")
	if err != nil {
		return nil, err
	}
	type room struct {
		id   string
		isDM bool
	}
	var rooms []room
	for rows.Next() {
		var r room
		if err = rows.Scan(&r.id, &r.isDM); err != nil {
			rows.Close()
			return nil, err
		}
		rooms = append(rooms, r)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}

	statuses := s.connectedPresence()
	for _, r := range rooms {
		portal, err := n.Bridge.GetPortalByMXID(ctx, id.RoomID(r.id))
		if err != nil || portal == nil {
			continue
		}
		if r.isDM {
			if st := statuses.of(string(portal.Receiver), string(portal.OtherUserID)); st != "" {
				out.DMs[r.id] = st
			}
			continue
		}
		members, err := s.Store.Members(ctx, id.RoomID(r.id))
		if err != nil {
			return nil, err
		}
		for userID, membership := range members {
			if membership != event.MembershipJoin {
				continue
			}
			if ghost, ok := n.MX.ParseGhostMXID(userID); ok && statuses.of(string(portal.Receiver), string(ghost)) != "" {
				out.Groups[r.id]++
			}
		}
	}
	return out, nil
}

// memberPresence gives the status of each person in a Discord chat, by
// their user ID in the store, for the @mention suggestions.
func (s *Service) memberPresence(ctx context.Context, roomID string) map[id.UserID]string {
	n, ok := s.networks["discord"]
	if !ok || !n.started.Load() {
		return nil
	}
	portal, err := n.Bridge.GetPortalByMXID(ctx, id.RoomID(roomID))
	if err != nil || portal == nil {
		return nil
	}
	members, err := s.Store.Members(ctx, id.RoomID(roomID))
	if err != nil {
		return nil
	}
	statuses := s.connectedPresence()
	out := map[id.UserID]string{}
	for userID := range members {
		if ghost, ok := n.MX.ParseGhostMXID(userID); ok {
			if st := statuses.of(string(portal.Receiver), string(ghost)); st != "" {
				out[userID] = st
			}
		}
	}
	return out
}

// presenceByLogin is friends' statuses, by account and then user.
type presenceByLogin map[string]map[string]string

// connectedPresence copies the statuses known for connected accounts.
func (s *Service) connectedPresence() presenceByLogin {
	s.lock.Lock()
	connected := map[string]bool{}
	for key, st := range s.states {
		connected[key] = st.StateEvent == status.StateConnected
	}
	s.lock.Unlock()
	s.presenceLock.Lock()
	defer s.presenceLock.Unlock()
	out := presenceByLogin{}
	for loginID, statuses := range s.presence {
		if connected["discord/"+loginID] {
			out[loginID] = maps.Clone(statuses)
		}
	}
	return out
}

// of gives a user's status as one of the shown ones ("online", "idle",
// "dnd"), or "" when offline or unknown. Group chats aren't tied to one
// account (loginID is empty), so any account that's friends with them will
// do.
func (p presenceByLogin) of(loginID, userID string) string {
	shown := func(st string) bool { return st == "online" || st == "idle" || st == "dnd" }
	if loginID != "" {
		if st := p[loginID][userID]; shown(st) {
			return st
		}
		return ""
	}
	for _, statuses := range p {
		if st := statuses[userID]; shown(st) {
			return st
		}
	}
	return ""
}
