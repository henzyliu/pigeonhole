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
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
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
		if n.RoomID != string(roomID) || n.Title != "Bob" || n.Body != "hey!" || n.Network != "instagram" {
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

// Categories come from the bridge's own portal records: message requests and
// Marketplace threads (which sit under a parent folder) get their own tabs,
// the rest split into one-to-one and group chats, and the folder is hidden.
func TestChatCategories(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	br := svc.networks["messenger"].Bridge
	makePortal := func(key string, edit func(p *bridgev2.Portal)) id.RoomID {
		roomID, _ := newPortal(t, svc, "messenger")
		portal, err := br.GetPortalByKey(ctx, networkid.PortalKey{ID: networkid.PortalID(key)})
		if err != nil {
			t.Fatal(err)
		}
		portal.MXID = roomID
		edit(portal)
		if err = portal.Save(ctx); err != nil {
			t.Fatal(err)
		}
		return roomID
	}
	makePortal("folder", func(p *bridgev2.Portal) { p.RoomType = database.RoomTypeSpace })
	inbox := makePortal("friend", func(p *bridgev2.Portal) { p.RoomType = database.RoomTypeDM })
	group := makePortal("book-club", func(p *bridgev2.Portal) {})
	request := makePortal("stranger", func(p *bridgev2.Portal) { p.MessageRequest = true })
	market := makePortal("seller", func(p *bridgev2.Portal) { p.ParentKey = networkid.PortalKey{ID: "folder"} })
	// Older Marketplace chats arrive as plain groups outside the folder; the
	// name and size give them away. A big group with that name pattern doesn't.
	oldMarket := makePortal("old-seller", func(p *bridgev2.Portal) {})
	dotGroup := makePortal("dot-group", func(p *bridgev2.Portal) {})
	exec := func(q string, args ...any) {
		if _, err := svc.Store.DB.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("UPDATE rooms SET name=?, is_dm=0 WHERE room_id=?", "Singh · 2017 BMW 330e", oldMarket)
	exec("UPDATE rooms SET name=?, is_dm=0 WHERE room_id=?", "Uni · Flatmates", dotGroup)
	for _, user := range []string{"@a:local", "@b:local", "@c:local"} {
		exec("INSERT INTO members (room_id, user_id, membership) VALUES (?, ?, 'join')", dotGroup, user)
	}

	chats, err := svc.Chats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[id.RoomID]string{}
	for _, c := range chats {
		got[id.RoomID(c.RoomID)] = c.Category
	}
	want := map[id.RoomID]string{
		inbox: CategoryInbox, group: CategoryGroups, request: CategoryRequests, market: CategoryMarketplace,
		oldMarket: CategoryMarketplace, dotGroup: CategoryGroups,
	}
	if len(got) != len(want) {
		t.Fatalf("expected the folder to be hidden, got %v", got)
	}
	for roomID, category := range want {
		if got[roomID] != category {
			t.Errorf("%s: got category %q, want %q", roomID, got[roomID], category)
		}
	}
}

// Opening someone you already have a chat with must not call Meta (which
// would bump the chat on other devices); it just returns the existing room.
func TestOpenChatReusesExistingDM(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	br := svc.networks["messenger"].Bridge
	roomID, _ := newPortal(t, svc, "messenger")
	portal, err := br.GetPortalByKey(ctx, networkid.PortalKey{ID: "dm-with-friend"})
	if err != nil {
		t.Fatal(err)
	}
	portal.MXID = roomID
	portal.RoomType = database.RoomTypeDM
	portal.OtherUserID = "friend-id"
	if err = portal.Save(ctx); err != nil {
		t.Fatal(err)
	}
	// No login exists in the test, so reaching Meta would fail.
	got, err := svc.OpenChat(ctx, "messenger", "no-such-login", "friend-id")
	if err != nil || got != string(roomID) {
		t.Fatalf("OpenChat = %q, %v; want %q", got, err, roomID)
	}
	if _, err = svc.OpenChat(ctx, "messenger", "no-such-login", "stranger-id"); err == nil {
		t.Fatal("expected opening a new chat without a login to fail")
	}
	if people, err := svc.SearchPeople(ctx, "a"); err != nil || people != nil {
		t.Fatalf("one-letter search should be a no-op, got %v, %v", people, err)
	}
}

func TestMediaFileName(t *testing.T) {
	cases := []struct{ name, mime, want string }{
		{"beach.jpg", "image/jpeg", "beach.jpg"},
		{"", "image/jpeg", "abc123.jpg"},
		{"IMG_0001", "image/png", "IMG_0001.png"},
		{"../../etc/passwd", "text/plain", "passwd.txt"},
		{`C:\Users\x\photo`, "image/webp", "photo.webp"},
		{"", "application/x-unknown", "abc123"},
	}
	for _, c := range cases {
		if got := MediaFileName(c.name, c.mime, "abc123"); got != c.want {
			t.Errorf("MediaFileName(%q, %q) = %q, want %q", c.name, c.mime, got, c.want)
		}
	}
}

// Opening or saving a photo copies the stored file out with a usable name,
// and refuses anything that isn't a media link.
func TestMediaCopy(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	src := filepath.Join(svc.Store.MediaDir, "pic1")
	if err := os.WriteFile(src, []byte("jpeg"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := svc.Store.SaveMedia(ctx, "mxc://local/pic1", src, "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	path, err := svc.MediaCopy(ctx, MediaURL("mxc://local/pic1"), "", out)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); filepath.Base(path) != "pic1.jpg" || string(data) != "jpeg" {
		t.Fatalf("copied to %s with %q", path, data)
	}
	for _, bad := range []string{"/media/../inbox.db", "/etc/passwd", "/media/", "/media/missing"} {
		if _, err = svc.MediaCopy(ctx, bad, "", out); err == nil {
			t.Errorf("%s: expected an error", bad)
		}
	}
}

// A reel from history is stored as a thumbnail the UI offers to play; the
// bridge's fetch arrives as an edit that turns it into the video without
// marking it edited.
func TestFetchedReelReplacesThumbnail(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	roomID, _ := newPortal(t, svc, "instagram")
	ghost := svc.networks["instagram"].MX.GhostIntent("999")
	resp, err := ghost.SendMessage(ctx, roomID, event.EventMessage, &event.Content{
		Parsed: &event.MessageEventContent{MsgType: event.MsgImage, Body: "funny dog", URL: "mxc://local/thumb"},
		Raw: map[string]any{
			"external_url":     "https://www.instagram.com/reel/abc123/",
			unresolvedMediaKey: map[string]any{"kind": "permanent"},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	message := func() Message {
		t.Helper()
		msgs, err := svc.Messages(ctx, string(roomID), 0, 10)
		if err != nil || len(msgs) != 1 {
			t.Fatalf("messages: %+v, %v", msgs, err)
		}
		return msgs[0]
	}
	if m := message(); m.Fetchable != "reel" || m.Kind != "image" {
		t.Fatalf("expected a fetchable reel thumbnail: %+v", m)
	}

	video := &event.MessageEventContent{MsgType: event.MsgVideo, Body: "funny dog", URL: "mxc://local/video"}
	_, err = ghost.SendMessage(ctx, roomID, event.EventMessage, &event.Content{
		Parsed: &event.MessageEventContent{
			MsgType: event.MsgVideo, Body: "* funny dog", NewContent: video,
			RelatesTo: &event.RelatesTo{Type: event.RelReplace, EventID: resp.EventID},
		},
		Raw: map[string]any{"com.beeper.dont_render_edited": true},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m := message(); m.Kind != "video" || m.Fetchable != "" || m.Edited || m.MediaURL != MediaURL("mxc://local/video") {
		t.Fatalf("expected the reel's video, not marked edited: %+v", m)
	}
	// Asking again once it's fetched does nothing.
	if err = svc.ResolveMedia(ctx, string(roomID), string(resp.EventID)); err != nil {
		t.Fatal(err)
	}
}

// Muting a chat in Pigeonhole stops its alerts and takes it out of the
// app's unread badge, while it still shows its own unread count.
func TestMutedChat(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	got := make(chan Notification, 4)
	svc.OnNotify = func(n Notification) { got <- n }
	roomID, send := newPortal(t, svc, "whatsapp")
	if err := svc.SetMuted(ctx, string(roomID), true); err != nil {
		t.Fatal(err)
	}
	send("you there?", time.Now())
	select {
	case n := <-got:
		t.Fatalf("muted chat shouldn't alert: %+v", n)
	case <-time.After(300 * time.Millisecond):
	}
	chats, err := svc.Chats(ctx)
	if err != nil || len(chats) != 1 {
		t.Fatalf("chats: %+v, %v", chats, err)
	}
	if c := chats[0]; !c.Muted || c.MutedOnNetwork || c.Unread != 1 {
		t.Fatalf("expected a muted chat that's still unread: %+v", c)
	}
	if n, err := svc.UnreadChats(ctx); err != nil || n != 0 {
		t.Fatalf("muted chats shouldn't count toward the badge: %d, %v", n, err)
	}

	if err = svc.SetMuted(ctx, string(roomID), false); err != nil {
		t.Fatal(err)
	}
	if n, _ := svc.UnreadChats(ctx); n != 1 {
		t.Fatalf("unmuted chat should count again, got %d", n)
	}
	send("hello?", time.Now())
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("unmuted chat should alert")
	}
}

// A chat's draft comes back with the chat list, and sending clears it in
// the UI by saving it empty.
func TestDraft(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	roomID, send := newPortal(t, svc, "whatsapp")
	send("lunch?", time.Now())
	if err := svc.SetDraft(ctx, string(roomID), "sure, where", "$asked"); err != nil {
		t.Fatal(err)
	}
	chats, err := svc.Chats(ctx)
	if err != nil || len(chats) != 1 {
		t.Fatalf("chats: %+v, %v", chats, err)
	}
	if c := chats[0]; c.Draft != "sure, where" || c.DraftReply != "$asked" || c.Preview != "lunch?" {
		t.Fatalf("expected the draft alongside the preview: %+v", c)
	}
	if err = svc.SetDraft(ctx, string(roomID), "", ""); err != nil {
		t.Fatal(err)
	}
	if chats, _ = svc.Chats(ctx); chats[0].Draft != "" || chats[0].DraftReply != "" {
		t.Fatalf("draft should be cleared: %+v", chats[0])
	}
}
