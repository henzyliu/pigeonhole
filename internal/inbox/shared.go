package inbox

import (
	"context"
	"fmt"

	"maunium.net/go/mautrix/id"
)

// The Shared panel lists what's been exchanged in a chat: photos and
// videos, files and voice messages, and messages with links, newest first.

// SharedPageSize is how many items the Shared panel loads at once.
const SharedPageSize = 60

// sharedFilters picks each kind of shared item.
var sharedFilters = map[string]string{
	"media": "json_extract(content, '$.msgtype') IN ('m.image', 'm.video')",
	"files": "json_extract(content, '$.msgtype') IN ('m.file', 'm.audio')",
	"links": "json_extract(content, '$.msgtype') IN ('m.text', 'm.notice', 'm.emote') AND (json_extract(content, '$.body') LIKE '%http://%' OR json_extract(content, '$.body') LIKE '%https://%')",
}

// ChatShared lists one kind of item shared in a chat ("media", "files" or
// "links"), newest first, skipping offset items.
func (s *Service) ChatShared(ctx context.Context, roomID, kind string, offset int) ([]Message, error) {
	filter, ok := sharedFilters[kind]
	if !ok {
		return nil, fmt.Errorf("unknown kind %q", kind)
	}
	profiles, err := s.profiles(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.Store.DB.QueryContext(ctx, `
		SELECT event_id, sender, type, content, ts FROM events
		WHERE room_id=? AND type='m.room.message' AND redacted=0 AND `+filter+`
		ORDER BY ts DESC LIMIT ? OFFSET ?`, roomID, SharedPageSize, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		var sender, evtType, raw string
		if err = rows.Scan(&m.EventID, &sender, &evtType, &raw, &m.TS); err != nil {
			return nil, err
		}
		userID := id.UserID(sender)
		m.IsMe = userID == Me || s.isSelf(userID)
		p := profiles[userID]
		m.SenderName, m.SenderAvatar = p.name, MediaURL(p.avatar)
		fillContent(&m, evtType, raw)
		out = append(out, m)
	}
	return out, rows.Err()
}
