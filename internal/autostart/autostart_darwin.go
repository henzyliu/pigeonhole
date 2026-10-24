//go:build darwin

package autostart

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
)

const supported = true

// A per-user LaunchAgent. macOS lists it under System Settings → General →
// Login Items, where it can be switched off too.
const label = "com.pigeonhole.app"

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist")
}

func enabled() bool {
	_, err := os.Stat(plistPath())
	return err == nil
}

func enable(exe string) error {
	var args bytes.Buffer
	for _, a := range []string{exe, BackgroundFlag} {
		args.WriteString("\t\t<string>")
		_ = xml.EscapeText(&args, []byte(a))
		args.WriteString("</string>\n")
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + label + `</string>
	<key>ProgramArguments</key>
	<array>
` + args.String() + `	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>LimitLoadToSessionType</key>
	<string>Aqua</string>
</dict>
</plist>
`
	if err := os.MkdirAll(filepath.Dir(plistPath()), 0o755); err != nil {
		return err
	}
	if current, err := os.ReadFile(plistPath()); err == nil && string(current) == plist {
		return nil
	}
	return os.WriteFile(plistPath(), []byte(plist), 0o644)
}

func disable() error {
	err := os.Remove(plistPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
