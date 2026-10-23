package localmx

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
