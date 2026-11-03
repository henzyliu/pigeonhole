package connector

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waArmadilloBackupMessage"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/proto/waMsgApplication"
	"google.golang.org/protobuf/proto"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

func TestMissingArchiveMessages(t *testing.T) {
	now := time.Date(2026, 11, 3, 13, 0, 0, 0, time.UTC)
	anchor := &database.Message{ID: "latest", Timestamp: now.Add(-time.Minute)}
	msg := func(id networkid.MessageID, age time.Duration, disappear time.Duration) *bridgev2.BackfillMessage {
		return &bridgev2.BackfillMessage{
			ID:        id,
			Timestamp: now.Add(-age),
			ConvertedMessage: &bridgev2.ConvertedMessage{
				Parts:     []*bridgev2.ConvertedMessagePart{{}},
				Disappear: database.DisappearingSetting{Timer: disappear},
			},
		}
	}
	empty := msg("empty", time.Hour, 0)
	empty.Parts = nil
	got := missingArchiveMessages(anchor, []*bridgev2.BackfillMessage{
		msg("too-old", 8*24*time.Hour, 0),
		msg("known", 2*time.Hour, 0),
		msg("gap", time.Hour, 0),
		msg("expired", time.Hour, time.Minute),
		msg("timer-running", time.Hour, 24*time.Hour),
		empty,
		msg("latest", time.Minute, 0),
		msg("newer", 0, 0),
	}, now, func(m *bridgev2.BackfillMessage) bool { return m.ID == "known" })
	var ids []networkid.MessageID
	for _, m := range got {
		ids = append(ids, m.ID)
	}
	if len(ids) != 2 || ids[0] != "gap" || ids[1] != "timer-running" {
		t.Fatalf("got %v", ids)
	}
}

func TestArchiveRevokes(t *testing.T) {
	supplemental := func(app *waConsumerApplication.ConsumerApplication) *waArmadilloBackupMessage.BackupMessage {
		inner, err := proto.Marshal(app)
		if err != nil {
			t.Fatal(err)
		}
		outer, err := proto.Marshal(&waMsgApplication.MessageApplication{
			Payload: &waMsgApplication.MessageApplication_Payload{
				Content: &waMsgApplication.MessageApplication_Payload_SubProtocol{
					SubProtocol: &waMsgApplication.MessageApplication_SubProtocolPayload{
						SubProtocol: &waMsgApplication.MessageApplication_SubProtocolPayload_ConsumerMessage{
							ConsumerMessage: &waCommon.SubProtocol{Payload: inner},
						},
					},
				},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return &waArmadilloBackupMessage.BackupMessage{
			Payload: &waArmadilloBackupMessage.BackupMessage_EncryptedTransportMessage{EncryptedTransportMessage: outer},
		}
	}
	revoke := supplemental(wrapRevoke(&waConsumerApplication.ConsumerApplication_RevokeMessage{
		Key: &waCommon.MessageKey{ID: proto.String("m1")},
	}))
	if !archiveRevokes(revoke, "m1") {
		t.Error("unsend of this message not recognised")
	}
	if archiveRevokes(revoke, "m2") {
		t.Error("unsend of another message matched")
	}
	if archiveRevokes(supplemental(&waConsumerApplication.ConsumerApplication{}), "m1") {
		t.Error("an empty message counted as an unsend")
	}
}
