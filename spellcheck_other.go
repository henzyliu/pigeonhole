//go:build !darwin

package main

// enableSpellChecking is only needed on macOS: WebView2 checks spelling
// in text boxes by default.
func enableSpellChecking() {}
