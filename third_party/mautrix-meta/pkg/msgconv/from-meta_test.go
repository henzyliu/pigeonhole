package msgconv

import (
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
)

func TestXMALink(t *testing.T) {
	card := func(button, own string) *table.WrappedXMA {
		att := &table.WrappedXMA{LSInsertXmaAttachment: &table.LSInsertXmaAttachment{ActionUrl: own}}
		if button != "" {
			att.CTA = &table.LSInsertAttachmentCta{ActionUrl: button}
		}
		return att
	}
	for _, c := range []struct{ button, own, want string }{
		{"https://a.example/", "https://b.example/", "https://a.example/"},
		{"", "https://b.example/", "https://b.example/"},
		{"fb://event/1", "", ""},
		{"", "", ""},
		{"https://l.facebook.com/l.php?u=https%3A%2F%2Fc.example%2F", "", "https://c.example/"},
	} {
		if got := xmaLink(card(c.button, c.own)); got != c.want {
			t.Errorf("xmaLink(%q, %q) = %q, want %q", c.button, c.own, got, c.want)
		}
	}
}
