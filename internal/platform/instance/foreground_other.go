//go:build !windows

package instance

// allowForeground is a Windows matter; other desktops need nothing here.
func allowForeground() {}
