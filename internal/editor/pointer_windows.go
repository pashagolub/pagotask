//go:build windows

package editor

import (
	"github.com/wailsapp/wails/v3/pkg/w32"
	"golang.org/x/sys/windows"
)

var procShowCursor = windows.NewLazySystemDLL("user32.dll").NewProc("ShowCursor")

// showPointer makes the mouse pointer visible again on the UI thread.
//
// WebView2 runtime 152+ hides the pointer while typing ("Hide pointer while
// typing") and can leave it hidden for the whole app when its window goes
// away before the mouse moves (MicrosoftEdge/WebView2Feedback#5687). The
// popup hides on Enter, mid-typing, so the tray menu, which belongs to the
// same thread, then showed no pointer. Must run on the UI thread.
func showPointer() {
	// ShowCursor returns the new display count; the pointer shows at 0 or more.
	n := showCursor(true)
	for i := 0; n < 0 && i < 64; i++ {
		n = showCursor(true)
	}
	if n > 0 {
		showCursor(false) // it was visible already; keep the count balanced
	}
	w32.SetCursor(w32.LoadCursorWithResourceID(0, w32.IDC_ARROW))
}

func showCursor(show bool) int32 {
	var b uintptr
	if show {
		b = 1
	}
	r, _, _ := procShowCursor.Call(b)
	return int32(r)
}
