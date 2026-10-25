package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"unified-inbox/internal/update"
)

// Pigeonhole checks its GitHub releases for a newer version shortly after
// starting and every few hours, and can download and install one itself
// (see internal/update). The window follows along through "update:status"
// events.

const (
	firstUpdateCheck = 30 * time.Second
	updateCheckEvery = 6 * time.Hour
)

// UpdateStatus is what Settings and the update banner show.
type UpdateStatus struct {
	// State is "idle", "checking", "upToDate", "available", "downloading",
	// "installing" or "error".
	State   string          `json:"state"`
	Current string          `json:"current"`
	Latest  *update.Release `json:"latest"`
	// Progress is 0–1 while downloading.
	Progress float64 `json:"progress"`
	Error    string  `json:"error"`
	// CanInstall is false when this copy can't replace itself (then the
	// download page is offered instead).
	CanInstall bool `json:"canInstall"`
}

type updater struct {
	lock   sync.Mutex
	status UpdateStatus
}

func (a *App) setUpdateStatus(change func(*UpdateStatus)) UpdateStatus {
	a.updates.lock.Lock()
	change(&a.updates.status)
	s := a.updates.status
	a.updates.lock.Unlock()
	runtime.EventsEmit(a.ctx, "update:status", s)
	return s
}

// GetUpdateStatus returns the latest update status.
func (a *App) GetUpdateStatus() UpdateStatus {
	a.updates.lock.Lock()
	defer a.updates.lock.Unlock()
	s := a.updates.status
	if s.State == "" {
		s.State = "idle"
	}
	s.Current = versionInfo().Version
	return s
}

func (a *App) startUpdateChecks() {
	go func() {
		timer := time.NewTimer(firstUpdateCheck)
		defer timer.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-timer.C:
			}
			if s := a.CheckForUpdates(); s.State == "available" {
				a.notifyUpdate(s.Latest)
			}
			timer.Reset(updateCheckEvery)
		}
	}()
}

// CheckForUpdates asks GitHub for the newest release.
func (a *App) CheckForUpdates() UpdateStatus {
	a.updates.lock.Lock()
	busy := a.updates.status.State == "downloading" || a.updates.status.State == "installing"
	a.updates.lock.Unlock()
	if busy {
		return a.GetUpdateStatus()
	}
	current := versionInfo().Version
	a.setUpdateStatus(func(s *UpdateStatus) { s.State, s.Error, s.Current = "checking", "", current })
	ctx, cancel := context.WithTimeout(a.ctx, time.Minute)
	defer cancel()
	latest, err := update.Latest(ctx)
	if err != nil {
		a.svc.Log.Debug().Err(err).Msg("Update check failed")
		return a.setUpdateStatus(func(s *UpdateStatus) {
			s.State, s.Error = "error", "Couldn't check for updates. Check your connection and try again."
		})
	}
	if !update.Newer(latest.Version, current) {
		return a.setUpdateStatus(func(s *UpdateStatus) { s.State, s.Latest = "upToDate", latest })
	}
	canInstall := latest.AssetURL != "" && update.CanInstall() == nil
	a.svc.Log.Info().Str("current", current).Str("latest", latest.Version).Bool("can_install", canInstall).
		Msg("Update available")
	return a.setUpdateStatus(func(s *UpdateStatus) {
		s.State, s.Latest, s.CanInstall, s.Progress = "available", latest, canInstall, 0
	})
}

// notifyUpdate sends one notification per new version.
func (a *App) notifyUpdate(r *update.Release) {
	settings := a.GetSettings()
	if settings.NotificationsOff || settings.UpdateNotified == r.Version {
		return
	}
	err := runtime.SendNotification(a.ctx, runtime.NotificationOptions{
		ID:    "pigeonhole-update",
		Title: "Pigeonhole " + r.Version + " is available",
		Body:  "Open Settings to install it.",
		Data:  map[string]any{"update": true},
	})
	if err == nil {
		_ = a.updateSettings(func(s *Settings) { s.UpdateNotified = r.Version })
	}
}

// InstallUpdate downloads the available update, installs it and quits;
// the new version then starts by itself.
func (a *App) InstallUpdate() error {
	a.updates.lock.Lock()
	s := a.updates.status
	a.updates.lock.Unlock()
	if s.State != "available" || s.Latest == nil {
		return errors.New("there's no update to install")
	}
	if !s.CanInstall {
		return a.OpenUpdatePage()
	}
	a.setUpdateStatus(func(s *UpdateStatus) { s.State, s.Progress, s.Error = "downloading", 0, "" })
	dir := filepath.Join(os.TempDir(), "Pigeonhole-update")
	_ = os.RemoveAll(dir)
	file, err := update.Download(a.ctx, s.Latest, dir, func(done, total int64) {
		if total > 0 {
			a.setUpdateStatus(func(s *UpdateStatus) { s.Progress = float64(done) / float64(total) })
		}
	})
	if err == nil {
		a.setUpdateStatus(func(s *UpdateStatus) { s.State = "installing" })
		err = update.Install(file)
	}
	if err != nil {
		a.svc.Log.Warn().Err(err).Str("version", s.Latest.Version).Msg("Update failed")
		a.setUpdateStatus(func(st *UpdateStatus) {
			st.State, st.Error = "available", "The update couldn't be installed. You can download it instead."
			st.CanInstall = false
		})
		return err
	}
	a.svc.Log.Info().Str("version", s.Latest.Version).Msg("Installing update; quitting so it can replace this version")
	// Give the window a moment to show "Restarting…".
	time.AfterFunc(700*time.Millisecond, func() { runtime.Quit(a.ctx) })
	return nil
}

// OpenUpdatePage opens the latest release's page, to download by hand.
func (a *App) OpenUpdatePage() error {
	url := "https://github.com/henzyliu/pigeonhole/releases/latest"
	if s := a.GetUpdateStatus(); s.Latest != nil && s.Latest.PageURL != "" {
		url = s.Latest.PageURL
	}
	runtime.BrowserOpenURL(a.ctx, url)
	return nil
}
