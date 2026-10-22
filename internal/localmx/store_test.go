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
