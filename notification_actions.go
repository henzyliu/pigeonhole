package main

import (
	"context"
	"strings"

	"github.com/rs/zerolog"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Message alerts carry a Reply box and a Mark as read button, so a quick
// answer doesn't need the window.
const (
	messageCategory   = "pigeonhole-message"
	replyActionID     = "TEXT_REPLY" // what Wails reports for the reply box on macOS and Windows
	markReadActionID  = "MARK_READ"
	replyFailedPrefix = "pigeonhole-reply-failed-"
)

func registerMessageActions(ctx context.Context, log zerolog.Logger) bool {
	err := runtime.RegisterNotificationCategory(ctx, runtime.NotificationCategory{
		ID:               messageCategory,
		Actions:          []runtime.NotificationAction{{ID: markReadActionID, Title: "Mark as read"}},
		HasReplyField:    true,
		ReplyPlaceholder: "Message",
		ReplyButtonTitle: "Reply",
	})
	if err != nil {
		log.Warn().Err(err).Msg("Notification actions unavailable; alerts won't offer Reply")
		return false
	}
	return true
}

// messageAction is what a Reply or Mark as read on a message alert asks for.
type messageAction struct {
	roomID string
	reply  string // the text to send; "" for Mark as read
}

// parseMessageAction reads a notification response as a message alert's
// action. ok is false for anything else (a plain click, another kind of
// alert), and for an empty reply, which has nothing to send.
func parseMessageAction(resp runtime.NotificationResponse) (action messageAction, ok bool) {
	roomID, _ := resp.UserInfo["roomId"].(string)
	if roomID == "" {
		return action, false
	}
	switch resp.ActionIdentifier {
	case replyActionID:
		text := strings.TrimSpace(resp.UserText)
		if text == "" {
			return action, false
		}
		return messageAction{roomID: roomID, reply: text}, true
	case markReadActionID:
		return messageAction{roomID: roomID}, true
	}
	return action, false
}

// handleMessageAction carries out Reply or Mark as read, and reports whether
// the response was one of those. It runs in the background so the alert
// isn't held up while the message goes out.
func (a *App) handleMessageAction(resp runtime.NotificationResponse) bool {
	action, ok := parseMessageAction(resp)
	if !ok {
		// An empty reply is still a reply: don't treat it as a click that
		// opens the window.
		return resp.ActionIdentifier == replyActionID
	}
	go func() {
		log := a.svc.Log.With().Str("room_id", action.roomID).Logger()
		if action.reply != "" {
			if err := a.svc.SendText(a.ctx, action.roomID, action.reply, "", nil); err != nil {
				// Only the chat is logged, never the message.
				log.Warn().Err(err).Msg("Failed to send reply from notification")
				a.replyFailed(action.roomID, resp.Title)
				return
			}
			log.Info().Msg("Sent reply from notification")
		}
		// Answering a chat means you've seen it, as on a phone.
		if err := a.svc.MarkRead(a.ctx, action.roomID); err != nil {
			log.Warn().Err(err).Msg("Failed to mark chat read from notification")
		}
		_ = runtime.RemoveDeliveredNotification(a.ctx, action.roomID)
	}()
	return true
}

// replyFailed says a reply from an alert didn't go out; clicking it opens
// the chat to try again.
func (a *App) replyFailed(roomID, chatTitle string) {
	body := "Open the chat to try again."
	if chatTitle != "" {
		body = "Your reply to " + chatTitle + " wasn't sent. " + body
	}
	err := runtime.SendNotification(a.ctx, runtime.NotificationOptions{
		ID:    replyFailedPrefix + roomID,
		Title: "Couldn't send your reply",
		Body:  body,
		Data:  map[string]any{"roomId": roomID},
	})
	if err != nil {
		a.svc.Log.Warn().Err(err).Str("room_id", roomID).Msg("Failed to send reply-failed notification")
	}
}
