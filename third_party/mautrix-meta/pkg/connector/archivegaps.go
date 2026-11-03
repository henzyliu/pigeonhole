package connector

// Pigeonhole: this file isn't in upstream mautrix-meta.
//
// Messages you send from another device sometimes can't be decrypted when
// they arrive. They're still in the chat's secure history, but catch-up only
// keeps messages newer than the latest one bridged, and only runs on
// reconnecting. So they arrive late, or never if a reply got in first. This
// runs catch-up soon after a failure, and has it fill recent gaps too.

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow/proto/waArmadilloBackupMessage"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/proto/waMsgApplication"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/simplevent"
)

// archiveGapWindow is how far back catch-up looks for missing messages.
const archiveGapWindow = 7 * 24 * time.Hour

// decryptCatchupDelays are the waits between catch-ups after a message can't
// be decrypted, as it takes a moment to reach the secure history.
var decryptCatchupDelays = []time.Duration{15 * time.Second, 45 * time.Second, 2 * time.Minute}

// catchUpAfterDecryptFailure runs catch-up for a chat a few times, once per
// burst of failures: each failed retry of a message reports another one.
func (m *MetaClient) catchUpAfterDecryptFailure(chat types.JID) {
	key := chat.String()
	if !m.decryptCatchups.Add(key) {
		return
	}
	go func() {
		defer m.decryptCatchups.Remove(key)
		for _, delay := range decryptCatchupDelays {
			select {
			case <-time.After(delay):
			case <-m.Main.Bridge.BackgroundCtx.Done():
				return
			}
			m.UserLogin.QueueRemoteEvent(&simplevent.ChatResync{
				EventMeta: simplevent.EventMeta{
					Type:      bridgev2.RemoteEventChatResync,
					PortalKey: m.makeWAPortalKey(chat),
				},
				CheckNeedsBackfillFunc: func(ctx context.Context, latest *database.Message) (bool, error) {
					return latest != nil, nil
				},
			})
		}
	}()
}

// checkThreadForGaps says whether to run catch-up for a recently active
// encrypted chat that has nothing newer to fetch, once per connection, to
// pick up messages that were missed earlier.
func (r *FBChatResync) checkThreadForGaps() bool {
	var encrypted bool
	switch {
	case r.Raw != nil:
		encrypted = r.Raw.ThreadType.IsWhatsApp()
	case r.Update != nil:
		encrypted = r.Update.ThreadType.IsWhatsApp()
	}
	if !encrypted || time.Since(time.UnixMilli(r.getLastActivityTimestampMs())) > archiveGapWindow {
		return false
	}
	return r.m.gapCheckedThreads.Add(string(r.PortalKey.ID))
}

// fillArchiveGaps adds messages from the secure history that are older than
// the latest one bridged but were never bridged. Catch-up would discard them,
// so they're queued as if they'd just arrived, keeping their own timestamps.
func (m *MetaClient) fillArchiveGaps(ctx context.Context, portal *bridgev2.Portal, anchor *database.Message, messages []*bridgev2.BackfillMessage) {
	missing := missingArchiveMessages(anchor, messages, time.Now(), func(msg *bridgev2.BackfillMessage) bool {
		existing, err := m.Main.Bridge.DB.Message.GetFirstPartByID(ctx, portal.Receiver, msg.ID)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Str("message_id", string(msg.ID)).Msg("Failed to check for missing message")
			return true
		}
		return existing != nil
	})
	if len(missing) == 0 {
		return
	}
	ids := make([]string, len(missing))
	for i, msg := range missing {
		ids[i] = string(msg.ID)
	}
	zerolog.Ctx(ctx).Info().Strs("message_ids", ids).Msg("Filling gaps from secure history")
	// Catch-up holds the chat's event lock, which these wait for.
	go func() {
		for _, msg := range missing {
			m.UserLogin.QueueRemoteEvent(&simplevent.PreConvertedMessage{
				EventMeta: simplevent.EventMeta{
					Type:        bridgev2.RemoteEventMessage,
					PortalKey:   portal.PortalKey,
					Sender:      msg.Sender,
					Timestamp:   msg.Timestamp,
					StreamOrder: msg.StreamOrder,
				},
				Data: msg.ConvertedMessage,
				ID:   msg.ID,
			})
		}
	}()
}

// missingArchiveMessages picks the messages to fill in: recent, at or before
// the latest bridged message, not bridged already, with something to show,
// and not past their disappearing timer.
func missingArchiveMessages(anchor *database.Message, messages []*bridgev2.BackfillMessage, now time.Time, exists func(*bridgev2.BackfillMessage) bool) []*bridgev2.BackfillMessage {
	var missing []*bridgev2.BackfillMessage
	for _, msg := range messages {
		if msg.ID == anchor.ID || msg.Timestamp.After(anchor.Timestamp) || now.Sub(msg.Timestamp) > archiveGapWindow {
			continue
		}
		if msg.ConvertedMessage == nil || len(msg.Parts) == 0 {
			continue
		}
		if timer := msg.Disappear.Timer; timer > 0 && now.Sub(msg.Timestamp) > timer {
			continue
		}
		if !exists(msg) {
			missing = append(missing, msg)
		}
	}
	return missing
}

// archiveRevokes says whether a supplemental to a message in the secure
// history unsends that message.
func archiveRevokes(supplemental *waArmadilloBackupMessage.BackupMessage, messageID string) bool {
	payload, ok := supplemental.GetPayload().(*waArmadilloBackupMessage.BackupMessage_EncryptedTransportMessage)
	if !ok {
		return false
	}
	var application waMsgApplication.MessageApplication
	if proto.Unmarshal(payload.EncryptedTransportMessage, &application) != nil {
		return false
	}
	var consumer waConsumerApplication.ConsumerApplication
	if proto.Unmarshal(application.GetPayload().GetSubProtocol().GetConsumerMessage().GetPayload(), &consumer) != nil {
		return false
	}
	revoke := consumer.GetPayload().GetApplicationData().GetRevoke()
	if revoke == nil {
		return false
	}
	target := revoke.GetKey().GetID()
	return target == "" || target == messageID
}
