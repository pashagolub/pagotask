//go:build windows

package windows

import (
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/pashagolub/pagotask/internal/rules"
)

// Reader reads the foreground window. Process name and title come from
// Win32; the browser URL comes from UI Automation (see uia.go).
type Reader struct{}

// NewReader returns a Reader.
func NewReader() *Reader { return &Reader{} }

// Foreground implements platform.PageReader.
func (Reader) Foreground(wantURL bool) (rules.Capture, error) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return rules.Capture{}, nil
	}
	cap := rules.Capture{Title: windowTitle(hwnd), Process: processName(hwnd)}
	if wantURL {
		cap.URL = browserURL(hwnd)
	}
	return cap, nil
}

func windowTitle(hwnd uintptr) string {
	n, _, _ := procGetWindowTextLengthW.Call(hwnd)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf)
}

func processName(hwnd uintptr) string {
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ""
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return filepath.Base(windows.UTF16ToString(buf[:size]))
}
