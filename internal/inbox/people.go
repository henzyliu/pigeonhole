package inbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/provisionutil"
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
			resp, err := provisionutil.SearchUsers(ctx, login, query)
			if err != nil {
				lastErr = fmt.Errorf("%s search failed: %w", n.Name, err)
				continue
			}
			for _, r := range resp.Results {
				p := Person{
					Network: netID, NetworkName: n.Name, LoginID: string(login.ID),
					ID: string(r.ID), Name: r.Name, Avatar: MediaURL(string(r.AvatarURL)),
				}
				if p.Name == "" {
					p.Name = p.ID
				}
				p.RoomID = s.existingDM(ctx, n, r.ID)
				people = append(people, p)
			}
		}
	}
	if len(people) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return people, nil
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
