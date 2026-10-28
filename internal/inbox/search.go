package inbox

import (
	"context"
	"database/sql"
	"strings"

	"maunium.net/go/mautrix/id"
)

// Message search looks through the text of every stored message (and
// photo and video captions), newest first. Everything is on this computer,
// so it's a straight scan: about 30 ms for 35,000 messages, which is fine
// while typing.

// SearchPageSize is how many results a search returns at once.
const SearchPageSize = 40

// SearchResult is a message matching a search, with its chat.
type SearchResult struct {
	RoomID     string `json:"roomId"`
	EventID    string `json:"eventId"`
	ChatName   string `json:"chatName"`
	ChatAvatar string `json:"chatAvatar"`
	Network    string `json:"network"`
	IsDM       bool   `json:"isDm"`
	SenderName string `json:"senderName"`
	IsMe       bool   `json:"isMe"`
	TS         int64  `json:"ts"`
	Body       string `json:"body"`
}

// SearchMessages finds messages containing query (ignoring case, for
// Latin letters), newest first, skipping offset results.
func (s *Service) SearchMessages(ctx context.Context, query string, offset int) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 {
		return []SearchResult{}, nil
	}
	rows, err := s.searchRows(ctx, query, "", SearchPageSize, offset)
	if err != nil {
		return nil, err
	}
	var out []SearchResult
	for rows.Next() {
		var r SearchResult
		var sender string
		if err = rows.Scan(&r.EventID, &r.RoomID, &sender, &r.TS, &r.Body); err != nil {
			rows.Close()
			return nil, err
		}
		userID := id.UserID(sender)
		r.IsMe = userID == Me || s.isSelf(userID)
		r.SenderName = sender
		out = append(out, r)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return []SearchResult{}, nil
	}
	// Chat names and avatars as the chat list shows them (DMs are named
	// after the other person), and senders' display names.
	chats, err := s.Chats(ctx)
	if err != nil {
		return nil, err
	}
	byRoom := make(map[string]Chat, len(chats))
	for _, c := range chats {
		byRoom[c.RoomID] = c
	}
	profiles, err := s.profiles(ctx)
	if err != nil {
		return nil, err
	}
	for i := range out {
		r := &out[i]
		c := byRoom[r.RoomID]
		r.ChatName, r.ChatAvatar, r.Network, r.IsDM = c.Name, c.Avatar, c.Network, c.IsDM
		r.SenderName = profiles[id.UserID(r.SenderName)].name
	}
	return out, nil
}

// searchRows runs the search, in one chat if roomID is set.
func (s *Service) searchRows(ctx context.Context, query, roomID string, limit, offset int) (*sql.Rows, error) {
	return s.Store.DB.QueryContext(ctx, `
		SELECT e.event_id, e.room_id, e.sender, e.ts, json_extract(e.content, '$.body') AS body
		FROM events e JOIN rooms r ON r.room_id = e.room_id AND r.is_portal = 1
		WHERE e.type = 'm.room.message' AND e.redacted = 0 AND (? = '' OR e.room_id = ?)
			AND (json_extract(e.content, '$.msgtype') IN ('m.text', 'm.notice', 'm.emote')
				-- a photo or video's body is its caption when it has a separate file name
				OR (json_extract(e.content, '$.filename') IS NOT NULL
					AND json_extract(e.content, '$.body') <> json_extract(e.content, '$.filename')))
			AND body LIKE ? ESCAPE '\'
		ORDER BY e.ts DESC LIMIT ? OFFSET ?`,
		roomID, roomID, "%"+likeEscaper.Replace(query)+"%", limit, offset)
}

// maxChatMatches caps how many matches a search in one chat steps through.
const maxChatMatches = 500

// SearchChat finds messages in one chat containing query, newest first, as
// event IDs to step through.
func (s *Service) SearchChat(ctx context.Context, roomID, query string) ([]string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []string{}, nil
	}
	rows, err := s.searchRows(ctx, query, roomID, maxChatMatches, 0)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var eventID, room, sender, body string
		var ts int64
		if err = rows.Scan(&eventID, &room, &sender, &ts, &body); err != nil {
			return nil, err
		}
		out = append(out, eventID)
	}
	return out, rows.Err()
}

// likeEscaper escapes LIKE's wildcards so a search for "50%" means that.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// MessagesThrough loads a chat from its newest message back to (and a few
// messages before) eventID, for jumping to a search result.
func (s *Service) MessagesThrough(ctx context.Context, roomID, eventID string) ([]Message, error) {
	var ts int64
	if err := s.Store.DB.QueryRowContext(ctx, "SELECT ts FROM events WHERE event_id=? AND room_id=?", eventID, roomID).Scan(&ts); err != nil {
		return nil, err
	}
	var newer int
	if err := s.Store.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE room_id=? AND ts>=?", roomID, ts).Scan(&newer); err != nil {
		return nil, err
	}
	return s.Messages(ctx, roomID, 0, newer+10)
}
