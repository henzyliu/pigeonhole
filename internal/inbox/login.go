package inbox

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"
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
	DisplayType  string       `json:"displayType"`
	DisplayData  string       `json:"displayData"`
	AccountID    string       `json:"accountId"`
}

type Account struct {
	Network     string `json:"network"`
	NetworkName string `json:"networkName"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	State       string `json:"state"`
	Error       string `json:"error"`
}

type pendingLogin struct {
	network string
	process bridgev2.LoginProcess
	step    *bridgev2.LoginStep
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
		return nil, err
	}
	step, err := process.Start(s.ctx)
	if err != nil {
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
		return nil, fmt.Errorf("login session expired, please start again")
	}
	var step *bridgev2.LoginStep
	var err error
	switch pending.step.Type {
	case bridgev2.LoginStepTypeUserInput:
		step, err = pending.process.(bridgev2.LoginProcessUserInput).SubmitUserInput(s.ctx, values)
	case bridgev2.LoginStepTypeCookies:
		var cookies map[string]string
		if cookies, err = ParseCookies(values["cookies"]); err == nil {
			step, err = pending.process.(bridgev2.LoginProcessCookies).SubmitCookies(s.ctx, cookies)
		}
	case bridgev2.LoginStepTypeDisplayAndWait:
		step, err = pending.process.(bridgev2.LoginProcessDisplayAndWait).Wait(s.ctx)
	default:
		err = fmt.Errorf("unsupported login step %q", pending.step.Type)
	}
	if err != nil {
		return nil, err
	}
	return s.advance(sessionID, pending, step), nil
}

func (s *Service) CancelLogin(sessionID string) {
	s.lock.Lock()
	pending, ok := s.logins[sessionID]
	delete(s.logins, sessionID)
	s.lock.Unlock()
	if ok {
		pending.process.Cancel()
	}
}

func (s *Service) advance(sessionID string, pending *pendingLogin, step *bridgev2.LoginStep) *LoginStep {
	pending.step = step
	s.lock.Lock()
	if step.Type == bridgev2.LoginStepTypeComplete {
		delete(s.logins, sessionID)
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
				Default: f.DefaultValue, Optional: f.Optional,
			})
		}
	case step.CookiesParams != nil:
		out.CookieURL = step.CookiesParams.URL
		for _, f := range step.CookiesParams.Fields {
			out.CookieNames = append(out.CookieNames, f.ID)
		}
	case step.DisplayAndWaitParams != nil:
		out.DisplayType = string(step.DisplayAndWaitParams.Type)
		out.DisplayData = step.DisplayAndWaitParams.Data
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
		user, err := n.Bridge.GetUserByMXID(s.ctx, Me)
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
