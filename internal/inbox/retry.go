package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"unified-inbox/internal/localmx"
)

// A message that fails in a way the bridge calls retriable (typically sent
// while its account was reconnecting) is sent again once by itself, a few
// seconds later. If that fails too, it shows as "Failed to send" with
// Try again.

// autoRetryDelay gives a reconnecting account time to come back. Tests
// shorten it.
var autoRetryDelay = 5 * time.Second

// autoRetryMaxAge stops old messages being sent long after they were
// written, which would be confusing.
const autoRetryMaxAge = 2 * time.Minute

func (s *Service) onRetriableFailure(eventID id.EventID, roomID id.RoomID) {
	s.lock.Lock()
	if s.autoRetried == nil {
		s.autoRetried = make(map[id.EventID]bool)
	}
	already := s.autoRetried[eventID]
	s.autoRetried[eventID] = true
	s.lock.Unlock()
	if already {
		return
	}
	time.AfterFunc(autoRetryDelay, func() {
		ctx := s.ctx
		if ctx == nil || ctx.Err() != nil {
			return
		}
		newID, err := s.resend(ctx, string(roomID), string(eventID), autoRetryMaxAge)
		if err != nil {
			s.Log.Debug().Err(err).Stringer("event_id", eventID).Msg("Not retrying failed message")
			return
		}
		s.Log.Info().Stringer("event_id", eventID).Stringer("new_event_id", newID).Msg("Retried a failed message")
		// The retry gets one automatic attempt of its own, no more.
		s.lock.Lock()
		s.autoRetried[newID] = true
		s.lock.Unlock()
	})
}

// RetrySend sends a failed message again (the Try again button).
func (s *Service) RetrySend(ctx context.Context, roomID, eventID string) error {
	_, err := s.resend(ctx, roomID, eventID, 0)
	return err
}

// resend sends a failed message of yours again as a new message, then
// removes the failed one. maxAge, if set, skips messages older than that.
func (s *Service) resend(ctx context.Context, roomID, eventID string, maxAge time.Duration) (id.EventID, error) {
	var sender, evtType, raw, status string
	var ts int64
	err := s.Store.DB.QueryRowContext(ctx, `
		SELECT sender, type, content, status, ts FROM events WHERE event_id=? AND room_id=?`,
		eventID, roomID).Scan(&sender, &evtType, &raw, &status, &ts)
	if err != nil {
		return "", errors.New("couldn't find that message")
	}
	if userID := id.UserID(sender); userID != Me && !s.isSelf(userID) {
		return "", errors.New("that isn't your message")
	} else if !localmx.SendFailed(status) {
		return "", errors.New("that message didn't fail")
	} else if maxAge > 0 && time.Since(time.UnixMilli(ts)) > maxAge {
		return "", errors.New("too old to retry by itself")
	}
	var content event.Content
	if err = json.Unmarshal([]byte(raw), &content); err != nil {
		return "", err
	}
	t := event.Type{Type: evtType, Class: event.MessageEventType}
	if err = content.ParseRaw(t); err != nil {
		return "", err
	}
	newID, err := s.sendLocalID(ctx, roomID, t, content.Parsed)
	if err != nil {
		return "", err
	}
	return newID, s.Store.DeleteFailedMessage(ctx, id.EventID(eventID), id.RoomID(roomID))
}
