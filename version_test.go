package main

import "testing"

func TestVersionFromWailsJSON(t *testing.T) {
	if v := versionInfo(); v.Version == "" {
		t.Fatal("expected a version from wails.json")
	}
}
