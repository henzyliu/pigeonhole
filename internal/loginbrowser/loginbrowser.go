// Package loginbrowser opens a small native browser window for network
// logins that bridgev2 describes as "cookies" steps: the user logs in on the
// real website, and the cookies (or script results) the bridge asked for are
// collected automatically, instead of being copied out of developer tools.
//
// The native side only reports snapshots of what the window sees (URL,
// cookies, localStorage, ExtractJS results). Deciding when the login is
// complete happens here, in portable code.
package loginbrowser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
)

var (
	ErrUnsupported = errors.New("the built-in login browser isn't available on this platform yet")
	ErrClosed      = errors.New("the login window was closed before the login finished")
)

// Cookie is one cookie as reported by the native browser.
type Cookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
}

// Snapshot is what the native window reports each time it checks the page.
type Snapshot struct {
	URL          string            `json:"url"`
	Cookies      []Cookie          `json:"cookies"`
	LocalStorage map[string]string `json:"localStorage"`
	// Extracted is the resolved value of ExtractJS, if it has finished.
	Extracted map[string]any `json:"extracted"`
}

// Supported reports whether the login window works on this platform.
func Supported() bool { return supported }

// CanComplete reports whether every required field has a source this
// browser can read: cookies, localStorage, or "special" values produced by
// the bridge's ExtractJS. Request header/body sources need request
// interception, which isn't supported.
func CanComplete(p *bridgev2.LoginCookiesParams) bool {
	for _, f := range p.Fields {
		if !f.Required {
			continue
		}
		ok := false
		for _, src := range f.Sources {
			switch src.Type {
			case bridgev2.LoginCookieTypeCookie, bridgev2.LoginCookieTypeLocalStorage:
				ok = true
			case bridgev2.LoginCookieTypeSpecial:
				ok = ok || p.ExtractJS != ""
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// Collect works out the field values available in a snapshot, and whether
// the login is complete: every required field found and, if the bridge gave
// one, the URL matching WaitForURLPattern.
func Collect(p *bridgev2.LoginCookiesParams, snap *Snapshot) (values map[string]string, done bool) {
	values = collectValues(p, snap)
	return values, len(missingRequired(p, values)) == 0 && urlMatches(p, snap.URL)
}

func collectValues(p *bridgev2.LoginCookiesParams, snap *Snapshot) map[string]string {
	values := make(map[string]string)
	for _, f := range p.Fields {
		if v, ok := fieldValue(f, snap); ok {
			values[f.ID] = v
		}
	}
	return values
}

// missingRequired lists the IDs of required fields that have no value yet.
func missingRequired(p *bridgev2.LoginCookiesParams, values map[string]string) []string {
	var missing []string
	for _, f := range p.Fields {
		if f.Required && values[f.ID] == "" {
			missing = append(missing, f.ID)
		}
	}
	return missing
}

func urlMatches(p *bridgev2.LoginCookiesParams, url string) bool {
	if p.WaitForURLPattern == "" {
		return true
	}
	re, err := regexp.Compile(p.WaitForURLPattern)
	return err == nil && re.MatchString(url)
}

// pageForLog strips the query and fragment from a URL, which can carry
// tokens, so the page can be logged safely.
func pageForLog(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host + u.Path
}

func fieldValue(f bridgev2.LoginCookieField, snap *Snapshot) (string, bool) {
	for _, src := range f.Sources {
		switch src.Type {
		case bridgev2.LoginCookieTypeCookie:
			for _, c := range snap.Cookies {
				if c.Name == src.Name && c.Value != "" && domainMatches(c.Domain, src.CookieDomain) {
					return c.Value, true
				}
			}
		case bridgev2.LoginCookieTypeLocalStorage:
			if v := snap.LocalStorage[src.Name]; v != "" {
				return v, true
			}
		case bridgev2.LoginCookieTypeSpecial:
			for _, key := range []string{f.ID, src.Name} {
				if v, ok := snap.Extracted[key].(string); ok && v != "" {
					return v, true
				}
			}
		}
	}
	return "", false
}

// domainMatches checks a cookie's domain (e.g. ".messenger.com") against the
// domain the bridge asked for (e.g. "messenger.com").
func domainMatches(cookieDomain, want string) bool {
	if want == "" {
		return true
	}
	d := strings.TrimPrefix(strings.ToLower(cookieDomain), ".")
	want = strings.TrimPrefix(strings.ToLower(want), ".")
	return d == want || strings.HasSuffix(d, "."+want)
}

// localStorageKeys lists the localStorage keys the native side should read.
func localStorageKeys(p *bridgev2.LoginCookiesParams) []string {
	var keys []string
	for _, f := range p.Fields {
		for _, src := range f.Sources {
			if src.Type == bridgev2.LoginCookieTypeLocalStorage {
				keys = append(keys, src.Name)
			}
		}
	}
	return keys
}

// openRequest is what's sent to the native window.
type openRequest struct {
	Title            string                 `json:"title"`
	URL              string                 `json:"url"`
	UserAgent        string                 `json:"userAgent"`
	InitialCookies   []bridgev2.LoginCookie `json:"initialCookies"`
	LocalStorageKeys []string               `json:"localStorageKeys"`
	ExtractJS        string                 `json:"extractJS"`
	Hidden           bool                   `json:"hidden"`
}

type session struct {
	params *bridgev2.LoginCookiesParams
	log    *zerolog.Logger
	result chan result
	once   sync.Once

	lock       sync.Mutex
	last       *Snapshot
	lastStatus string
}

type result struct {
	values map[string]string
	err    error
}

func (s *session) finish(values map[string]string, err error) {
	s.once.Do(func() { s.result <- result{values, err} })
}

var (
	sessionsLock sync.Mutex
	sessions     = map[int]*session{}
	nextID       = 1
)

// Open shows a login window and blocks until the required fields have been
// collected (the window then closes itself), the user closes the window, or
// ctx is cancelled.
func Open(ctx context.Context, title string, p *bridgev2.LoginCookiesParams) (map[string]string, error) {
	if !supported {
		return nil, ErrUnsupported
	}
	req, err := json.Marshal(openRequest{
		Title: title, URL: p.URL, UserAgent: p.UserAgent, InitialCookies: p.InitialCookies,
		LocalStorageKeys: localStorageKeys(p), ExtractJS: p.ExtractJS, Hidden: p.Hidden,
	})
	if err != nil {
		return nil, err
	}
	sess := &session{params: p, log: zerolog.Ctx(ctx), result: make(chan result, 1)}
	sessionsLock.Lock()
	id := nextID
	nextID++
	sessions[id] = sess
	sessionsLock.Unlock()
	defer func() {
		sessionsLock.Lock()
		delete(sessions, id)
		sessionsLock.Unlock()
	}()

	nativeOpen(id, string(req))
	select {
	case r := <-sess.result:
		nativeClose(id)
		return r.values, r.err
	case <-ctx.Done():
		nativeClose(id)
		return nil, ctx.Err()
	}
}

// handleSnapshot is called by the native side whenever it checks the page.
func handleSnapshot(id int, data string) {
	sessionsLock.Lock()
	sess := sessions[id]
	sessionsLock.Unlock()
	if sess == nil {
		return
	}
	var snap Snapshot
	if json.Unmarshal([]byte(data), &snap) != nil {
		return
	}
	values, done := Collect(sess.params, &snap)

	// Log progress whenever it changes: the page (without query) and which
	// required fields are still missing. Never values.
	missing := missingRequired(sess.params, values)
	page := pageForLog(snap.URL)
	status := fmt.Sprintf("%s|%v|%v", page, missing, done)
	sess.lock.Lock()
	sess.last = &snap
	changed := status != sess.lastStatus
	sess.lastStatus = status
	sess.lock.Unlock()
	if changed {
		sess.log.Info().Str("page", page).Strs("missing_fields", missing).
			Bool("page_matches", urlMatches(sess.params, snap.URL)).Bool("done", done).
			Msg("Login window progress")
	}
	if done {
		sess.finish(values, nil)
	}
}

// handleClosed is called by the native side when the user closes the window.
func handleClosed(id int) {
	sessionsLock.Lock()
	sess := sessions[id]
	sessionsLock.Unlock()
	if sess == nil {
		return
	}
	// Per the bridgev2 contract, closing the window after all required values
	// were collected still counts, even if the final page wasn't reached (e.g.
	// the user closed it on a "Save your login info?" screen).
	sess.lock.Lock()
	last := sess.last
	sess.lock.Unlock()
	if last != nil {
		values := collectValues(sess.params, last)
		if len(missingRequired(sess.params, values)) == 0 {
			sess.log.Info().Msg("Login window closed after logging in; using the collected values")
			sess.finish(values, nil)
			return
		}
	}
	sess.finish(nil, ErrClosed)
}
