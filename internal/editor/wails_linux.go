//go:build linux

package editor

import (
	_ "embed"
	"os"
)

// The Linux tray (StatusNotifierItem) takes a PNG.
//
//go:embed icon.png
var trayIcon []byte

// Wayland lets no app place its own window or keep it on top, so the
// popups run through XWayland, where centring and always-on-top work.
// GDK_BACKEND set by the user wins.
func init() {
	if os.Getenv("WAYLAND_DISPLAY") != "" && os.Getenv("GDK_BACKEND") == "" {
		_ = os.Setenv("GDK_BACKEND", "x11")
	}
}

// showPointer is a Windows WebView2 workaround; nothing to do here.
func showPointer() {}
