//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// On Windows the update is the release's installer. A small detached script
// runs it silently (it closes Pigeonhole first, see project.nsi), then
// starts the installed copy, which lives in %LocalAppData%\Programs.

func canInstall() error { return nil }

func installedExe() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Pigeonhole", "Pigeonhole.exe")
}

func install(setup string) error {
	script := filepath.Join(filepath.Dir(setup), "install-update.cmd")
	body := fmt.Sprintf("@echo off\r\n\"%s\" /S\r\nstart \"\" \"%s\"\r\n", setup, installedExe())
	if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
		return err
	}
	cmd := exec.Command("cmd", "/C", script)
	const detachedProcess, newProcessGroup = 0x00000008, 0x00000200
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: detachedProcess | newProcessGroup}
	return cmd.Start()
}
