package loginbrowser

import (
	"testing"

	"github.com/rs/zerolog"

	"maunium.net/go/mautrix/bridgev2"
)

func cookieField(id, domain string, required bool) bridgev2.LoginCookieField {
	return bridgev2.LoginCookieField{ID: id, Required: required, Sources: []bridgev2.LoginCookieFieldSource{
		{Type: bridgev2.LoginCookieTypeCookie, Name: id, CookieDomain: domain},
	}}
}

// The messenger.com cookie step: done once all required cookies exist for
// the right domain AND the page has reached the inbox.
func TestCollectMessengerCookies(t *testing.T) {
	p := &bridgev2.LoginCookiesParams{
		Fields: []bridgev2.LoginCookieField{
			cookieField("xs", "messenger.com", true),
			cookieField("c_user", "messenger.com", true),
			cookieField("datr", "messenger.com", true),
		},
		WaitForURLPattern: `^https://www\.messenger\.com/(?:e2ee/)?(?:t/[0-9]+/?)?(?:\?.*)?$`,
	}
	loggedIn := []Cookie{
		{Name: "xs", Value: "x1", Domain: ".messenger.com"},
		{Name: "c_user", Value: "42", Domain: ".messenger.com"},
		{Name: "datr", Value: "d1", Domain: ".messenger.com"},
		{Name: "xs", Value: "wrong-site", Domain: ".facebook.com"},
	}

	if _, done := Collect(p, &Snapshot{URL: "https://www.messenger.com/login/", Cookies: loggedIn[1:]}); done {
		t.Fatal("not done without xs")
	}
	if _, done := Collect(p, &Snapshot{URL: "https://www.messenger.com/login/", Cookies: loggedIn}); done {
		t.Fatal("not done until the URL reaches the inbox")
	}
	values, done := Collect(p, &Snapshot{URL: "https://www.messenger.com/t/123/", Cookies: loggedIn})
	if !done || values["xs"] != "x1" || values["c_user"] != "42" || values["datr"] != "d1" {
		t.Fatalf("expected messenger.com cookies, got %v (done=%v)", values, done)
	}
}

// Optional cookies are passed along when present but don't hold up completion.
func TestCollectOptionalCookies(t *testing.T) {
	p := &bridgev2.LoginCookiesParams{Fields: []bridgev2.LoginCookieField{
		cookieField("sessionid", "instagram.com", true),
		cookieField("rur", "instagram.com", false),
	}}
	values, done := Collect(p, &Snapshot{Cookies: []Cookie{{Name: "sessionid", Value: "s", Domain: "www.instagram.com"}}})
	if !done || values["sessionid"] != "s" || values["rur"] != "" {
		t.Fatalf("got %v, %v", values, done)
	}
}

// Instagram's CAPTCHA: the token comes from the bridge's ExtractJS result.
func TestCollectExtractJSToken(t *testing.T) {
	p := &bridgev2.LoginCookiesParams{
		ExtractJS: "(() => Promise.resolve({captcha_token: 'abc'}))()",
		Fields: []bridgev2.LoginCookieField{{ID: "captcha_token", Required: true, Sources: []bridgev2.LoginCookieFieldSource{
			{Type: bridgev2.LoginCookieTypeSpecial, Name: "captcha_token"},
		}}},
	}
	if !CanComplete(p) {
		t.Fatal("ExtractJS steps should be completable")
	}
	if _, done := Collect(p, &Snapshot{}); done {
		t.Fatal("not done before the script resolves")
	}
	values, done := Collect(p, &Snapshot{Extracted: map[string]any{"captcha_token": "abc"}})
	if !done || values["captcha_token"] != "abc" {
		t.Fatalf("got %v, %v", values, done)
	}
}

func TestCanComplete(t *testing.T) {
	header := &bridgev2.LoginCookiesParams{Fields: []bridgev2.LoginCookieField{{ID: "auth", Required: true,
		Sources: []bridgev2.LoginCookieFieldSource{{Type: bridgev2.LoginCookieTypeRequestHeader, Name: "Authorization"}}}}}
	if CanComplete(header) {
		t.Fatal("request headers can't be read without interception")
	}
	special := &bridgev2.LoginCookiesParams{Fields: []bridgev2.LoginCookieField{{ID: "t", Required: true,
		Sources: []bridgev2.LoginCookieFieldSource{{Type: bridgev2.LoginCookieTypeSpecial, Name: "t"}}}}}
	if CanComplete(special) {
		t.Fatal("special values need ExtractJS")
	}
}

func TestDomainMatches(t *testing.T) {
	for _, c := range []struct {
		cookie, want string
		ok           bool
	}{
		{".messenger.com", "messenger.com", true},
		{"www.messenger.com", "messenger.com", true},
		{"messenger.com", "messenger.com", true},
		{".facebook.com", "messenger.com", false},
		{"evilmessenger.com", "messenger.com", false},
		{".instagram.com", "", true},
	} {
		if got := domainMatches(c.cookie, c.want); got != c.ok {
			t.Errorf("domainMatches(%q, %q) = %v", c.cookie, c.want, got)
		}
	}
}

func newTestSession(p *bridgev2.LoginCookiesParams) (int, *session) {
	nop := zerolog.Nop()
	sess := &session{params: p, log: &nop, result: make(chan result, 1)}
	sessionsLock.Lock()
	id := nextID
	nextID++
	sessions[id] = sess
	sessionsLock.Unlock()
	return id, sess
}

func instagramParams() *bridgev2.LoginCookiesParams {
	return &bridgev2.LoginCookiesParams{
		Fields: []bridgev2.LoginCookieField{
			cookieField("sessionid", "instagram.com", true),
			cookieField("csrftoken", "instagram.com", true),
			cookieField("ds_user_id", "instagram.com", true),
		},
		WaitForURLPattern: `^https://www\.instagram\.com/(?:direct/(?:inbox/|t/[0-9]+/)?)?(?:\?.*)?$`,
	}
}

var igCookies = `[{"name":"sessionid","value":"s","domain":".instagram.com"},
	{"name":"csrftoken","value":"c","domain":".instagram.com"},
	{"name":"ds_user_id","value":"1","domain":".instagram.com"}]`

// Logged in, but closed on an interstitial ("Save your login info?") that
// doesn't match the final URL pattern: closing should still finish the login.
func TestClosedAfterLoggingInFinishes(t *testing.T) {
	id, sess := newTestSession(instagramParams())
	handleSnapshot(id, `{"url":"https://www.instagram.com/accounts/onetap/?next=%2F","cookies":`+igCookies+`}`)
	select {
	case r := <-sess.result:
		t.Fatalf("finished too early on the interstitial: %+v", r)
	default:
	}
	handleClosed(id)
	r := <-sess.result
	if r.err != nil || r.values["sessionid"] != "s" || r.values["ds_user_id"] != "1" {
		t.Fatalf("expected the collected cookies, got %+v", r)
	}
}

// Closing before logging in is still a cancel.
func TestClosedBeforeLoggingInCancels(t *testing.T) {
	id, sess := newTestSession(instagramParams())
	handleSnapshot(id, `{"url":"https://www.instagram.com/accounts/login/","cookies":[{"name":"csrftoken","value":"c","domain":".instagram.com"}]}`)
	handleClosed(id)
	if r := <-sess.result; r.err != ErrClosed {
		t.Fatalf("expected ErrClosed, got %+v", r)
	}
}

// Reaching the feed finishes on its own.
func TestReachingFeedFinishes(t *testing.T) {
	id, sess := newTestSession(instagramParams())
	handleSnapshot(id, `{"url":"https://www.instagram.com/","cookies":`+igCookies+`}`)
	if r := <-sess.result; r.err != nil || r.values["csrftoken"] != "c" {
		t.Fatalf("expected login to finish on the feed, got %+v", r)
	}
}

func TestPageForLogDropsQuery(t *testing.T) {
	if got := pageForLog("https://www.instagram.com/accounts/onetap/?next=%2F&token=secret#x"); got != "https://www.instagram.com/accounts/onetap/" {
		t.Fatalf("got %q", got)
	}
}

// Messenger's facebook.com mode: same cookies, all on facebook.com, finishing
// on the Facebook home page or Messages.
func TestCollectFacebookCookies(t *testing.T) {
	p := &bridgev2.LoginCookiesParams{
		Fields: []bridgev2.LoginCookieField{
			cookieField("xs", "facebook.com", true),
			cookieField("c_user", "facebook.com", true),
			cookieField("datr", "facebook.com", true),
		},
		WaitForURLPattern: `^https://www\.facebook\.com/(?:messages/(?:e2ee/)?(?:t/[0-9]+/?)?)?(?:\?.*)?$`,
	}
	fb := []Cookie{
		{Name: "xs", Value: "x", Domain: ".facebook.com"},
		{Name: "c_user", Value: "42", Domain: ".facebook.com"},
		{Name: "datr", Value: "d", Domain: ".facebook.com"},
	}
	if _, done := Collect(p, &Snapshot{URL: "https://www.facebook.com/two_step_verification/authentication/", Cookies: fb}); done {
		t.Fatal("shouldn't finish on the 2FA page")
	}
	if values, done := Collect(p, &Snapshot{URL: "https://www.facebook.com/", Cookies: fb}); !done || values["c_user"] != "42" {
		t.Fatalf("expected to finish on the home page, got %v %v", values, done)
	}
}
