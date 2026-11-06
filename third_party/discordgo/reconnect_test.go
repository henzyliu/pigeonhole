package discordgo

import (
	"testing"
	"time"
)

// A stopped session, or one already reconnecting, must not start another
// reconnect loop (each loop would try to log in again).
func TestReconnectRespectsStopAndSingleFlight(t *testing.T) {
	for name, setup := range map[string]func(s *Session){
		"stopped":      func(s *Session) { s.stopped.Store(true) },
		"reconnecting": func(s *Session) { s.reconnecting.Store(true) },
	} {
		t.Run(name, func(t *testing.T) {
			s := &Session{ShouldReconnectOnError: true}
			tried := false
			s.BeforeReconnect = func(*Session) { tried = true }
			setup(s)
			done := make(chan struct{})
			go func() { s.reconnect(); close(done) }()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("reconnect kept running")
			}
			if tried {
				t.Fatal("reconnect tried to connect")
			}
		})
	}
}
