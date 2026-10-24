package inbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"

	"unified-inbox/internal/loginbrowser"
)

type LoginFlow struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type LoginField struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Default     string `json:"default"`
	Optional    bool   `json:"optional"`
	// Options lists the allowed values for select fields (e.g. 2FA method).
	Options []string `json:"options"`
}

// LoginStep is the UI-facing version of bridgev2.LoginStep.
type LoginStep struct {
	SessionID    string       `json:"sessionId"`
	Network      string       `json:"network"`
	Type         string       `json:"type"` // user_input, cookies, display_and_wait, complete
	Instructions string       `json:"instructions"`
	Fields       []LoginField `json:"fields"`
	CookieURL    string       `json:"cookieUrl"`
	CookieNames  []string     `json:"cookieNames"`
	// NeedsBrowser is set for cookie steps that can only be completed inside an
	// embedded browser (e.g. Instagram's CAPTCHA), so pasting can't work.
	NeedsBrowser bool `json:"needsBrowser"`
	// BrowserAvailable is set when the built-in login window can complete
	// this cookie step (see BrowserLogin).
	BrowserAvailable bool   `json:"browserAvailable"`
	DisplayType      string `json:"displayType"`
	DisplayData      string `json:"displayData"`
	// QRImage is DisplayData drawn as a QR code (a PNG data URL), for QR steps.
	QRImage   string `json:"qrImage"`
	AccountID string `json:"accountId"`
}

type Account struct {
	Network     string `json:"network"`
	NetworkName string `json:"networkName"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	State       string `json:"state"`
	Error       string `json:"error"`
	// HistoryPending counts this account's chats whose older messages are
	// still loading.
	HistoryPending int `json:"historyPending"`
}

type pendingLogin struct {
	network string
	process bridgev2.LoginProcess
	step    *bridgev2.LoginStep
	// closeBrowser closes the login window, if one is open for this login.
	closeBrowser context.CancelFunc
}

func (s *Service) Networks() []LoginFlow {
	out := make([]LoginFlow, 0, len(s.order))
	for _, netID := range s.order {
		out = append(out, LoginFlow{ID: netID, Name: s.networks[netID].Name})
	}
	return out
}

func (s *Service) LoginFlows(netID string) ([]LoginFlow, error) {
	n, err := s.network(netID)
	if err != nil {
		return nil, err
	}
	var out []LoginFlow
	for _, flow := range n.Bridge.Network.GetLoginFlows() {
		out = append(out, LoginFlow{ID: flow.ID, Name: flow.Name, Description: flow.Description})
	}
	return out, nil
}

func (s *Service) StartLogin(netID, flowID string) (*LoginStep, error) {
	n, err := s.network(netID)
	if err != nil {
		return nil, err
	}
	user, err := n.Bridge.GetUserByMXID(s.ctx, Me)
	if err != nil {
		return nil, err
	}
	process, err := n.Bridge.Network.CreateLogin(s.ctx, user, flowID)
	if err != nil {
		s.Log.Warn().Err(err).Str("network", netID).Str("flow", flowID).Msg("Failed to create login")
		return nil, err
	}
	step, err := process.Start(s.ctx)
	if err != nil {
		s.Log.Warn().Err(err).Str("network", netID).Str("flow", flowID).Msg("Failed to start login")
		process.Cancel()
		return nil, err
	}
	sessionID := fmt.Sprintf("%s-%d", netID, time.Now().UnixNano())
	return s.advance(sessionID, &pendingLogin{network: netID, process: process}, step), nil
}

// SubmitLogin answers the current step. For display_and_wait steps, values is
// ignored and the call blocks until the remote side advances.
func (s *Service) SubmitLogin(sessionID string, values map[string]string) (*LoginStep, error) {
	s.lock.Lock()
	pending, ok := s.logins[sessionID]
	s.lock.Unlock()
	if !ok {
		s.Log.Warn().Str("session", sessionID).Msg("Login submit for unknown or expired session")
		return nil, fmt.Errorf("login session expired, please start again")
	}
	// Only names are logged: values are passwords, codes or session cookies.
	log := s.Log.With().Str("network", pending.network).Str("step_type", string(pending.step.Type)).
		Str("step_id", pending.step.StepID).Logger()
	var step *bridgev2.LoginStep
	var err error
	switch pending.step.Type {
	case bridgev2.LoginStepTypeUserInput:
		log.Info().Strs("fields", mapKeys(values)).Msg("Submitting login step")
		if proc, ok := pending.process.(bridgev2.LoginProcessUserInput); !ok {
			err = errUnsupportedStep
		} else if err = validateFields(pending.step.UserInputParams.Fields, values); err == nil {
			step, err = proc.SubmitUserInput(s.ctx, values)
		}
	case bridgev2.LoginStepTypeCookies:
		var cookies map[string]string
		if cookies, err = ParseCookies(values["cookies"]); err == nil {
			log.Info().Strs("cookie_names", mapKeys(cookies)).Msg("Submitting login cookies")
			step, err = s.submitCookies(pending, cookies)
		}
	case bridgev2.LoginStepTypeDisplayAndWait:
		if proc, ok := pending.process.(bridgev2.LoginProcessDisplayAndWait); !ok {
			err = errUnsupportedStep
		} else {
			step, err = proc.Wait(s.ctx)
		}
	default:
		err = fmt.Errorf("unsupported login step %q", pending.step.Type)
	}
	if err != nil {
		log.Warn().Err(err).Msg("Login step failed")
		return nil, err
	}
	return s.advance(sessionID, pending, step), nil
}

// needsEmbeddedBrowser reports whether a cookie step has a required value
// that can't be pasted: one only produced by "special" client-side handling,
// like the token from Instagram's CAPTCHA script.
func needsEmbeddedBrowser(params *bridgev2.LoginCookiesParams) bool {
	for _, f := range params.Fields {
		if !f.Required || len(f.Sources) == 0 {
			continue
		}
		pasteable := false
		for _, src := range f.Sources {
			if src.Type != bridgev2.LoginCookieTypeSpecial {
				pasteable = true
			}
		}
		if !pasteable {
			return true
		}
	}
	return false
}

// postLoginBackfillDelay gives a new login's chat-list paging time to finish
// (about 10 seconds in practice) before queueing history for its chats.
const postLoginBackfillDelay = 45 * time.Second

var errUnsupportedStep = errors.New("this login step isn't supported, please try a different sign-in method")

func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// validateFields applies each field's own validation (which may also
// normalize the value, e.g. strip spaces from a 2FA code), the same way the
// bridge's login command does before submitting.
func validateFields(fields []bridgev2.LoginInputDataField, values map[string]string) error {
	for _, f := range fields {
		val, ok := values[f.ID]
		if !ok {
			if f.Optional {
				continue
			}
			return fmt.Errorf("%s is required", f.Name)
		}
		if len(f.Options) > 0 && !slices.Contains(f.Options, val) {
			return fmt.Errorf("%s must be one of: %s", f.Name, strings.Join(f.Options, ", "))
		}
		if f.Validate != nil {
			cleaned, err := f.Validate(val)
			if err != nil {
				return fmt.Errorf("%s: %w", f.Name, err)
			}
			values[f.ID] = cleaned
		}
	}
	return nil
}

func (s *Service) CancelLogin(sessionID string) {
	s.lock.Lock()
	pending, ok := s.logins[sessionID]
	delete(s.logins, sessionID)
	closeBrowser := pending.closeBrowserFunc()
	s.lock.Unlock()
	if closeBrowser != nil {
		closeBrowser()
	}
	if ok {
		pending.process.Cancel()
	}
}

func (p *pendingLogin) closeBrowserFunc() context.CancelFunc {
	if p == nil {
		return nil
	}
	return p.closeBrowser
}

func (s *Service) submitCookies(pending *pendingLogin, cookies map[string]string) (*bridgev2.LoginStep, error) {
	proc, ok := pending.process.(bridgev2.LoginProcessCookies)
	if !ok {
		return nil, errUnsupportedStep
	}
	return proc.SubmitCookies(s.ctx, cookies)
}

// BrowserLogin completes the current cookie step in the built-in login
// window: the user logs in on the real website, and the cookies (or script
// results) the bridge asked for are collected automatically. It blocks until
// the window finishes or is closed.
func (s *Service) BrowserLogin(sessionID string) (*LoginStep, error) {
	s.lock.Lock()
	pending, ok := s.logins[sessionID]
	s.lock.Unlock()
	if !ok {
		return nil, fmt.Errorf("login session expired, please start again")
	} else if pending.step.Type != bridgev2.LoginStepTypeCookies || pending.step.CookiesParams == nil {
		return nil, errUnsupportedStep
	}
	n, err := s.network(pending.network)
	if err != nil {
		return nil, err
	}
	log := s.Log.With().Str("network", pending.network).Str("step_id", pending.step.StepID).Logger()
	ctx, closeBrowser := context.WithCancel(s.ctx)
	defer closeBrowser()
	s.lock.Lock()
	pending.closeBrowser = closeBrowser
	s.lock.Unlock()
	defer func() {
		s.lock.Lock()
		pending.closeBrowser = nil
		s.lock.Unlock()
	}()

	log.Info().Msg("Opening login window")
	values, err := loginbrowser.Open(ctx, "Log in to "+n.Name, pending.step.CookiesParams)
	if err != nil {
		log.Info().Err(err).Msg("Login window finished without logging in")
		return nil, err
	}
	// Only names are logged, never the values.
	log.Info().Strs("cookie_names", mapKeys(values)).Msg("Submitting cookies from login window")
	step, err := s.submitCookies(pending, values)
	if err != nil {
		log.Warn().Err(err).Msg("Login step failed")
		return nil, err
	}
	return s.advance(sessionID, pending, step), nil
}

func (s *Service) advance(sessionID string, pending *pendingLogin, step *bridgev2.LoginStep) *LoginStep {
	s.Log.Info().Str("network", pending.network).Str("step_type", string(step.Type)).
		Str("step_id", step.StepID).Str("instructions", step.Instructions).Msg("Login step")
	pending.step = step
	s.lock.Lock()
	if step.Type == bridgev2.LoginStepTypeComplete {
		delete(s.logins, sessionID)
		// A new login syncs its chat list in the background for a little
		// while; queue history for those chats once that has had time to run
		// (the periodic check catches anything later).
		if n, ok := s.networks[pending.network]; ok {
			go func() {
				select {
				case <-time.After(postLoginBackfillDelay):
					if err := s.queueMissingBackfill(s.ctx, n); err != nil {
						s.Log.Warn().Err(err).Str("network", n.ID).Msg("Failed to queue history backfill after login")
					}
				case <-s.ctx.Done():
				}
			}()
		}
	} else {
		s.logins[sessionID] = pending
	}
	s.lock.Unlock()

	out := &LoginStep{SessionID: sessionID, Network: pending.network, Type: string(step.Type), Instructions: step.Instructions}
	switch {
	case step.UserInputParams != nil:
		for _, f := range step.UserInputParams.Fields {
			out.Fields = append(out.Fields, LoginField{
				ID: f.ID, Name: f.Name, Type: string(f.Type), Description: f.Description,
				Default: f.DefaultValue, Optional: f.Optional, Options: f.Options,
			})
		}
	case step.CookiesParams != nil:
		out.CookieURL = step.CookiesParams.URL
		for _, f := range step.CookiesParams.Fields {
			out.CookieNames = append(out.CookieNames, f.ID)
		}
		out.NeedsBrowser = needsEmbeddedBrowser(step.CookiesParams)
		out.BrowserAvailable = loginbrowser.Supported() && loginbrowser.CanComplete(step.CookiesParams)
	case step.DisplayAndWaitParams != nil:
		out.DisplayType = string(step.DisplayAndWaitParams.Type)
		out.DisplayData = step.DisplayAndWaitParams.Data
		if step.DisplayAndWaitParams.Type == bridgev2.LoginDisplayTypeQR {
			if png, err := qrcode.Encode(out.DisplayData, qrcode.Medium, 512); err == nil {
				out.QRImage = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
			} else {
				s.Log.Warn().Err(err).Msg("Failed to draw login QR code")
			}
		}
	case step.CompleteParams != nil:
		out.AccountID = string(step.CompleteParams.UserLoginID)
		s.emit("inbox:accounts", nil)
	}
	return out
}

func (s *Service) Accounts() ([]Account, error) {
	var out []Account
	for _, netID := range s.order {
		n := s.networks[netID]
		if !n.started.Load() {
			continue // listed once started (see Network.started)
		}
		user, err := n.Bridge.GetUserByMXID(s.ctx, Me)
		if err != nil {
			return nil, err
		}
		pending, err := s.pendingHistory(s.ctx, n)
		if err != nil {
			return nil, err
		}
		for _, login := range user.GetUserLogins() {
			acc := Account{Network: netID, NetworkName: n.Name, ID: string(login.ID), Name: login.RemoteName, State: "connecting"}
			s.lock.Lock()
			state, ok := s.states[netID+"/"+string(login.ID)]
			s.lock.Unlock()
			if ok {
				acc.State = strings.ToLower(string(state.StateEvent))
				if state.StateEvent != status.StateConnected {
					acc.Error = state.Message
					if acc.Error == "" {
						acc.Error = string(state.Error)
					}
				}
			}
			for _, owner := range pending {
				if owner == login.ID {
					acc.HistoryPending++
				}
			}
			out = append(out, acc)
		}
	}
	return out, nil
}

func (s *Service) Logout(netID, loginID string) error {
	n, err := s.network(netID)
	if err != nil {
		return err
	}
	login, err := n.Bridge.GetExistingUserLoginByID(s.ctx, networkid.UserLoginID(loginID))
	if err != nil {
		return err
	} else if login == nil {
		return fmt.Errorf("account not found")
	}
	login.Logout(s.ctx)
	s.emit("inbox:accounts", nil)
	return nil
}

var (
	curlCookieHeader = regexp.MustCompile(`(?i)-H\s+['"]cookie:\s*([^'"]+)['"]`)
	curlCookieFlag   = regexp.MustCompile(`(?:-b|--cookie)\s+['"]([^'"]+)['"]`)
)

// ParseCookies accepts a JSON object, a cookie-editor style JSON array, a
// "Copy as cURL" command, or a raw "a=b; c=d" cookie header.
func ParseCookies(input string) (map[string]string, error) {
	input = strings.TrimSpace(input)
	out := make(map[string]string)
	switch {
	case strings.HasPrefix(input, "{"):
		if err := json.Unmarshal([]byte(input), &out); err != nil {
			return nil, fmt.Errorf("invalid cookie JSON: %w", err)
		}
		return out, nil
	case strings.HasPrefix(input, "["):
		var list []struct{ Name, Value string }
		if err := json.Unmarshal([]byte(input), &list); err != nil {
			return nil, fmt.Errorf("invalid cookie JSON: %w", err)
		}
		for _, c := range list {
			out[c.Name] = c.Value
		}
		return out, nil
	case strings.HasPrefix(input, "curl"):
		if m := curlCookieHeader.FindStringSubmatch(input); m != nil {
			input = m[1]
		} else if m = curlCookieFlag.FindStringSubmatch(input); m != nil {
			input = m[1]
		} else {
			return nil, fmt.Errorf("no cookies found in the cURL command")
		}
	}
	for _, part := range strings.Split(input, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && name != "" {
			out[name] = value
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no cookies found")
	}
	return out, nil
}
