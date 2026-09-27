//go:build windows

// Package windows implements the platform interfaces with Win32.
package windows

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/pashagolub/pagotask/internal/platform"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey           = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey         = user32.NewProc("UnregisterHotKey")
	procGetMessageW              = user32.NewProc("GetMessageW")
	procPostThreadMessageW       = user32.NewProc("PostThreadMessageW")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
)

const (
	modAlt      = 0x0001
	modControl  = 0x0002
	modShift    = 0x0004
	modWin      = 0x0008
	modNoRepeat = 0x4000
	wmHotkey    = 0x0312
	wmQuit      = 0x0012
	hotkeyID    = 1
)

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

// Hotkey registers a global shortcut with RegisterHotKey and pumps messages
// on a locked OS thread.
type Hotkey struct {
	threadID uint32
	done     chan struct{}
}

// NewHotkey returns an unregistered hotkey.
func NewHotkey() *Hotkey { return &Hotkey{} }

// Register parses combo and starts the message loop calling fn on each press.
func (h *Hotkey) Register(combo string, fn func()) error {
	c, err := platform.ParseCombo(combo)
	if err != nil {
		return err
	}
	var mods uintptr = modNoRepeat
	if c.Win {
		mods |= modWin
	}
	if c.Ctrl {
		mods |= modControl
	}
	if c.Alt {
		mods |= modAlt
	}
	if c.Shift {
		mods |= modShift
	}
	vk := uintptr(c.Key[0]) // letters and digits map to their ASCII virtual-key codes
	if c.Key == "SPACE" {
		vk = 0x20
	}
	errCh := make(chan error, 1)
	h.done = make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		h.threadID = windows.GetCurrentThreadId()
		r, _, e := procRegisterHotKey.Call(0, hotkeyID, mods, vk)
		if r == 0 {
			errCh <- fmt.Errorf("RegisterHotKey(%s): %v (is it taken by another app?)", combo, e)
			return
		}
		errCh <- nil
		var m msg
		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if r == 0 || int32(r) == -1 {
				break
			}
			if m.message == wmHotkey && m.wParam == hotkeyID {
				fn()
			}
		}
		procUnregisterHotKey.Call(0, hotkeyID)
		close(h.done)
	}()
	return <-errCh
}

// Unregister stops the message loop.
func (h *Hotkey) Unregister() error {
	if h.threadID == 0 {
		return nil
	}
	procPostThreadMessageW.Call(uintptr(h.threadID), wmQuit, 0, 0)
	<-h.done
	h.threadID = 0
	return nil
}
