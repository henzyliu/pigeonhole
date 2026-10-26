package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"unified-inbox/internal/localmx"
)

// changeTimeout is how long an edit or unsend waits for the network before
// it counts as failed.
const changeTimeout = time.Minute

// limitMargin keeps Edit and Unsend from being offered in the last moments
// before the network's limit, when they'd likely arrive too late.
const limitMargin = 30 * time.Second

// changeLimits is what a chat allows for your own messages; a zero max age
// means no limit.
type changeLimits struct {
	edit, unsend             bool
	editMaxAge, unsendMaxAge time.Duration
}

func (l changeLimits) canEdit(age time.Duration) bool {
	return l.edit && (l.editMaxAge == 0 || age < l.editMaxAge-limitMargin)
}

func (l changeLimits) canUnsend(age time.Duration) bool {
	return l.unsend && (l.unsendMaxAge == 0 || age < l.unsendMaxAge-limitMargin)
}

// limitsFor asks the chat's bridge what it allows.
func (s *Service) limitsFor(ctx context.Context, roomID string) changeLimits {
	n, err := s.roomNetwork(ctx, roomID)
	if err != nil || !n.started.Load() {
		return changeLimits{}
	}
	portal, err := n.Bridge.GetPortalByMXID(ctx, id.RoomID(roomID))
	if err != nil || portal == nil {
		return changeLimits{}
	}
	user, err := n.Bridge.GetUserByMXID(ctx, Me)
	if err != nil || user == nil {
		return changeLimits{}
	}
	login, _, err := portal.FindPreferredLogin(ctx, user, false)
	if err != nil || login == nil || login.Client == nil {
		return changeLimits{}
	}
	caps := login.Client.GetCapabilities(ctx, portal)
	if caps == nil {
		return changeLimits{}
	}
	l := changeLimits{edit: caps.Edit.Partial(), unsend: caps.Delete.Partial()}
	if caps.EditMaxAge != nil {
		l.editMaxAge = caps.EditMaxAge.Duration
	}
	if caps.DeleteMaxAge != nil {
		l.unsendMaxAge = caps.DeleteMaxAge.Duration
	}
	return l
}

// attachChangeLimits marks which of your messages can still be edited or
// unsent.
func (s *Service) attachChangeLimits(ctx context.Context, roomID string, msgs []Message) {
	var limits *changeLimits
	now := time.Now()
	for i := range msgs {
		m := &msgs[i]
		if !m.IsMe || m.Redacted || m.Pending != "" || (m.Status != "" && m.Status != localmx.StatusSuccess && m.Status != localmx.StatusDelivered) {
			continue
		}
		if limits == nil {
			l := s.limitsFor(ctx, roomID)
			limits = &l
		}
		age := now.Sub(time.UnixMilli(m.TS))
		m.CanEdit = m.Kind == "text" && limits.canEdit(age)
		m.CanUnsend = limits.canUnsend(age)
	}
}

// ownMessage loads one of your messages for editing or unsending.
func (s *Service) ownMessage(ctx context.Context, roomID, eventID string) (*Message, error) {
	var sender, evtType, raw string
	m := &Message{EventID: eventID}
	err := s.Store.DB.QueryRowContext(ctx, `
		SELECT sender, type, content, ts, status, redacted, pending_kind FROM events WHERE event_id=? AND room_id=?`,
		eventID, roomID).Scan(&sender, &evtType, &raw, &m.TS, &m.Status, &m.Redacted, &m.Pending)
	if err != nil {
		return nil, errors.New("couldn't find that message")
	}
	if userID := id.UserID(sender); userID != Me && !s.isSelf(userID) {
		return nil, errors.New("you can only change your own messages")
	} else if m.Redacted {
		return nil, errors.New("that message has been deleted")
	} else if m.Pending != "" {
		return nil, localmx.ErrChangePending
	}
	fillContent(m, evtType, raw)
	return m, nil
}

// EditMessage changes the text of one of your messages. The message only
// changes for good once the network confirms it (see localmx/changes.go).
func (s *Service) EditMessage(ctx context.Context, roomID, eventID, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("a message can't be edited to nothing; unsend it instead")
	}
	m, err := s.ownMessage(ctx, roomID, eventID)
	if err != nil {
		return err
	} else if m.Kind != "text" {
		return errors.New("only text messages can be edited")
	} else if m.Body == text {
		return nil
	}
	limits := s.limitsFor(ctx, roomID)
	if !limits.edit {
		return errors.New("this chat doesn't allow editing")
	} else if !limits.canEdit(time.Since(time.UnixMilli(m.TS)) - limitMargin) {
		return errors.New("it's too late to edit that message")
	}
	newContent := &event.MessageEventContent{MsgType: event.MsgText, Body: text}
	raw, err := json.Marshal(newContent)
	if err != nil {
		return err
	}
	return s.sendChange(ctx, roomID, eventID, localmx.ChangeEdit, raw, event.EventMessage, &event.MessageEventContent{
		MsgType:    event.MsgText,
		Body:       "* " + text,
		NewContent: newContent,
		RelatesTo:  &event.RelatesTo{Type: event.RelReplace, EventID: id.EventID(eventID)},
	})
}

// UnsendMessage removes one of your messages for everyone, once the
// network confirms it.
func (s *Service) UnsendMessage(ctx context.Context, roomID, eventID string) error {
	m, err := s.ownMessage(ctx, roomID, eventID)
	if err != nil {
		return err
	}
	limits := s.limitsFor(ctx, roomID)
	if !limits.unsend {
		return errors.New("this chat doesn't allow unsending")
	} else if !limits.canUnsend(time.Since(time.UnixMilli(m.TS)) - limitMargin) {
		return errors.New("it's too late to unsend that message")
	}
	return s.sendChange(ctx, roomID, eventID, localmx.ChangeUnsend, nil, event.EventRedaction,
		&event.RedactionEventContent{Redacts: id.EventID(eventID)})
}

// sendChange records the change as pending and hands it to the bridge,
// which reports back on it like any message it sends.
func (s *Service) sendChange(ctx context.Context, roomID, target, kind string, newContent []byte, evtType event.Type, content any) error {
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
	if evtType == event.EventRedaction {
		evt.Redacts = id.EventID(target)
	}
	if err = s.Store.StartChange(ctx, evt.RoomID, id.EventID(target), kind, evt.ID, newContent); err != nil {
		return err
	}
	go n.Bridge.QueueMatrixEvent(n.Bridge.BackgroundCtx, evt)
	time.AfterFunc(changeTimeout, func() { s.Store.FailChange(context.Background(), evt.ID) })
	return nil
}
