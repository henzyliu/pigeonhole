package localmx

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Media must survive the data folder moving, e.g. when the app was renamed
// from Unified Inbox to Pigeonhole. Rows from before the fix stored absolute
// paths into the old folder, so they're repaired when the store opens.
func TestMediaSurvivesDataFolderMove(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	oldDir, newDir := filepath.Join(base, "UnifiedInbox"), filepath.Join(base, "Pigeonhole")

	store, err := OpenStore(oldDir, "@me:local")
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(store.MediaDir, "legacy-avatar")
	current := filepath.Join(store.MediaDir, "new-avatar")
	for _, f := range []string{legacy, current} {
		if err = os.WriteFile(f, []byte("jpeg"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A row written the old way, with an absolute path.
	if _, err = store.DB.Exec("INSERT INTO media (mxc, path, mime) VALUES (?, ?, ?)", "mxc://local/legacy-avatar", legacy, "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	if err = store.SaveMedia(ctx, "mxc://local/new-avatar", current, "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	store.DB.Close()

	if err = os.Rename(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	store, err = OpenStore(newDir, "@me:local")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	for _, mxc := range []id.ContentURIString{"mxc://local/legacy-avatar", "mxc://local/new-avatar"} {
		path, mime, err := store.MediaPath(ctx, mxc)
		if err != nil {
			t.Fatalf("%s: %v", mxc, err)
		}
		if filepath.Dir(path) != filepath.Join(newDir, "media") || mime != "image/jpeg" {
			t.Fatalf("%s resolved to %s (%s)", mxc, path, mime)
		}
		if _, err = os.Stat(path); err != nil {
			t.Fatalf("%s: file not found after move: %v", mxc, err)
		}
	}
}

// Deleting a chat removes its messages, receipts and media files, but keeps
// media still used elsewhere and never touches files nothing refers to yet
// (e.g. an upload in progress for another chat).
func TestDeleteRoomCleansUp(t *testing.T) {
	ctx := context.Background()
	store, err := OpenStore(t.TempDir(), "@me:local")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	media := func(name string) id.ContentURIString {
		path := filepath.Join(store.MediaDir, name)
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		mxc := id.ContentURIString("mxc://local/" + name)
		if err := store.SaveMedia(ctx, mxc, path, "image/jpeg"); err != nil {
			t.Fatal(err)
		}
		return mxc
	}
	onlyHere, shared, avatar, uploading := media("only-here"), media("shared"), media("avatar"), media("uploading")
	exec := func(q string, args ...any) {
		if _, err := store.DB.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO rooms (room_id, network, avatar) VALUES ('!a:local', 'instagram', ?), ('!b:local', 'messenger', '')", avatar)
	exec("INSERT INTO profiles (user_id, avatar) VALUES ('@friend:local', ?)", avatar)
	exec(`INSERT INTO events (event_id, room_id, sender, type, content, ts) VALUES
		('$1', '!a:local', '@x:local', 'm.room.message', ?, 1),
		('$2', '!a:local', '@x:local', 'm.room.message', ?, 2),
		('$3', '!b:local', '@x:local', 'm.room.message', ?, 3)`,
		`{"msgtype":"m.image","url":"`+string(onlyHere)+`"}`,
		`{"msgtype":"m.image","url":"`+string(shared)+`"}`,
		`{"msgtype":"m.image","url":"`+string(shared)+`"}`)
	exec("INSERT INTO receipts (room_id, user_id, event_id, ts) VALUES ('!a:local', '@x:local', '$2', 1)")

	if err = store.DeleteRoom(ctx, "!a:local"); err != nil {
		t.Fatal(err)
	}
	count := func(q string) (n int) {
		if err := store.DB.QueryRow(q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return
	}
	if n := count("SELECT count(*) FROM events WHERE room_id='!a:local'") + count("SELECT count(*) FROM receipts") + count("SELECT count(*) FROM rooms WHERE room_id='!a:local'"); n != 0 {
		t.Fatalf("expected the room's rows to be gone, %d left", n)
	}
	exists := func(mxc id.ContentURIString) bool {
		_, err := os.Stat(filepath.Join(store.MediaDir, filepath.Base(string(mxc))))
		return err == nil
	}
	if exists(onlyHere) {
		t.Error("media only used by the deleted room should be removed")
	}
	for _, kept := range []id.ContentURIString{shared, avatar, uploading} {
		if !exists(kept) {
			t.Errorf("%s should have been kept", kept)
		}
	}
}

// A hidden chat stays hidden through older history loading in, and comes
// back by itself when a new message arrives.
func TestHiddenChatReturnsOnNewMessage(t *testing.T) {
	ctx := context.Background()
	store, err := OpenStore(t.TempDir(), "@me:local")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if err = store.CreateRoom(ctx, "!a:local", "discord", true, "", ""); err != nil {
		t.Fatal(err)
	}
	send := func(eventID id.EventID, ts int64) {
		evt := &event.Event{ID: eventID, RoomID: "!a:local", Sender: "@friend:local", Type: event.EventMessage, Timestamp: ts,
			Content: event.Content{Parsed: &event.MessageEventContent{MsgType: event.MsgText, Body: "hi"}}}
		if err := store.HandleEvent(ctx, evt, true); err != nil {
			t.Fatal(err)
		}
	}
	hidden := func() (h bool) {
		if err := store.DB.QueryRow("SELECT " + HiddenClause + " FROM rooms WHERE room_id='!a:local'").Scan(&h); err != nil {
			t.Fatal(err)
		}
		return
	}

	send("$1", 100)
	if err = store.SetHidden(ctx, "!a:local", true); err != nil {
		t.Fatal(err)
	} else if !hidden() {
		t.Fatal("chat should be hidden")
	}
	send("$0", 50) // older history
	if !hidden() {
		t.Fatal("older history shouldn't unhide the chat")
	}
	send("$2", 200)
	if hidden() {
		t.Fatal("a new message should unhide the chat")
	}
	if err = store.SetHidden(ctx, "!a:local", true); err != nil {
		t.Fatal(err)
	} else if err = store.SetHidden(ctx, "!a:local", false); err != nil {
		t.Fatal(err)
	} else if hidden() {
		t.Fatal("unhiding should show the chat")
	}
}

// Databases from before hiding existed get the column added on open.
func TestHiddenColumnAddedToOldDatabase(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(dir, "@me:local")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec("ALTER TABLE rooms DROP COLUMN hidden_ts"); err != nil {
		t.Fatal(err)
	}
	store.DB.Close()
	if store, err = OpenStore(dir, "@me:local"); err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if _, err = store.DB.Exec("SELECT hidden_ts FROM rooms"); err != nil {
		t.Fatal(err)
	}
}

// The chat-list preview follows edits and deletions of the latest message,
// and previews saved before that are repaired when the store opens.
func TestPreviewFollowsEditsAndDeletions(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := OpenStore(dir, "@me:local")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.CreateRoom(ctx, "!a:local", "messenger", true, "", ""); err != nil {
		t.Fatal(err)
	}
	handle := func(evt *event.Event) {
		t.Helper()
		evt.RoomID = "!a:local"
		if evt.Sender == "" {
			evt.Sender = "@me:local"
		}
		if err := store.HandleEvent(ctx, evt, false); err != nil {
			t.Fatal(err)
		}
	}
	text := func(eventID id.EventID, ts int64, body string) {
		handle(&event.Event{ID: eventID, Type: event.EventMessage, Timestamp: ts,
			Content: event.Content{Parsed: &event.MessageEventContent{MsgType: event.MsgText, Body: body}}})
	}
	edit := func(eventID, target id.EventID, ts int64, body string) {
		handle(&event.Event{ID: eventID, Type: event.EventMessage, Timestamp: ts,
			Content: event.Content{Parsed: &event.MessageEventContent{
				MsgType: event.MsgText, Body: "* " + body,
				NewContent: &event.MessageEventContent{MsgType: event.MsgText, Body: body},
				RelatesTo:  &event.RelatesTo{Type: event.RelReplace, EventID: target},
			}}})
	}
	preview := func() (p string) {
		t.Helper()
		if err := store.DB.QueryRow("SELECT preview FROM rooms WHERE room_id='!a:local'").Scan(&p); err != nil {
			t.Fatal(err)
		}
		return
	}

	text("$old", 100, "first")
	text("$new", 200, "its been a whILE")
	edit("$e1", "$new", 300, "its been a while")
	if p := preview(); p != "its been a while" {
		t.Fatalf("editing the latest message should update the preview, got %q", p)
	}
	edit("$e2", "$old", 400, "first, edited")
	if p := preview(); p != "its been a while" {
		t.Fatalf("editing an older message shouldn't change the preview, got %q", p)
	}
	handle(&event.Event{ID: "$r1", Type: event.EventRedaction, Timestamp: 500, Redacts: "$new",
		Content: event.Content{Parsed: &event.RedactionEventContent{Redacts: "$new"}}})
	if p := preview(); p != DeletedPreview {
		t.Fatalf("deleting the latest message should show %q, got %q", DeletedPreview, p)
	}

	// A preview saved before this existed is repaired on open.
	if _, err = store.DB.Exec("UPDATE rooms SET preview='stale' WHERE room_id='!a:local'"); err != nil {
		t.Fatal(err)
	}
	store.DB.Close()
	if store, err = OpenStore(dir, "@me:local"); err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if p := preview(); p != DeletedPreview {
		t.Fatalf("expected the stale preview to be repaired, got %q", p)
	}
}

// Drafts are saved per chat, clearing the text clears the reply too, and
// saving the same draft again doesn't notify.
func TestDrafts(t *testing.T) {
	ctx := context.Background()
	store, err := OpenStore(t.TempDir(), "@me:local")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if err = store.CreateRoom(ctx, "!a:local", "messenger", true, "", ""); err != nil {
		t.Fatal(err)
	}
	notified := 0
	store.Notify = func(Change) { notified++ }
	get := func() (text, reply string) {
		t.Helper()
		if err := store.DB.QueryRow("SELECT draft, draft_reply FROM rooms WHERE room_id='!a:local'").Scan(&text, &reply); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err = store.SetDraft(ctx, "!a:local", "see you at", "$q"); err != nil {
		t.Fatal(err)
	}
	if text, reply := get(); text != "see you at" || reply != "$q" || notified != 1 {
		t.Fatalf("draft %q reply %q, %d notifications", text, reply, notified)
	}
	_ = store.SetDraft(ctx, "!a:local", "see you at", "$q")
	if notified != 1 {
		t.Fatalf("an unchanged draft shouldn't notify (%d)", notified)
	}
	_ = store.SetDraft(ctx, "!a:local", "", "$q")
	if text, reply := get(); text != "" || reply != "" || notified != 2 {
		t.Fatalf("clearing: draft %q reply %q, %d notifications", text, reply, notified)
	}
}

// Typing shows until it's stopped, runs out, or the person's message
// arrives; the local user's own typing isn't listed.
func TestTyping(t *testing.T) {
	ctx := context.Background()
	store, err := OpenStore(t.TempDir(), "@me:local")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	if err = store.CreateRoom(ctx, "!a:local", "messenger", true, "", ""); err != nil {
		t.Fatal(err)
	}
	changes := make(chan Change, 10)
	store.Notify = func(c Change) {
		if c.Kind == "typing" {
			changes <- c
		}
	}
	typing := func() []id.UserID { return store.Typing()["!a:local"] }

	store.SetTyping("!a:local", "@me:local", time.Minute)
	if len(typing()) != 0 {
		t.Fatal("your own typing shouldn't show")
	}
	store.SetTyping("!a:local", "@sam:local", time.Minute)
	if got := typing(); len(got) != 1 || got[0] != "@sam:local" {
		t.Fatalf("typing: %v", got)
	}
	<-changes
	evt := &event.Event{ID: "$1", RoomID: "!a:local", Sender: "@sam:local", Type: event.EventMessage, Timestamp: 1,
		Content: event.Content{Parsed: &event.MessageEventContent{MsgType: event.MsgText, Body: "hi"}}}
	if err = store.insertMessage(ctx, evt, evt.Content.AsMessage(), false); err != nil {
		t.Fatal(err)
	}
	if len(typing()) != 0 {
		t.Fatal("sending a message should stop typing")
	}
	<-changes

	store.SetTyping("!a:local", "@sam:local", 50*time.Millisecond)
	<-changes
	select {
	case <-changes:
	case <-time.After(2 * time.Second):
		t.Fatal("typing should run out")
	}
	if len(typing()) != 0 {
		t.Fatal("typing should have run out")
	}
}
