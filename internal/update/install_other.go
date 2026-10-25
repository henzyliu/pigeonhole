//go:build !darwin && !windows

package update

func canInstall() error         { return ErrCantInstall }
func install(file string) error { return ErrCantInstall }
