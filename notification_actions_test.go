package main

import (
	"testing"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestParseMessageAction(t *testing.T) {
	room := map[string]any{"roomId": "!a:local"}
	for _, tc := range []struct {
		name string
		resp runtime.NotificationResponse
		want messageAction
		ok   bool
	}{
		{"reply", runtime.NotificationResponse{ActionIdentifier: replyActionID, UserText: "  on my way \n", UserInfo: room}, messageAction{roomID: "!a:local", reply: "on my way"}, true},
		{"empty reply", runtime.NotificationResponse{ActionIdentifier: replyActionID, UserText: "   ", UserInfo: room}, messageAction{}, false},
		{"mark read", runtime.NotificationResponse{ActionIdentifier: markReadActionID, UserInfo: room}, messageAction{roomID: "!a:local"}, true},
		{"click", runtime.NotificationResponse{ActionIdentifier: "DEFAULT_ACTION", UserInfo: room}, messageAction{}, false},
		{"other alert", runtime.NotificationResponse{ActionIdentifier: replyActionID, UserText: "hi", UserInfo: map[string]any{"update": true}}, messageAction{}, false},
	} {
		got, ok := parseMessageAction(tc.resp)
		if got != tc.want || ok != tc.ok {
			t.Errorf("%s: got %+v, %v; want %+v, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}
