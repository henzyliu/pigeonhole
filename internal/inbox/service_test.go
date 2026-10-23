package inbox

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
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

// Every login flow should produce a first step the UI knows how to render.
// QR flows need the network to get there: the code comes from the server.
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
			case "display_and_wait":
				if step.DisplayType == "qr" && !strings.HasPrefix(step.QRImage, "data:image/png;base64,") {
					t.Errorf("%s/%s: QR step without a QR image", network.ID, flow.ID)
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

func TestValidateFields(t *testing.T) {
	fields := []bridgev2.LoginInputDataField{
		{ID: "mfatype", Name: "Login method", Type: bridgev2.LoginInputFieldTypeSelect, Options: []string{"Authentication app", "Text message"}},
		{ID: "code", Name: "Code", Validate: func(s string) (string, error) { return strings.ReplaceAll(s, " ", ""), nil }},
	}
	values := map[string]string{"mfatype": "Phone", "code": "123 456"}
	if err := validateFields(fields, values); err == nil {
		t.Fatal("expected an error for a value that isn't one of the options")
	}
	values["mfatype"] = "Text message"
	if err := validateFields(fields, values); err != nil {
		t.Fatal(err)
	}
	if values["code"] != "123456" {
		t.Fatalf("expected Validate to normalize the code, got %q", values["code"])
	}
}

// Other people's read receipts show up as "seen by" on the newest message at
// or before where they've read to, and move forward as they read more.
func TestSeenReceipts(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	mx := svc.networks["messenger"].MX
	friend := mx.GhostIntent("friend")
	if err := friend.SetDisplayName(ctx, "Friend"); err != nil {
		t.Fatal(err)
	}
	stateKey := ""
	roomID, err := mx.BotIntent().CreateRoom(ctx, &mautrix.ReqCreateRoom{
		IsDirect: true,
		Invite:   []id.UserID{friend.GetMXID(), Me},
		InitialState: []*event.Event{{
			Type: event.StateBridge, StateKey: &stateKey,
			Content: event.Content{Parsed: &event.BridgeEventContent{}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	me, _, _ := mx.NewUserIntent(ctx, Me, "")
	send := func(intent bridgev2.MatrixAPI, body string, ts time.Time) id.EventID {
		resp, err := intent.SendMessage(ctx, roomID, event.EventMessage, &event.Content{
			Parsed: &event.MessageEventContent{MsgType: event.MsgText, Body: body},
		}, &bridgev2.MatrixSendExtra{Timestamp: ts})
		if err != nil {
			t.Fatal(err)
		}
		return resp.EventID
	}
	base := time.Now().Add(-time.Hour)
	first := send(me, "hi", base)
	send(me, "you there?", base.Add(time.Minute))
	second := send(friend, "yep", base.Add(2*time.Minute))

	seenOn := func() map[string][]string {
		msgs, err := svc.Messages(ctx, string(roomID), 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string][]string{}
		for _, m := range msgs {
			for _, r := range m.SeenBy {
				out[m.Body] = append(out[m.Body], r.Name)
			}
		}
		return out
	}
	// Read up to the first message only.
	if err = friend.MarkRead(ctx, roomID, first, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := seenOn(); len(got) != 1 || len(got["hi"]) != 1 || got["hi"][0] != "Friend" {
		t.Fatalf("expected Friend to have seen 'hi', got %v", got)
	}
	// Once their read position is their own reply, nothing is shown: the reply
	// already makes it obvious they've seen what came before (as in Messenger).
	if err = friend.MarkRead(ctx, roomID, second, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := seenOn(); len(got) != 0 {
		t.Fatalf("reader shouldn't be shown on their own message, got %v", got)
	}
	// The local user's own receipts clear unread instead of adding a reader.
	if err = me.MarkRead(ctx, roomID, second, time.Now()); err != nil {
		t.Fatal(err)
	}
	if chats, _ := svc.Chats(ctx); chats[0].Unread != 0 {
		t.Fatalf("expected chat to be read, got %+v", chats[0])
	}
}

// Delivery receipts upgrade a sent message to "delivered", and a late plain
// success doesn't downgrade it again.
func TestDeliveryStatus(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	roomID, _ := newPortal(t, svc, "messenger")
	mx := svc.networks["messenger"].MX
	// Stored directly rather than via SendText, which would also hand the
	// message to the (login-less) bridge and race its failure status.
	me, _, _ := mx.NewUserIntent(ctx, Me, "")
	sent, err := me.SendMessage(ctx, roomID, event.EventMessage, &event.Content{
		Parsed: &event.MessageEventContent{MsgType: event.MsgText, Body: "hello"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.Store.SetEventStatus(ctx, sent.EventID, roomID, "pending")
	status := func() (string, string) {
		msgs, err := svc.Messages(ctx, string(roomID), 0, 10)
		if err != nil || len(msgs) != 1 {
			t.Fatalf("expected one message, got %v, %v", msgs, err)
		}
		return msgs[0].EventID, msgs[0].Status
	}
	eventID, st := status()
	if st != "pending" {
		t.Fatalf("expected pending, got %q", st)
	}
	info := &bridgev2.MessageStatusEventInfo{RoomID: roomID, SourceEventID: id.EventID(eventID)}
	mx.SendMessageStatus(ctx, &bridgev2.MessageStatus{Status: event.MessageStatusSuccess}, info)
	if _, st = status(); st != "success" {
		t.Fatalf("expected success, got %q", st)
	}
	mx.SendMessageStatus(ctx, &bridgev2.MessageStatus{Status: event.MessageStatusSuccess, DeliveredTo: []id.UserID{"@x:local"}}, info)
	if _, st = status(); st != "delivered" {
		t.Fatalf("expected delivered, got %q", st)
	}
	mx.SendMessageStatus(ctx, &bridgev2.MessageStatus{Status: event.MessageStatusSuccess}, info)
	if _, st = status(); st != "delivered" {
		t.Fatalf("a late success shouldn't downgrade delivered, got %q", st)
	}
}

// Steps whose required value only comes from client-side "special" handling
// (Instagram's CAPTCHA token) can't be pasted, so the UI must say so.
func TestNeedsEmbeddedBrowser(t *testing.T) {
	cookie := bridgev2.LoginCookieFieldSource{Type: bridgev2.LoginCookieTypeCookie, Name: "sessionid"}
	special := bridgev2.LoginCookieFieldSource{Type: bridgev2.LoginCookieTypeSpecial, Name: "captcha_token"}
	cases := []struct {
		name   string
		fields []bridgev2.LoginCookieField
		want   bool
	}{
		{"cookies", []bridgev2.LoginCookieField{{ID: "sessionid", Required: true, Sources: []bridgev2.LoginCookieFieldSource{cookie}}}, false},
		{"captcha", []bridgev2.LoginCookieField{{ID: "captcha_token", Required: true, Sources: []bridgev2.LoginCookieFieldSource{special}}}, true},
		{"optional special", []bridgev2.LoginCookieField{{ID: "x", Sources: []bridgev2.LoginCookieFieldSource{special}}}, false},
	}
	for _, c := range cases {
		if got := needsEmbeddedBrowser(&bridgev2.LoginCookiesParams{Fields: c.fields}); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
