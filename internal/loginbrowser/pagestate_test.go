package loginbrowser

import (
	"strings"
	"testing"

	"maunium.net/go/mautrix/bridgev2"
)

// Messages from the Windows window's page scripts feed the same Collect
// logic as the Mac window's snapshots.
func TestPageStateFeedsCollect(t *testing.T) {
	discord := &bridgev2.LoginCookiesParams{
		URL: "https://discord.com/login",
		Fields: []bridgev2.LoginCookieField{{
			ID: "token", Required: true,
			Sources: []bridgev2.LoginCookieFieldSource{{Type: bridgev2.LoginCookieTypeSpecial, Name: "fi.mau.discord.token"}},
		}},
		ExtractJS: "Promise.resolve({token: 'x'})",
	}
	var page pageState
	if _, done := Collect(discord, page.snapshot("https://discord.com/login", nil)); done {
		t.Fatal("shouldn't be done before the script reports")
	}
	if err := page.apply(`{"kind":"extracted","value":{"token":"abc"}}`); err != nil {
		t.Fatal(err)
	}
	values, done := Collect(discord, page.snapshot("https://discord.com/channels/@me", nil))
	if !done || values["token"] != "abc" {
		t.Fatalf("got %v, done=%v", values, done)
	}

	stored := &bridgev2.LoginCookiesParams{Fields: []bridgev2.LoginCookieField{{
		ID: "session", Required: true,
		Sources: []bridgev2.LoginCookieFieldSource{{Type: bridgev2.LoginCookieTypeLocalStorage, Name: "sess"}},
	}}}
	if err := page.apply(`{"kind":"storage","value":{"sess":"s1"}}`); err != nil {
		t.Fatal(err)
	}
	if values, done = Collect(stored, page.snapshot("", nil)); !done || values["session"] != "s1" {
		t.Fatalf("got %v, done=%v", values, done)
	}

	for _, bad := range []string{`not json`, `{"kind":"other","value":{}}`, `{"kind":"extracted","value":"str"}`} {
		if err := page.apply(bad); err == nil {
			t.Errorf("%s: expected an error", bad)
		}
	}
}

func TestPageScripts(t *testing.T) {
	extract := extractScript("new Promise(r => r({a: 1}))")
	for _, want := range []string{"await (new Promise(r => r({a: 1})))", "window.top !== window", `kind: "extracted"`} {
		if !strings.Contains(extract, want) {
			t.Errorf("extract script missing %q", want)
		}
	}
	storage := storageScript([]string{"token", `we"ird`})
	if !strings.Contains(storage, `["token","we\"ird"]`) {
		t.Errorf("storage script should embed the keys as JSON: %s", storage)
	}
}
