package inbox

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"unified-inbox/internal/localmx"
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

// Link previews the network sent come through with the message; ones with
// nothing to show, or that aren't web links, are left out.
func TestLinkPreviews(t *testing.T) {
	m := &Message{}
	fillContent(m, "m.room.message", `{"msgtype":"m.text","body":"look https://www.example.com/a","com.beeper.linkpreviews":[
		{"matched_url":"https://www.example.com/a","og:title":" A page ","og:description":"About it","og:image":"mxc://local/pic","og:image:width":1200,"og:image:height":630},
		{"matched_url":"https://example.org/empty"},
		{"matched_url":"javascript:alert(1)","og:title":"Nope"}]}`)
	want := []LinkPreview{{URL: "https://www.example.com/a", Site: "example.com", Title: "A page", Description: "About it",
		Image: "/media/pic", Width: 1200, Height: 630}}
	if !slices.Equal(m.Previews, want) {
		t.Fatalf("previews: %+v", m.Previews)
	}
}

// A change the bridge can't make (here: no account signed in) comes back
// as failed, leaving the message as it was. Your own messages can't be
// changed in a chat that doesn't allow it, and others' never can.
func TestEditRefused(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	roomID, send := newPortal(t, svc, "whatsapp")
	send("from bob", time.Now())
	mine := &event.Event{ID: "$mine", RoomID: roomID, Sender: Me, Type: event.EventMessage, Timestamp: time.Now().UnixMilli(),
		Content: event.Content{Parsed: &event.MessageEventContent{MsgType: event.MsgText, Body: "helo"}}}
	if err := svc.Store.HandleEvent(ctx, mine, false); err != nil {
		t.Fatal(err)
	}
	msgs, err := svc.Messages(ctx, string(roomID), 0, 10)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("messages: %+v, %v", msgs, err)
	}
	if msgs[0].CanEdit || msgs[0].CanUnsend || msgs[1].CanEdit {
		t.Fatalf("nothing should be editable without a signed-in account: %+v", msgs)
	}
	if err = svc.EditMessage(ctx, string(roomID), string(msgs[0].EventID), "hi"); err == nil {
		t.Fatal("editing someone else's message should fail")
	}
	if err = svc.EditMessage(ctx, string(roomID), "$mine", "hello"); err == nil {
		t.Fatal("editing where the chat doesn't allow it should fail")
	}

	// Skip the checks to see what the bridge itself says.
	err = svc.sendChange(ctx, string(roomID), "$mine", localmx.ChangeEdit, []byte(`{"msgtype":"m.text","body":"hello"}`),
		event.EventMessage, &event.MessageEventContent{MsgType: event.MsgText, Body: "* hello",
			NewContent: &event.MessageEventContent{MsgType: event.MsgText, Body: "hello"},
			RelatesTo:  &event.RelatesTo{Type: event.RelReplace, EventID: "$mine"}})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		msgs, _ = svc.Messages(ctx, string(roomID), 0, 10)
		if m := msgs[1]; m.Pending == "" {
			if m.Body != "helo" || m.Edited || m.ChangeFailed != localmx.ChangeEdit {
				t.Fatalf("refused edit should leave the message: %+v", m)
			}
			break
		} else if m.Body != "hello" {
			t.Fatalf("a pending edit should show its new text: %+v", m)
		}
		if time.Now().After(deadline) {
			t.Fatal("the bridge never answered the edit")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Voice notes labelled audio/opus are served as Ogg, which the webview
// plays; other types are left alone.
func TestPlayableMime(t *testing.T) {
	for in, want := range map[string]string{
		"audio/opus":             "audio/ogg; codecs=opus",
		"audio/ogg; codecs=opus": "audio/ogg; codecs=opus",
		"audio/mp4":              "audio/mp4",
		"image/jpeg":             "image/jpeg",
	} {
		if got := playableMime(in); got != want {
			t.Errorf("playableMime(%q) = %q, want %q", in, got, want)
		}
	}
}

// A failed message of yours can be sent again: Try again sends it as a new
// message and removes the failed one, and one that failed in a retriable
// way (e.g. sent while reconnecting) is retried once by itself.
func TestRetrySend(t *testing.T) {
	autoRetryDelay = 100 * time.Millisecond
	svc := newTestService(t)
	ctx := context.Background()
	roomID, send := newPortal(t, svc, "whatsapp")
	send("from bob", time.Now())
	failed := func(evtID id.EventID, body string, status string) {
		t.Helper()
		evt := &event.Event{ID: evtID, RoomID: roomID, Sender: Me, Type: event.EventMessage, Timestamp: time.Now().UnixMilli(),
			Content: event.Content{Parsed: &event.MessageEventContent{MsgType: event.MsgText, Body: body}}}
		if err := svc.Store.HandleEvent(ctx, evt, false); err != nil {
			t.Fatal(err)
		}
		svc.Store.SetEventStatus(ctx, evtID, roomID, status)
	}
	mine := func() (out []string) {
		msgs, _ := svc.Messages(ctx, string(roomID), 0, 20)
		for _, m := range msgs {
			if m.IsMe {
				out = append(out, m.EventID+"="+m.Body)
			}
		}
		return
	}
	has := func(match func(string) bool) bool { return slices.ContainsFunc(mine(), match) }
	withID := func(evtID string) func(string) bool {
		return func(b string) bool { return strings.HasPrefix(b, evtID+"=") }
	}
	withBody := func(body string) func(string) bool {
		return func(b string) bool { return strings.HasSuffix(b, "="+body) }
	}

	failed("$perm", "nice", localmx.StatusFailPermanent)
	if err := svc.RetrySend(ctx, string(roomID), "$perm"); err != nil {
		t.Fatal(err)
	}
	if has(withID("$perm")) || !has(withBody("nice")) {
		t.Fatalf("Try again should replace the failed message: %v", mine())
	}
	if err := svc.RetrySend(ctx, string(roomID), "$perm"); err == nil {
		t.Fatal("a message that's gone can't be retried")
	}

	failed("$retry", "noice", localmx.StatusFailRetriable)
	deadline := time.Now().Add(5 * time.Second)
	for has(withID("$retry")) {
		if time.Now().After(deadline) {
			t.Fatalf("a retriable failure should be retried by itself: %v", mine())
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !has(withBody("noice")) {
		t.Fatalf("the retried message should be there: %v", mine())
	}
}

// Search finds text and captions (not file names), ignores case and LIKE
// wildcards, skips deleted messages, and fills in the chat; jumping to a
// result loads the chat back to it.
func TestSearchMessages(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	roomID, send := newPortal(t, svc, "whatsapp")
	start := time.Now().Add(-time.Hour)
	send("Dinner at 7?", start)
	for i := 0; i < 30; i++ {
		send(fmt.Sprintf("filler %d", i), start.Add(time.Duration(i+1)*time.Minute))
	}
	send("100% sure about dinner", start.Add(40*time.Minute))
	send("100 sure", start.Add(41*time.Minute))
	got, err := svc.SearchMessages(ctx, "DINNER", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Body != "100% sure about dinner" || got[1].Body != "Dinner at 7?" {
		t.Fatalf("results (newest first): %+v", got)
	}
	if r := got[0]; r.RoomID != string(roomID) || r.ChatName != "Bob" || r.SenderName != "Bob" || r.IsMe || r.Network != "whatsapp" {
		t.Fatalf("result details: %+v", r)
	}
	if got, _ = svc.SearchMessages(ctx, "100%", 0); len(got) != 1 {
		t.Fatalf("%% should be matched literally: %+v", got)
	}
	if got, _ = svc.SearchMessages(ctx, "d", 0); len(got) != 0 {
		t.Fatalf("one letter shouldn't search: %+v", got)
	}

	msgs, err := svc.MessagesThrough(ctx, string(roomID), got0(t, svc, "Dinner at 7?"))
	if err != nil || len(msgs) == 0 || msgs[0].Body != "Dinner at 7?" {
		t.Fatalf("jumping should load back to the result: %d messages, first %q, %v", len(msgs), first(msgs), err)
	}
}

func got0(t *testing.T, svc *Service, body string) string {
	t.Helper()
	r, err := svc.SearchMessages(context.Background(), body, 0)
	if err != nil || len(r) == 0 {
		t.Fatalf("no result for %q: %v", body, err)
	}
	return r[0].EventID
}

func first(msgs []Message) string {
	if len(msgs) == 0 {
		return ""
	}
	return msgs[0].Body
}

// Searching one chat only finds that chat's messages, newest first.
func TestSearchChat(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	roomA, sendA := newPortal(t, svc, "whatsapp")
	_, sendB := newPortal(t, svc, "messenger")
	start := time.Now().Add(-time.Hour)
	sendA("pizza tonight?", start)
	sendB("pizza in the other chat", start.Add(time.Minute))
	sendA("no", start.Add(2*time.Minute))
	sendA("ok PIZZA then", start.Add(3*time.Minute))
	ids, err := svc.SearchChat(ctx, string(roomA), "pizza")
	if err != nil || len(ids) != 2 {
		t.Fatalf("matches: %v, %v", ids, err)
	}
	msgs, _ := svc.Messages(ctx, string(roomA), 0, 10)
	body := map[string]string{}
	for _, m := range msgs {
		body[m.EventID] = m.Body
	}
	if body[ids[0]] != "ok PIZZA then" || body[ids[1]] != "pizza tonight?" {
		t.Fatalf("expected this chat's matches, newest first: %q, %q", body[ids[0]], body[ids[1]])
	}
}

// The Shared panel lists each kind separately, newest first.
func TestChatShared(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	roomID, send := newPortal(t, svc, "whatsapp")
	start := time.Now().Add(-time.Hour)
	send("see https://example.com/a", start)
	send("just text", start.Add(time.Minute))
	for i, c := range []*event.MessageEventContent{
		{MsgType: event.MsgImage, Body: "photo.jpg", URL: "mxc://local/p1"},
		{MsgType: event.MsgVideo, Body: "clip.mp4", URL: "mxc://local/v1"},
		{MsgType: event.MsgFile, Body: "notes.pdf", FileName: "notes.pdf", URL: "mxc://local/f1"},
	} {
		evt := &event.Event{ID: id.EventID(fmt.Sprintf("$s%d", i)), RoomID: roomID, Sender: Me, Type: event.EventMessage,
			Timestamp: start.Add(time.Duration(i+2) * time.Minute).UnixMilli(), Content: event.Content{Parsed: c}}
		if err := svc.Store.HandleEvent(ctx, evt, false); err != nil {
			t.Fatal(err)
		}
	}
	kinds := func(kind string) (out []string) {
		t.Helper()
		items, err := svc.ChatShared(ctx, string(roomID), kind, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range items {
			out = append(out, m.Kind+":"+m.FileName+m.Body)
		}
		return
	}
	if got := kinds("media"); len(got) != 2 || got[0] != "video:clip.mp4" || got[1] != "image:photo.jpg" {
		t.Fatalf("media: %v", got)
	}
	if got := kinds("files"); len(got) != 1 || got[0] != "file:notes.pdf" {
		t.Fatalf("files: %v", got)
	}
	if got := kinds("links"); len(got) != 1 || got[0] != "text:see https://example.com/a" {
		t.Fatalf("links: %v", got)
	}
	if _, err := svc.ChatShared(ctx, string(roomID), "nope", 0); err == nil {
		t.Fatal("unknown kinds should be refused")
	}
}

// GIFs go to WhatsApp as an MP4 marked as a GIF, so it plays as a looping,
// silent GIF there rather than as a video.
func TestSendGIFVideo(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	roomID, _ := newPortal(t, svc, "whatsapp")
	if err := svc.SendGIFVideo(ctx, string(roomID), []byte("pretend mp4"), 480, 270); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := svc.Store.DB.QueryRow("SELECT content FROM events WHERE room_id=? AND sender=?", roomID, Me).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var c event.MessageEventContent
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	if c.MsgType != event.MsgVideo || c.Info == nil || !c.Info.MauGIF || c.Info.MimeType != "video/mp4" ||
		c.Info.Width != 480 || c.Info.Height != 270 || c.URL == "" {
		t.Fatalf("expected a GIF-marked MP4: %s", raw)
	}
	if network, err := svc.ChatNetwork(ctx, string(roomID)); err != nil || network != "whatsapp" {
		t.Fatalf("chat network: %q, %v", network, err)
	}
}

// Picked mentions become links to the person in the message's HTML (which
// the bridges turn into real mentions), in order and escaped; text that no
// longer has the "@Name" stays plain.
func TestWithMentions(t *testing.T) {
	c := &event.MessageEventContent{MsgType: event.MsgText, Body: "hey @Sam & @Jo Lee\n<3 @Sam"}
	withMentions(c, []Mention{
		{UserID: "@whatsapp_1:local", Name: "Sam"},
		{UserID: "@whatsapp_2:local", Name: "Jo Lee"},
		{UserID: "@whatsapp_1:local", Name: "Sam"},
		{UserID: "@whatsapp_3:local", Name: "Gone"},
	})
	want := `hey <a href="https://matrix.to/#/@whatsapp_1:local">Sam</a> &amp; <a href="https://matrix.to/#/@whatsapp_2:local">Jo Lee</a><br>&lt;3 <a href="https://matrix.to/#/@whatsapp_1:local">Sam</a>`
	if c.Format != event.FormatHTML || c.FormattedBody != want {
		t.Fatalf("formatted body:\n got %s\nwant %s", c.FormattedBody, want)
	}
	if c.Mentions == nil || !slices.Equal(c.Mentions.UserIDs, []id.UserID{"@whatsapp_1:local", "@whatsapp_2:local"}) {
		t.Fatalf("mentions: %+v", c.Mentions)
	}
	plain := &event.MessageEventContent{MsgType: event.MsgText, Body: "no one here"}
	withMentions(plain, []Mention{{UserID: "@whatsapp_1:local", Name: "Sam"}})
	if plain.Format != "" || plain.FormattedBody != "" || plain.Mentions != nil {
		t.Fatalf("a message without mentions should stay plain: %+v", plain)
	}
}

// Members are the other people in the chat, with names, not you.
func TestChatMembers(t *testing.T) {
	svc := newTestService(t)
	roomID, _ := newPortal(t, svc, "whatsapp")
	members, err := svc.ChatMembers(context.Background(), string(roomID))
	if err != nil || len(members) != 1 || members[0].Name != "Bob" {
		t.Fatalf("members: %+v, %v", members, err)
	}
}
