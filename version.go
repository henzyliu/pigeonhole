package main

import (
	_ "embed"
	"encoding/json"
)

// The version number lives in wails.json (Wails also stamps it into the Mac
// and Windows apps); embedding it keeps that the only place to change it.
//
//go:embed wails.json
var wailsJSON []byte

// Set at build time by scripts/build.sh, with -ldflags "-X main.commit=...".
// Empty in builds made another way, e.g. wails dev.
var (
	commit    string
	branch    string
	buildDate string
)

// VersionInfo identifies this build, for Settings and the log.
type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Branch    string `json:"branch"`
	BuildDate string `json:"buildDate"`
}

func versionInfo() VersionInfo {
	var cfg struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	_ = json.Unmarshal(wailsJSON, &cfg)
	return VersionInfo{Version: cfg.Info.ProductVersion, Commit: commit, Branch: branch, BuildDate: buildDate}
}

// Version returns this build's version, commit, branch and build date.
func (a *App) Version() VersionInfo { return versionInfo() }
