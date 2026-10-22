package inbox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"unified-inbox/internal/localmx"
)

type Chat struct {
	RoomID  string `json:"roomId"`
	Network string `json:"network"`
	Name    string `json:"name"`
	Avatar  string `json:"avatar"`
	Preview string `json:"preview"`
	LastTS  int64  `json:"lastTs"`
	Unread  int    `json:"unread"`
	IsDM    bool   `json:"isDm"`
	// Category is "inbox" (one-to-one), "groups", "marketplace" or "requests".
	Category string `json:"category"`
}

type Reaction struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
	Mine  bool   `json:"mine"`
}

type Message struct {
	EventID      string     `json:"eventId"`
	SenderName   string     `json:"senderName"`
	SenderAvatar string     `json:"senderAvatar"`
	IsMe         bool       `json:"isMe"`
	TS           int64      `json:"ts"`
	Kind         string     `json:"kind"` // text, notice, emote, image, video, audio, file
	Body         string     `json:"body"`
	MediaURL     string     `json:"mediaUrl"`
	MimeType     string     `json:"mimeType"`
	FileName     string     `json:"fileName"`
	Width        int        `json:"width"`
	Height       int        `json:"height"`
	Status       string     `json:"status"`
	Edited       bool       `json:"edited"`
	Redacted     bool       `json:"redacted"`
	ReplyTo      string     `json:"replyTo"`
	Reactions    []Reaction `json:"reactions"`
	// SeenBy lists the people whose read position is this message.
	SeenBy []Reader `json:"seenBy"`

	senderID id.UserID
}

type Reader struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

type profile struct{ name, avatar string }

const mediaPrefix = "/media/"

// MediaURL converts an mxc:// URI into a URL served by MediaHandler.
func MediaURL(mxc string) string {
	if mxc == "" {
		return ""
	}
	return mediaPrefix + strings.TrimPrefix(mxc, "mxc://"+localmx.ServerName+"/")
}

// MediaHandler serves locally stored media to the webview.
func (s *Service) MediaHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaID, ok := strings.CutPrefix(r.URL.Path, mediaPrefix)
		if !ok || strings.ContainsAny(mediaID, "/\\") {
			http.NotFound(w, r)
			return
		}
		path, mime, err := s.Store.MediaPath(r.Context(), id.ContentURIString("mxc://"+localmx.ServerName+"/"+mediaID))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if mime != "" {
			w.Header().Set("Content-Type", mime)
		}
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		http.ServeFile(w, r, path)
	})
}

func (s *Service) profiles(ctx context.Context) (map[id.UserID]profile, error) {
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT user_id, name, avatar FROM profiles")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[id.UserID]profile)
	for rows.Next() {
		var userID string
		var p profile
		if err = rows.Scan(&userID, &p.name, &p.avatar); err != nil {
			return nil, err
		}
		out[id.UserID(userID)] = p
	}
	return out, rows.Err()
}

func (s *Service) Chats(ctx context.Context) ([]Chat, error) {
	profiles, err := s.profiles(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.Store.DB.QueryContext(ctx, `
		SELECT room_id, network, name, avatar, preview, last_ts, unread, is_dm
		FROM rooms WHERE is_portal=1 ORDER BY last_ts DESC`)
	if err != nil {
		return nil, err
	}
	var chats []Chat
	for rows.Next() {
		var c Chat
		var avatar string
		if err = rows.Scan(&c.RoomID, &c.Network, &c.Name, &avatar, &c.Preview, &c.LastTS, &c.Unread, &c.IsDM); err != nil {
			rows.Close()
			return nil, err
		}
		c.Avatar = MediaURL(avatar)
		chats = append(chats, c)
	}
	rows.Close()
	// Folder portals (e.g. Messenger's Marketplace space) are containers, not
	// conversations, so they never have messages and shouldn't be listed.
	chats = slices.DeleteFunc(chats, func(c Chat) bool {
		portal := s.portalFor(ctx, c.Network, c.RoomID)
		return portal != nil && portal.RoomType == database.RoomTypeSpace
	})
	for i := range chats {
		chats[i].Category = chatCategory(s.portalFor(ctx, chats[i].Network, chats[i].RoomID), chats[i].IsDM)
	}
	// DMs without an explicit name are named after the other person.
	for i := range chats {
		if chats[i].Name != "" && chats[i].Avatar != "" {
			continue
		}
		members, err := s.Store.Members(ctx, id.RoomID(chats[i].RoomID))
		if err != nil {
			return nil, err
		}
		var names []string
		for userID := range members {
			p, ok := profiles[userID]
			if !ok || s.isSelf(userID) || userID == Me {
				continue
			}
			names = append(names, p.name)
			if chats[i].Avatar == "" && chats[i].IsDM {
				chats[i].Avatar = MediaURL(p.avatar)
			}
		}
		if chats[i].Name == "" {
			chats[i].Name = strings.Join(names, ", ")
		}
	}
	return chats, nil
}

// Chat categories. Marketplace and requests mirror the folders Messenger and
// Instagram show; the rest of the inbox is split into one-to-one chats
// ("inbox", shown as People) and group chats.
const (
	CategoryInbox       = "inbox"
	CategoryGroups      = "groups"
	CategoryMarketplace = "marketplace"
	CategoryRequests    = "requests"
)

func (s *Service) portalFor(ctx context.Context, network, roomID string) *bridgev2.Portal {
	n, ok := s.networks[network]
	if !ok {
		return nil
	}
	portal, err := n.Bridge.GetPortalByMXID(ctx, id.RoomID(roomID))
	if err != nil {
		return nil
	}
	return portal
}

// chatCategory picks the tab a chat belongs in. isDM is the room's own flag,
// used when the bridge has no portal record for it.
func chatCategory(portal *bridgev2.Portal, isDM bool) string {
	switch {
	case portal == nil && isDM:
		return CategoryInbox
	case portal == nil:
		return CategoryGroups
	case portal.MessageRequest:
		return CategoryRequests
	// On Messenger only Marketplace threads live under a parent folder. The
	// thread type is checked too, since linking to the folder can fail.
	case portal.ParentKey.ID != "":
		return CategoryMarketplace
	case isMarketplaceThread(portal):
		return CategoryMarketplace
	case portal.RoomType == database.RoomTypeDM:
		return CategoryInbox
	default:
		return CategoryGroups
	}
}

func isMarketplaceThread(portal *bridgev2.Portal) bool {
	meta, ok := portal.Metadata.(*metaid.PortalMetadata)
	return ok && meta.ThreadType == table.MARKETPLACE
}

func (s *Service) Messages(ctx context.Context, roomID string, beforeTS int64, limit int) ([]Message, error) {
	if beforeTS <= 0 {
		beforeTS = 1 << 62
	}
	profiles, err := s.profiles(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.Store.DB.QueryContext(ctx, `
		SELECT event_id, sender, type, content, ts, status, edited, redacted
		FROM events WHERE room_id=? AND ts<? ORDER BY ts DESC LIMIT ?`, roomID, beforeTS, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []Message
	index := make(map[string]int)
	for rows.Next() {
		var sender, evtType, raw string
		var m Message
		if err = rows.Scan(&m.EventID, &sender, &evtType, &raw, &m.TS, &m.Status, &m.Edited, &m.Redacted); err != nil {
			return nil, err
		}
		senderID := id.UserID(sender)
		m.senderID = senderID
		m.IsMe = senderID == Me || s.isSelf(senderID)
		p := profiles[senderID]
		m.SenderName, m.SenderAvatar = p.name, MediaURL(p.avatar)
		if !m.Redacted {
			fillContent(&m, evtType, raw)
		}
		index[m.EventID] = len(msgs)
		msgs = append(msgs, m)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = s.attachReactions(ctx, roomID, msgs, index); err != nil {
		return nil, err
	}
	if err = s.attachReceipts(ctx, roomID, msgs, profiles); err != nil {
		return nil, err
	}
	// Oldest first, which is how the thread view renders.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

func fillContent(m *Message, evtType, raw string) {
	var content event.Content
	if json.Unmarshal([]byte(raw), &content) != nil {
		return
	}
	_ = content.ParseRaw(event.Type{Type: evtType, Class: event.MessageEventType})
	msg := content.AsMessage()
	m.Body = msg.Body
	m.ReplyTo = string(msg.RelatesTo.GetReplyTo())
	switch {
	case evtType == event.EventSticker.Type:
		m.Kind = "image"
	case msg.MsgType == event.MsgImage, msg.MsgType == event.MsgVideo,
		msg.MsgType == event.MsgAudio, msg.MsgType == event.MsgFile:
		m.Kind = string(msg.MsgType)[2:]
	case msg.MsgType == event.MsgNotice, msg.MsgType == event.MsgEmote:
		m.Kind = string(msg.MsgType)[2:]
	default:
		m.Kind = "text"
	}
	if m.Kind != "text" && m.Kind != "notice" && m.Kind != "emote" {
		m.Body = msg.GetCaption()
		m.FileName = msg.GetFileName()
		uri := msg.URL
		if msg.File != nil {
			uri = msg.File.URL
		}
		m.MediaURL = MediaURL(string(uri))
		if msg.Info != nil {
			m.MimeType, m.Width, m.Height = msg.Info.MimeType, msg.Info.Width, msg.Info.Height
		}
	}
}

func (s *Service) attachReactions(ctx context.Context, roomID string, msgs []Message, index map[string]int) error {
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT target, sender, key FROM reactions WHERE room_id=?", roomID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var target, sender, key string
		if err = rows.Scan(&target, &sender, &key); err != nil {
			return err
		}
		i, ok := index[target]
		if !ok {
			continue
		}
		mine := id.UserID(sender) == Me || s.isSelf(id.UserID(sender))
		found := false
		for j := range msgs[i].Reactions {
			if r := &msgs[i].Reactions[j]; r.Key == key {
				r.Count++
				r.Mine = r.Mine || mine
				found = true
			}
		}
		if !found {
			msgs[i].Reactions = append(msgs[i].Reactions, Reaction{Key: key, Count: 1, Mine: mine})
		}
	}
	return rows.Err()
}

// attachReceipts marks each other person's read position on the newest
// message at or before it, like Messenger's "Seen" and reader avatars. msgs
// is newest first. A reader isn't shown on their own message.
func (s *Service) attachReceipts(ctx context.Context, roomID string, msgs []Message, profiles map[id.UserID]profile) error {
	if len(msgs) == 0 {
		return nil
	}
	rows, err := s.Store.DB.QueryContext(ctx, `
		SELECT r.user_id, COALESCE(e.ts, -1)
		FROM receipts r LEFT JOIN events e ON e.event_id = r.event_id
		WHERE r.room_id=?`, roomID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var userID string
		var readTS int64
		if err = rows.Scan(&userID, &readTS); err != nil {
			return err
		}
		reader := id.UserID(userID)
		if readTS < 0 || reader == Me || s.isSelf(reader) {
			continue
		}
		for i := range msgs {
			if msgs[i].TS > readTS {
				continue
			}
			if msgs[i].senderID != reader {
				p := profiles[reader]
				msgs[i].SeenBy = append(msgs[i].SeenBy, Reader{Name: p.name, Avatar: MediaURL(p.avatar)})
			}
			break
		}
	}
	return rows.Err()
}

func (s *Service) roomNetwork(ctx context.Context, roomID string) (*Network, error) {
	var netID string
	if err := s.Store.DB.QueryRowContext(ctx, "SELECT network FROM rooms WHERE room_id=?", roomID).Scan(&netID); err != nil {
		return nil, fmt.Errorf("unknown chat: %w", err)
	}
	return s.network(netID)
}

// sendLocal stores an event authored in the UI and hands it to the bridge,
// exactly as a Matrix homeserver would forward it to an appservice.
func (s *Service) sendLocal(ctx context.Context, roomID string, evtType event.Type, content any) error {
	n, err := s.roomNetwork(ctx, roomID)
	if err != nil {
		return err
	}
	evt := &event.Event{
		ID:        localmx.NewEventID(),
		RoomID:    id.RoomID(roomID),
		Sender:    Me,
		Type:      evtType,
		Timestamp: time.Now().UnixMilli(),
		Content:   event.Content{Parsed: content},
	}
	if err = s.Store.HandleEvent(ctx, evt, false); err != nil {
		return err
	}
	if evtType == event.EventMessage {
		s.Store.SetEventStatus(ctx, evt.ID, evt.RoomID, "pending")
	}
	go n.Bridge.QueueMatrixEvent(n.Bridge.BackgroundCtx, evt)
	return nil
}

func (s *Service) SendText(ctx context.Context, roomID, text, replyTo string) error {
	content := &event.MessageEventContent{MsgType: event.MsgText, Body: text}
	if replyTo != "" {
		content.RelatesTo = (&event.RelatesTo{}).SetReplyTo(id.EventID(replyTo))
	}
	return s.sendLocal(ctx, roomID, event.EventMessage, content)
}

func (s *Service) React(ctx context.Context, roomID, eventID, key string) error {
	return s.sendLocal(ctx, roomID, event.EventReaction, &event.ReactionEventContent{
		RelatesTo: event.RelatesTo{Type: event.RelAnnotation, EventID: id.EventID(eventID), Key: key},
	})
}

func (s *Service) MarkRead(ctx context.Context, roomID string) error {
	if err := s.Store.MarkRead(ctx, id.RoomID(roomID)); err != nil {
		return err
	}
	n, err := s.roomNetwork(ctx, roomID)
	if err != nil {
		return err
	}
	var lastEvent string
	err = s.Store.DB.QueryRowContext(ctx, "SELECT event_id FROM events WHERE room_id=? ORDER BY ts DESC LIMIT 1", roomID).Scan(&lastEvent)
	if err != nil {
		return nil
	}
	receipt := &event.Event{
		Type:   event.EphemeralEventReceipt,
		RoomID: id.RoomID(roomID),
		// Without this flag bridgev2 handles the receipt like a timeline event,
		// which needs a sender and panics without one.
		Mautrix: event.MautrixInfo{EventSource: event.SourceEphemeral},
		Content: event.Content{Parsed: &event.ReceiptEventContent{
			id.EventID(lastEvent): {event.ReceiptTypeRead: {Me: {Timestamp: time.Now()}}},
		}},
	}
	go n.Bridge.QueueMatrixEvent(n.Bridge.BackgroundCtx, receipt)
	return nil
}
