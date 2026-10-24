//go:build !darwin && !windows

package autostart

const supported = false

func enabled() bool           { return false }
func enable(exe string) error { return ErrUnsupported }
func disable() error          { return ErrUnsupported }
