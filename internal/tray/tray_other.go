//go:build !darwin && !windows

package tray

func start()          {}
func stop()           {}
func setUnread(n int) {}
func hideApp()        {}
