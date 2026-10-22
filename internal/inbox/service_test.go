package inbox

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	svc, err := New(t.TempDir(), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Stop)
	return svc
}

// Every login flow should produce a first step the UI knows how to render,
// without any network traffic being needed to get there.
func TestLoginFlowsStart(t *testing.T) {
	svc := newTestService(t)
	for _, network := range svc.Networks() {
		flows, err := svc.LoginFlows(network.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, flow := range flows {
			step, err := svc.StartLogin(network.ID, flow.ID)
			if err != nil {
				t.Errorf("%s/%s: %v", network.ID, flow.ID, err)
				continue
			}
			switch step.Type {
			case "user_input":
				if len(step.Fields) == 0 {
					t.Errorf("%s/%s: user_input step without fields", network.ID, flow.ID)
				}
			case "cookies":
				if step.CookieURL == "" || len(step.CookieNames) == 0 {
					t.Errorf("%s/%s: cookies step missing url or names", network.ID, flow.ID)
				}
			default:
				t.Errorf("%s/%s: unexpected first step %q", network.ID, flow.ID, step.Type)
			}
			t.Logf("%s/%s -> %s %v %v", network.ID, flow.ID, step.Type, step.Fields, step.CookieNames)
			svc.CancelLogin(step.SessionID)
		}
	}
}

// Simulates what a bridge does when a chat syncs: create a portal room, set a
// ghost's profile, send a message, react and edit. Then reads it back the way
// the UI does.
func TestPortalRoundTrip(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	mx := svc.networks["instagram"].MX
	ghost := mx.GhostIntent("12345")
	if err := ghost.SetDisplayName(ctx, "Alice"); err != nil {
		t.Fatal(err)
	}
	stateKey := ""
	roomID, err := mx.BotIntent().CreateRoom(ctx, &mautrix.ReqCreateRoom{
		IsDirect: true,
		Invite:   []id.UserID{ghost.GetMXID(), Me},
		InitialState: []*event.Event{{
			Type: event.StateBridge, StateKey: &stateKey,
			Content: event.Content{Parsed: &event.BridgeEventContent{}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := ghost.SendMessage(ctx, roomID, event.EventMessage, &event.Content{
		Parsed: &event.MessageEventContent{MsgType: event.MsgText, Body: "hello there"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ghost.SendMessage(ctx, roomID, event.EventReaction, &event.Content{
		Parsed: &event.ReactionEventContent{RelatesTo: event.RelatesTo{Type: event.RelAnnotation, EventID: resp.EventID, Key: "❤️"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ghost.SendMessage(ctx, roomID, event.EventMessage, &event.Content{
		Parsed: &event.MessageEventContent{
			MsgType: event.MsgText, Body: "* hello!",
			NewContent: &event.MessageEventContent{MsgType: event.MsgText, Body: "hello!"},
			RelatesTo:  &event.RelatesTo{Type: event.RelReplace, EventID: resp.EventID},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	chats, err := svc.Chats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 1 || chats[0].Name != "Alice" || chats[0].Unread != 1 || chats[0].Preview != "hello there" {
		t.Fatalf("unexpected chats: %+v", chats)
	}
	msgs, err := svc.Messages(ctx, string(roomID), 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %+v", msgs)
	}
	m := msgs[0]
	if m.Body != "hello!" || !m.Edited || m.SenderName != "Alice" || m.IsMe || len(m.Reactions) != 1 {
		t.Fatalf("unexpected message: %+v", m)
	}
	if err = svc.Store.MarkRead(ctx, roomID); err != nil {
		t.Fatal(err)
	}
	if chats, _ = svc.Chats(ctx); chats[0].Unread != 0 {
		t.Fatalf("expected chat to be read: %+v", chats[0])
	}
}

func TestParseCookies(t *testing.T) {
	cases := []string{
		`{"sessionid":"a","csrftoken":"b"}`,
		`[{"name":"sessionid","value":"a"},{"name":"csrftoken","value":"b"}]`,
		`curl 'https://www.instagram.com/' -H 'accept: */*' -H 'cookie: sessionid=a; csrftoken=b'`,
		`sessionid=a; csrftoken=b`,
	}
	for _, input := range cases {
		got, err := ParseCookies(input)
		if err != nil || got["sessionid"] != "a" || got["csrftoken"] != "b" {
			t.Errorf("ParseCookies(%q) = %v, %v", input, got, err)
		}
	}
}
