package inbox

import (
	"context"
	"strings"
	"time"

	"maunium.net/go/mautrix/bridgev2/status"
)

// Connections can look alive after the computer sleeps while really being
// dead, until a ping times out minutes later, so messages arrive late or
// seem missing. Reconnecting makes each network catch up on what was missed.

// minReconnectGap stops repeated refreshes (or a refresh right after a
// wake) from reconnecting over and over.
const minReconnectGap = 10 * time.Second

// Reconnect resets every account's connection, as the Refresh button does
// and as happens after the computer wakes. It reports whether it ran.
func (s *Service) Reconnect(reason string) bool {
	s.lock.Lock()
	if time.Since(s.lastReconnect) < minReconnectGap {
		s.lock.Unlock()
		return false
	}
	s.lastReconnect = time.Now()
	s.lock.Unlock()
	s.Log.Info().Str("reason", reason).Msg("Reconnecting all accounts")
	for _, netID := range s.order {
		n := s.networks[netID]
		if !n.started.Load() {
			continue
		}
		// After a wake, a network that has already reconnected by itself is
		// left alone: resetting it again only opens a moment in which
		// anything sent fails.
		if reason == "wake" && s.justConnected(netID) {
			s.Log.Debug().Str("network", n.Name).Msg("Already reconnected, not resetting")
			continue
		}
		go func() {
			// A bridge that trips over an account in an odd state (e.g. signed
			// out) mustn't take the app down with it.
			defer func() {
				if err := recover(); err != nil {
					s.Log.Error().Any("panic", err).Str("network", n.Name).Msg("Reconnecting failed")
				}
			}()
			n.Bridge.ResetNetworkConnections()
		}()
	}
	s.emit("inbox:accounts", nil)
	return true
}

// justConnectedWindow is how recently a network must have reconnected for
// a wake-up to leave it alone.
const justConnectedWindow = 30 * time.Second

// justConnected reports whether every account on a network has reported
// connecting within justConnectedWindow.
func (s *Service) justConnected(netID string) bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	any := false
	for key, state := range s.states {
		if !strings.HasPrefix(key, netID+"/") {
			continue
		}
		any = true
		if state.StateEvent != status.StateConnected || time.Since(s.connectedAt[key]) > justConnectedWindow {
			return false
		}
	}
	return any
}

// wakeCheckInterval is how often the wake watcher looks at the clock.
const wakeCheckInterval = 15 * time.Second

// watchForWake reconnects after the computer has slept. While asleep the
// watcher doesn't run, so on waking, far more wall-clock time has passed
// since it last looked than its interval. (Wall-clock, because Go's
// monotonic clock may pause during sleep.)
func (s *Service) watchForWake(ctx context.Context) {
	ticker := time.NewTicker(wakeCheckInterval)
	defer ticker.Stop()
	last := time.Now().Round(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().Round(0)
			if slept := now.Sub(last); slept > 4*wakeCheckInterval {
				s.Log.Info().Dur("gap", slept).Msg("Computer woke up")
				s.Reconnect("wake")
			}
			last = now
		}
	}
}
