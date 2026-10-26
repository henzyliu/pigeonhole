package localmx

import (
	"slices"
	"time"

	"maunium.net/go/mautrix/id"
)

// maxTyping caps how long someone shows as typing without the network
// saying so again, in case a stop is never sent.
const maxTyping = 30 * time.Second

// SetTyping records that someone is typing in a chat for the next timeout,
// or has stopped (timeout 0). It's kept in memory only, and the UI is told
// when it starts, stops or runs out.
func (s *Store) SetTyping(roomID id.RoomID, userID id.UserID, timeout time.Duration) {
	if s.isSelf(userID) {
		return
	}
	timeout = min(timeout, maxTyping)
	s.typingLock.Lock()
	room := s.typing[roomID]
	_, was := room[userID]
	if timeout > 0 {
		if s.typing == nil {
			s.typing = make(map[id.RoomID]map[id.UserID]time.Time)
		}
		if room == nil {
			room = make(map[id.UserID]time.Time)
			s.typing[roomID] = room
		}
		until := time.Now().Add(timeout)
		room[userID] = until
		time.AfterFunc(timeout, func() { s.expireTyping(roomID, userID, until) })
	} else {
		delete(room, userID)
	}
	s.typingLock.Unlock()
	if was || timeout > 0 {
		s.notify("typing", roomID)
	}
}

func (s *Store) expireTyping(roomID id.RoomID, userID id.UserID, until time.Time) {
	s.typingLock.Lock()
	expired := s.typing[roomID][userID].Equal(until)
	if expired {
		delete(s.typing[roomID], userID)
	}
	s.typingLock.Unlock()
	if expired {
		s.notify("typing", roomID)
	}
}

// Typing lists who is typing in each chat right now, in a steady order.
func (s *Store) Typing() map[id.RoomID][]id.UserID {
	s.typingLock.Lock()
	defer s.typingLock.Unlock()
	out := make(map[id.RoomID][]id.UserID)
	now := time.Now()
	for roomID, room := range s.typing {
		for userID, until := range room {
			if until.After(now) {
				out[roomID] = append(out[roomID], userID)
			}
		}
		slices.Sort(out[roomID])
	}
	return out
}
