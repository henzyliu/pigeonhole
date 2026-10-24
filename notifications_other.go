//go:build !windows

package main

// windowsBlocksNotifications only applies on Windows; elsewhere Wails asks
// the system.
func windowsBlocksNotifications() (blocked, known bool) { return false, false }
