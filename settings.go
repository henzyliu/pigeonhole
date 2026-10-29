package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"unified-inbox/internal/appearance"
)

// Settings are the user's app preferences, stored as settings.json in the
// data directory.
type Settings struct {
	// Theme is "system", "light" or "dark".
	Theme string `json:"theme"`
	// NotificationsOff silences desktop alerts. Stored as "off" so existing
	// settings files, which lack it, keep notifications on.
	NotificationsOff bool `json:"notificationsOff"`
	// HideMessageText shows alerts as "New message" instead of the text.
	HideMessageText bool `json:"hideMessageText"`
	// MutedAccounts lists accounts whose alerts are off, as AccountKey.
	// Their chats still sync and count as unread.
	MutedAccounts []string `json:"mutedAccounts,omitempty"`
	// UpdateNotified is the newest version a notification was sent for, so
	// each update is announced once.
	UpdateNotified string `json:"updateNotified,omitempty"`
	// BackgroundNoticeShown records that the "still running" notice has
	// been shown, after the window was first closed.
	BackgroundNoticeShown bool `json:"backgroundNoticeShown"`
	// WelcomeDone records that the first-run welcome was finished (or
	// skipped), so it isn't shown again.
	WelcomeDone bool `json:"welcomeDone"`
	// SingleChatList shows People, Groups and Marketplace chats in one list
	// instead of separate tabs. Stored this way round so tabs are the
	// default, including for settings saved before it existed.
	SingleChatList bool `json:"singleChatList"`
	// SpellCheckOff stops misspellings being underlined in the message box.
	// Stored this way round so spell check is on by default.
	SpellCheckOff bool `json:"spellCheckOff"`
}

func settingsPath(dir string) string { return filepath.Join(dir, "settings.json") }

func loadSettings(dir string) Settings {
	s := Settings{Theme: appearance.System}
	data, err := os.ReadFile(settingsPath(dir))
	if err == nil {
		_ = json.Unmarshal(data, &s)
	}
	if !appearance.Valid(s.Theme) {
		s.Theme = appearance.System
	}
	return s
}

func (s Settings) save(dir string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := settingsPath(dir) + ".tmp"
	if err = os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, settingsPath(dir))
}

// Dark matches the UI's dark --bg colour, so a dark-themed window doesn't
// flash white while the page loads.
var (
	lightBackground = &options.RGBA{R: 255, G: 255, B: 255, A: 1}
	darkBackground  = &options.RGBA{R: 0x17, G: 0x18, B: 0x1c, A: 1}
)

// windowAppearance returns the initial window options for a theme.
func windowAppearance(theme string) (*options.RGBA, *mac.Options, *windows.Options) {
	switch theme {
	case appearance.Dark:
		return darkBackground, &mac.Options{Appearance: mac.NSAppearanceNameDarkAqua}, &windows.Options{Theme: windows.Dark}
	case appearance.Light:
		return lightBackground, &mac.Options{Appearance: mac.NSAppearanceNameAqua}, &windows.Options{Theme: windows.Light}
	default:
		return lightBackground, &mac.Options{}, &windows.Options{Theme: windows.SystemDefault}
	}
}

// applyTheme switches native window chrome to match the theme. Wails'
// WindowSet*Theme only works on Windows, so macOS uses the appearance package.
func applyTheme(ctx context.Context, theme string) {
	appearance.Apply(theme)
	switch theme {
	case appearance.Dark:
		runtime.WindowSetDarkTheme(ctx)
	case appearance.Light:
		runtime.WindowSetLightTheme(ctx)
	default:
		runtime.WindowSetSystemDefaultTheme(ctx)
	}
}

// GetSettings returns the saved preferences.
func (a *App) GetSettings() Settings {
	a.settingsLock.Lock()
	defer a.settingsLock.Unlock()
	return a.settings
}

// updateSettings applies change to a copy of the settings, saves it, and
// only then makes it current.
func (a *App) updateSettings(change func(*Settings)) error {
	a.settingsLock.Lock()
	defer a.settingsLock.Unlock()
	next := a.settings
	change(&next)
	if err := next.save(a.dataDir); err != nil {
		return errors.New("couldn't save the setting: " + err.Error())
	}
	a.settings = next
	return nil
}

// SetTheme saves and applies "system", "light" or "dark".
func (a *App) SetTheme(theme string) error {
	if !appearance.Valid(theme) {
		return fmt.Errorf("unknown theme %q", theme)
	}
	if err := a.updateSettings(func(s *Settings) { s.Theme = theme }); err != nil {
		return err
	}
	applyTheme(a.ctx, theme)
	return nil
}

// SetNotifications turns desktop alerts on or off. Turning them on asks the
// system for permission, which macOS only prompts for if never answered.
func (a *App) SetNotifications(on bool) error {
	if err := a.updateSettings(func(s *Settings) { s.NotificationsOff = !on }); err != nil {
		return err
	}
	if on {
		a.requestNotificationPermission()
	}
	return nil
}

// AccountKey identifies an account in MutedAccounts.
func AccountKey(network, loginID string) string { return network + "/" + loginID }

func (s Settings) accountMuted(network, loginID string) bool {
	return slices.Contains(s.MutedAccounts, AccountKey(network, loginID))
}

// SetAccountNotifications turns one account's alerts on or off.
func (a *App) SetAccountNotifications(network, loginID string, on bool) error {
	key := AccountKey(network, loginID)
	return a.updateSettings(func(s *Settings) {
		s.MutedAccounts = slices.DeleteFunc(slices.Clone(s.MutedAccounts), func(k string) bool { return k == key })
		if !on {
			s.MutedAccounts = append(s.MutedAccounts, key)
		}
	})
}

// SetHideMessageText chooses whether alerts show the message text.
// ShowWelcome reports whether to show the first-run welcome: it hasn't
// been finished, and there are no chats yet (so people who used Pigeonhole
// before it had a welcome don't get one).
func (a *App) ShowWelcome() bool {
	return !a.GetSettings().WelcomeDone && !a.svc.HasChats(a.ctx)
}

// SetSpellCheck turns spell checking in the message box on or off.
func (a *App) SetSpellCheck(on bool) error {
	return a.updateSettings(func(s *Settings) { s.SpellCheckOff = !on })
}

// SetSeparateChats turns the People / Groups / Marketplace tabs on or off.
func (a *App) SetSeparateChats(on bool) error {
	return a.updateSettings(func(s *Settings) { s.SingleChatList = !on })
}

// FinishWelcome stops the welcome from showing again.
func (a *App) FinishWelcome() error {
	return a.updateSettings(func(s *Settings) { s.WelcomeDone = true })
}

func (a *App) SetHideMessageText(hide bool) error {
	return a.updateSettings(func(s *Settings) { s.HideMessageText = hide })
}
