package inbox

import (
	"context"
	"html"
	"slices"
	"strings"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Mentions are sent the Matrix way: the message's HTML has a link to the
// person (a "pill"), which each bridge turns into the network's own
// mention (a Messenger or Instagram tag, a WhatsApp @mention, Discord's
// <@user>), so the person is notified.

// Member is someone in a chat who can be mentioned.
type Member struct {
	UserID string `json:"userId"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
	// Presence is a Discord friend's status ("online", "idle" or "dnd").
	Presence string `json:"presence,omitempty"`
}

// Mention is a person picked from the @ suggestions, as "@Name" in the
// text.
type Mention struct {
	UserID string `json:"userId"`
	Name   string `json:"name"`
}

// ChatMembers lists the people in a chat other than you, by name.
func (s *Service) ChatMembers(ctx context.Context, roomID string) ([]Member, error) {
	members, err := s.Store.Members(ctx, id.RoomID(roomID))
	if err != nil {
		return nil, err
	}
	profiles, err := s.profiles(ctx)
	if err != nil {
		return nil, err
	}
	presence := s.memberPresence(ctx, roomID)
	out := []Member{}
	for userID, membership := range members {
		if membership != event.MembershipJoin || userID == Me || s.isSelf(userID) {
			continue
		}
		p, ok := profiles[userID]
		if !ok || p.name == "" {
			continue // the bridge bot, or someone not loaded yet
		}
		out = append(out, Member{UserID: string(userID), Name: p.name, Avatar: MediaURL(p.avatar), Presence: presence[userID]})
	}
	slices.SortFunc(out, func(a, b Member) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return out, nil
}

// withMentions adds the HTML and mention list for each "@Name" in the
// text that was picked as a mention. Text without any is left plain.
func withMentions(content *event.MessageEventContent, mentions []Mention) {
	var b strings.Builder
	rest := content.Body
	var userIDs []id.UserID
	for _, m := range mentions {
		token := "@" + m.Name
		at := strings.Index(rest, token)
		if m.UserID == "" || m.Name == "" || at < 0 {
			continue
		}
		b.WriteString(escapeHTML(rest[:at]))
		b.WriteString(`<a href="https://matrix.to/#/` + escapeHTML(m.UserID) + `">` + escapeHTML(m.Name) + `</a>`)
		rest = rest[at+len(token):]
		if !slices.Contains(userIDs, id.UserID(m.UserID)) {
			userIDs = append(userIDs, id.UserID(m.UserID))
		}
	}
	if len(userIDs) == 0 {
		return
	}
	b.WriteString(escapeHTML(rest))
	content.Format = event.FormatHTML
	content.FormattedBody = b.String()
	content.Mentions = &event.Mentions{UserIDs: userIDs}
}

// escapeHTML escapes text for the message's HTML, keeping line breaks.
func escapeHTML(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), "\n", "<br>")
}
