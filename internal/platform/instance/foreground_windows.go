package instance

import "golang.org/x/sys/windows"

var procAllowSetForegroundWindow = windows.NewLazySystemDLL("user32.dll").NewProc("AllowSetForegroundWindow")

// allowForeground lets the running instance bring its popup to the front.
// Windows gives that right to the process the user's key press started
// (this one, launched by e.g. PowerToys), not to the instance it wakes.
func allowForeground() {
	const asfwAny = ^uintptr(0) // ASFW_ANY
	procAllowSetForegroundWindow.Call(asfwAny)
}
