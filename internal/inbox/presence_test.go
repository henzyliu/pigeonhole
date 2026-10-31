package inbox

import (
	"testing"

	"github.com/bwmarrin/discordgo"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
)

// Friends' statuses come from the raw gateway events: all of them on
// connecting, then one at a time. Server presence updates and activity-only
// changes are ignored, and the UI only hears about real status changes.
func TestDiscordPresenceEvents(t *testing.T) {
	refreshes := 0
	svc := &Service{Emit: func(name string, data any) { refreshes++ }}
	login := &bridgev2.UserLogin{UserLogin: &database.UserLogin{ID: "me1"}}
	send := func(typ, raw string) {
		svc.onDiscordEvent(login, &discordgo.Event{Type: typ, RawData: []byte(raw)})
	}
	status := func(userID string) string { return svc.presence["me1"][userID] }

	send("READY_SUPPLEMENTAL", `{"merged_presences":{"friends":[
		{"user_id":"1","status":"online","activities":[]},
		{"user_id":"2","status":"idle"}]}}`)
	if status("1") != "online" || status("2") != "idle" || refreshes != 1 {
		t.Fatalf("after connecting: 1=%q 2=%q refreshes=%d", status("1"), status("2"), refreshes)
	}

	send("PRESENCE_UPDATE", `{"user":{"id":"2"},"status":"dnd"}`)
	if status("2") != "dnd" || refreshes != 2 {
		t.Fatalf("after update: 2=%q refreshes=%d", status("2"), refreshes)
	}

	// A new song on the same status, and a server's presence update.
	send("PRESENCE_UPDATE", `{"user":{"id":"2"},"status":"dnd","activities":[{"name":"Spotify"}]}`)
	send("PRESENCE_UPDATE", `{"user":{"id":"1"},"status":"offline","guild_id":"99"}`)
	if status("1") != "online" || refreshes != 2 {
		t.Fatalf("ignored updates changed things: 1=%q refreshes=%d", status("1"), refreshes)
	}

	// Reconnecting replaces the list, dropping anyone no longer a friend.
	send("READY_SUPPLEMENTAL", `{"merged_presences":{"friends":[{"user_id":"1","status":"offline"}]}}`)
	if status("1") != "offline" || status("2") != "" || refreshes != 3 {
		t.Fatalf("after reconnecting: 1=%q 2=%q refreshes=%d", status("1"), status("2"), refreshes)
	}
}

// A DM uses its own account's view; a group (no account) uses any account
// that's friends with the person. Offline and unknown people show nothing.
func TestPresenceLookup(t *testing.T) {
	p := presenceByLogin{
		"a": {"1": "online", "2": "offline"},
		"b": {"2": "idle", "3": "invisible"},
	}
	for _, c := range []struct{ login, user, want string }{
		{"a", "1", "online"},
		{"a", "2", ""},
		{"b", "2", "idle"},
		{"a", "3", ""},
		{"", "2", "idle"},
		{"", "3", ""},
		{"", "4", ""},
		{"c", "1", ""},
	} {
		if got := p.of(c.login, c.user); got != c.want {
			t.Errorf("of(%q, %q) = %q, want %q", c.login, c.user, got, c.want)
		}
	}
}
