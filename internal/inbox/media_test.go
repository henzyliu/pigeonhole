package inbox

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func newPortal(t *testing.T, svc *Service, network string) (id.RoomID, func(body string, ts time.Time)) {
	t.Helper()
	ctx := context.Background()
	mx := svc.networks[network].MX
	ghost := mx.GhostIntent("999")
	if err := ghost.SetDisplayName(ctx, "Bob"); err != nil {
		t.Fatal(err)
	}
	stateKey := ""
	roomID, err := mx.BotIntent().CreateRoom(ctx, &mautrix.ReqCreateRoom{
		IsDirect: true,
		Invite:   []id.UserID{ghost.GetMXID(), Me},
		InitialState: []*event.Event{{
			Type: event.StateBridge, StateKey: &stateKey,
			Content: event.Content{Parsed: &event.BridgeEventContent{}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	send := func(body string, ts time.Time) {
		_, err := ghost.SendMessage(ctx, roomID, event.EventMessage, &event.Content{
			Parsed: &event.MessageEventContent{MsgType: event.MsgText, Body: body},
		}, &bridgev2.MatrixSendExtra{Timestamp: ts})
		if err != nil {
			t.Fatal(err)
		}
	}
	return roomID, send
}

func TestSendFile(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	roomID, _ := newPortal(t, svc, "messenger")

	path := filepath.Join(t.TempDir(), "photo.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, image.NewRGBA(image.Rect(0, 0, 40, 30))); err != nil {
		t.Fatal(err)
	}
	f.Close()

	if err = svc.SendFile(ctx, string(roomID), path); err != nil {
		t.Fatal(err)
	}
	msgs, err := svc.Messages(ctx, string(roomID), 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %+v", msgs)
	}
	m := msgs[0]
	if m.Kind != "image" || m.MimeType != "image/png" || m.Width != 40 || m.Height != 30 || !m.IsMe || m.MediaURL == "" {
		t.Fatalf("unexpected message: %+v", m)
	}
	// The stored media must be readable back, which is what the bridge does
	// before uploading to Meta.
	data, err := svc.networks["messenger"].MX.BotIntent().DownloadMedia(ctx, id.ContentURIString("mxc://local/"+m.MediaURL[len(mediaPrefix):]), nil)
	if err != nil || len(data) == 0 {
		t.Fatalf("failed to read stored media back: %v", err)
	}
	if chats, _ := svc.Chats(ctx); chats[0].Preview != "📷 Photo" {
		t.Fatalf("unexpected preview %q", chats[0].Preview)
	}

	if err = svc.SendFile(ctx, string(roomID), t.TempDir()); err == nil {
		t.Fatal("expected sending a folder to fail")
	}
}

func TestNotifications(t *testing.T) {
	svc := newTestService(t)
	got := make(chan Notification, 4)
	svc.OnNotify = func(n Notification) { got <- n }
	roomID, send := newPortal(t, svc, "instagram")

	send("old message", time.Now().Add(-time.Hour))
	send("hey!", time.Now())
	select {
	case n := <-got:
		if n.RoomID != string(roomID) || n.Title != "Bob" || n.Body != "hey!" {
			t.Fatalf("unexpected notification: %+v", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notification")
	}

	if err := svc.Store.Mute(context.Background(), roomID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	send("muted", time.Now())
	select {
	case n := <-got:
		t.Fatalf("unexpected notification: %+v", n)
	case <-time.After(300 * time.Millisecond):
	}
}
