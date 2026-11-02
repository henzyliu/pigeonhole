package inbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/provisionutil"
	"maunium.net/go/mautrix/id"
)

// Person is a search result for starting or opening a one-to-one chat.
type Person struct {
	Network     string `json:"network"`
	NetworkName string `json:"networkName"`
	LoginID     string `json:"loginId"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar"`
	// RoomID is set when a chat with this person already exists in the app.
	RoomID string `json:"roomId"`
	// Number is set for a phone number typed in, rather than someone found.
	Number bool `json:"number"`
}

const peopleRequestTimeout = 30 * time.Second

// SearchPeople looks people up by name on every connected account.
//
// This matters most for Messenger: chats that started out end-to-end
// encrypted aren't in any thread list the bridge can read, so opening them
// from here is how they (and their history) get into the app.
func (s *Service) SearchPeople(ctx context.Context, query string) ([]Person, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(s.Log.WithContext(ctx), peopleRequestTimeout)
	defer cancel()
	var people []Person
	var lastErr error
	for _, netID := range s.order {
		n := s.networks[netID]
		for _, login := range s.loginsFor(ctx, n) {
			found, err := s.searchLogin(ctx, n, login, query)
			if err != nil {
				lastErr = fmt.Errorf("%s search failed: %w", n.Name, err)
				continue
			}
			for _, r := range found {
				p := Person{
					Network: netID, NetworkName: n.Name, LoginID: string(login.ID),
					ID: string(r.ID), Name: r.Name, Avatar: MediaURL(string(r.AvatarURL)),
				}
				if p.Name == "" {
					p.Name = p.ID
				}
				p.RoomID = s.existingDM(ctx, n, r.ghostID)
				people = append(people, p)
			}
		}
	}
	people = append(people, s.numberResults(ctx, query, people)...)
	if len(people) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return people, nil
}

// found is someone a search turned up. ID is what opens a chat with them;
// ghostID is who they are in the bridge, to find a chat that already exists.
type found struct {
	ID, ghostID networkid.UserID
	Name        string
	AvatarURL   id.ContentURIString
}

// maxContactResults caps the contacts listed for a search, as a short query
// can match much of a phone's address book.
const maxContactResults = 20

// searchLogin finds people on one account. Networks with a people search
// (Messenger, Instagram, WhatsApp, Discord) use it. Google Messages has none,
// but can list the phone's contacts, which are searched here by name or
// number. Networks with neither are skipped.
func (s *Service) searchLogin(ctx context.Context, n *Network, login *bridgev2.UserLogin, query string) ([]found, error) {
	if api, ok := login.Client.(bridgev2.UserSearchingNetworkAPI); ok {
		resp, err := api.SearchUsers(ctx, query)
		if err != nil {
			return nil, err
		}
		out := make([]found, 0, len(resp))
		for _, r := range resp {
			f := found{ID: r.UserID, ghostID: r.UserID}
			fillFound(&f, r)
			out = append(out, f)
		}
		return out, nil
	}
	api, ok := login.Client.(bridgev2.ContactListingNetworkAPI)
	if !ok {
		return nil, nil
	}
	contacts, err := api.GetContactList(ctx)
	if err != nil {
		return nil, err
	}
	var out []found
	for _, r := range contacts {
		f := found{ID: r.UserID, ghostID: r.UserID}
		fillFound(&f, r)
		phone := contactPhone(r)
		if !contactMatches(f.Name, phone, query) {
			continue
		}
		// Google Messages opens chats by phone number.
		if phone != "" {
			f.ID = networkid.UserID(phone)
		}
		out = append(out, f)
		if len(out) == maxContactResults {
			break
		}
	}
	return out, nil
}

func fillFound(f *found, r *bridgev2.ResolveIdentifierResponse) {
	if r.UserInfo != nil {
		if r.UserInfo.Name != nil {
			f.Name = *r.UserInfo.Name
		}
		if r.UserInfo.Avatar != nil && r.Ghost == nil {
			f.AvatarURL = r.UserInfo.Avatar.MXC
		}
	}
	if r.Ghost != nil {
		if f.Name == "" {
			f.Name = r.Ghost.Name
		}
		f.AvatarURL = r.Ghost.AvatarMXC
	}
}

// contactPhone gives a contact's phone number, from its "tel:" identifier.
func contactPhone(r *bridgev2.ResolveIdentifierResponse) string {
	if r.UserInfo == nil {
		return ""
	}
	for _, ident := range r.UserInfo.Identifiers {
		if phone, ok := strings.CutPrefix(ident, "tel:"); ok {
			return phone
		}
	}
	return ""
}

// contactMatches says whether a contact fits a search: its name contains the
// query, or its number contains the query's digits (at least three, so a
// name with a digit in it doesn't match every number).
func contactMatches(name, phone, query string) bool {
	if strings.Contains(strings.ToLower(name), strings.ToLower(query)) {
		return true
	}
	digits := onlyDigits(query)
	return len(digits) >= 3 && strings.Contains(onlyDigits(phone), digits)
}

func onlyDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

// phoneNumber gives the query as a phone number to text, or "" if it isn't
// one: digits, maybe a leading +, and the usual spaces, dashes, dots and
// brackets.
func phoneNumber(query string) string {
	number := strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9', r == '+':
			return r
		case r == ' ' || r == '-' || r == '.' || r == '(' || r == ')':
			return -1
		}
		return 'x'
	}, strings.TrimSpace(query))
	if strings.ContainsRune(number, 'x') || strings.LastIndexByte(number, '+') > 0 || len(onlyDigits(number)) < 3 {
		return ""
	}
	return number
}

// numberResults offers to text a typed phone number on each account of a
// network that can message any number (Google Messages), unless a contact
// with that number was already found there.
func (s *Service) numberResults(ctx context.Context, query string, people []Person) []Person {
	number := phoneNumber(query)
	if number == "" {
		return nil
	}
	var out []Person
	for _, netID := range s.order {
		n := s.networks[netID]
		if !n.started.Load() || !n.Bridge.Network.GetCapabilities().Provisioning.ResolveIdentifier.AnyPhone {
			continue
		}
	logins:
		for _, login := range s.loginsFor(ctx, n) {
			for _, p := range people {
				if p.Network == netID && p.LoginID == string(login.ID) && onlyDigits(p.ID) == onlyDigits(number) {
					continue logins
				}
			}
			out = append(out, Person{
				Network: netID, NetworkName: n.Name, LoginID: string(login.ID),
				ID: number, Name: strings.TrimSpace(query), Number: true,
			})
		}
	}
	return out
}

// OpenChat returns the chat with a person, creating it if needed. Creating a
// Messenger chat this way also restores its history from secure storage.
func (s *Service) OpenChat(ctx context.Context, network, loginID, userID string) (string, error) {
	n, err := s.network(network)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(s.Log.WithContext(ctx), peopleRequestTimeout)
	defer cancel()
	// Existing chats open locally, without telling Meta anything.
	if roomID := s.existingDM(ctx, n, networkid.UserID(userID)); roomID != "" {
		return roomID, nil
	}
	login, err := n.Bridge.GetExistingUserLoginByID(ctx, networkid.UserLoginID(loginID))
	if err != nil {
		return "", err
	} else if login == nil {
		return "", fmt.Errorf("%s account not found", n.Name)
	}
	resp, err := provisionutil.ResolveIdentifier(ctx, login, userID, true)
	if err != nil {
		return "", fmt.Errorf("couldn't open the chat: %w", err)
	} else if resp == nil || resp.DMRoomID == "" {
		return "", fmt.Errorf("%s didn't return a chat for this person", n.Name)
	}
	return string(resp.DMRoomID), nil
}

func (s *Service) loginsFor(ctx context.Context, n *Network) []*bridgev2.UserLogin {
	if !n.started.Load() {
		return nil
	}
	user, err := n.Bridge.GetUserByMXID(ctx, Me)
	if err != nil {
		return nil
	}
	return user.GetUserLogins()
}

// existingDM finds a one-to-one chat with the user that already has a room.
func (s *Service) existingDM(ctx context.Context, n *Network, userID networkid.UserID) string {
	portals, err := n.Bridge.DB.Portal.GetAllDMsWith(ctx, userID)
	if err != nil {
		return ""
	}
	for _, p := range portals {
		if p.MXID != "" {
			return string(p.MXID)
		}
	}
	return ""
}
