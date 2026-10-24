package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Images opened in another app are copies in this folder (stored media
// files have no extension, so they can't be opened directly).
func openedMediaDir() string { return filepath.Join(os.TempDir(), "Pigeonhole") }

// OpenMedia opens a photo or file from a chat in the computer's default app
// for it, e.g. Preview or Photos.
func (a *App) OpenMedia(mediaURL, fileName string) error {
	path, err := a.svc.MediaCopy(a.ctx, mediaURL, fileName, openedMediaDir())
	if err != nil {
		return err
	}
	return openWithDefaultApp(path)
}

// SaveMedia asks where to save a photo or file from a chat, then saves it.
func (a *App) SaveMedia(mediaURL, fileName string) error {
	tmp, err := a.svc.MediaCopy(a.ctx, mediaURL, fileName, openedMediaDir())
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	dest, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:            "Save",
		DefaultFilename:  filepath.Base(tmp),
		DefaultDirectory: filepath.Join(home, "Downloads"),
	})
	if err != nil || dest == "" {
		return err
	}
	return moveFile(tmp, dest)
}

// moveFile renames, or copies when dest is on another drive.
func moveFile(src, dest string) error {
	if os.Rename(src, dest) == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	in.Close()
	return os.Remove(src)
}

func openWithDefaultApp(path string) error {
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

// cleanOpenedMedia removes copies made for opening media more than a day
// ago, by which time the app they were opened in is done with them.
func cleanOpenedMedia() {
	entries, err := os.ReadDir(openedMediaDir())
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > 24*time.Hour {
			_ = os.Remove(filepath.Join(openedMediaDir(), e.Name()))
		}
	}
}
