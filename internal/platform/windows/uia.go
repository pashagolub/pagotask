//go:build windows

package windows

import (
	"log/slog"
	"strings"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
)

// UI Automation is used to read the address bar of the focused browser
// without touching the clipboard. Only the handful of COM calls needed are
// bound here: CUIAutomation.ElementFromHandle, CreatePropertyCondition,
// IUIAutomationElement.FindFirst and the ValuePattern's CurrentValue.

var (
	clsidCUIAutomation       = ole.NewGUID("{ff48dba4-60ef-4201-aa87-54103eef594e}")
	iidIUIAutomation         = ole.NewGUID("{30cbe57d-d9d0-452a-ab13-7ac5ac4825ee}")
	iidIUIAutomationValuePat = ole.NewGUID("{a94cd8b1-0844-4cd6-9d2d-640537ab39e9}")
)

const (
	treeScopeDescendants = 4
	uiaControlTypeProp   = 30003
	uiaValuePatternID    = 10002
	uiaEditControlType   = 50004
)

// vtable slots (0-based) of the interfaces we call; IUnknown occupies 0-2.
const (
	slotElementFromHandle       = 6  // IUIAutomation
	slotCreatePropertyCondition = 23 // IUIAutomation
	slotFindFirst               = 5  // IUIAutomationElement
	slotGetCurrentPattern       = 16 // IUIAutomationElement
	slotCurrentName             = 23 // IUIAutomationElement
	slotValueCurrentValue       = 4  // IUIAutomationValuePattern
)

func vcall(obj *ole.IUnknown, slot int, args ...uintptr) (uintptr, error) {
	vt := *(**[64]uintptr)(unsafe.Pointer(obj))
	all := append([]uintptr{uintptr(unsafe.Pointer(obj))}, args...)
	r, _, _ := syscall.SyscallN(vt[slot], all...)
	if r != 0 {
		return r, ole.NewError(r)
	}
	return r, nil
}

// browserURL returns the URL shown in the address bar of hwnd, or "".
func browserURL(hwnd uintptr) (url string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("uia: panic reading address bar", "err", r)
			url = ""
		}
	}()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		if oleErr, ok := err.(*ole.OleError); !ok || oleErr.Code() != 1 { // S_FALSE: already initialised
			return ""
		}
	}
	defer ole.CoUninitialize()

	auto, err := ole.CreateInstance(clsidCUIAutomation, iidIUIAutomation)
	if err != nil {
		slog.Warn("uia: CreateInstance", "err", err)
		return ""
	}
	defer auto.Release()

	var root *ole.IUnknown
	if _, err := vcall(auto, slotElementFromHandle, hwnd, uintptr(unsafe.Pointer(&root))); err != nil || root == nil {
		return ""
	}
	defer root.Release()

	// Condition: ControlType == Edit.
	v := ole.NewVariant(ole.VT_I4, uiaEditControlType)
	var cond *ole.IUnknown
	if _, err := vcall(auto, slotCreatePropertyCondition, uiaControlTypeProp, uintptr(unsafe.Pointer(&v)), uintptr(unsafe.Pointer(&cond))); err != nil || cond == nil {
		return ""
	}
	defer cond.Release()

	var edit *ole.IUnknown
	if _, err := vcall(root, slotFindFirst, treeScopeDescendants, uintptr(unsafe.Pointer(cond)), uintptr(unsafe.Pointer(&edit))); err != nil || edit == nil {
		return ""
	}
	defer edit.Release()

	var pat *ole.IUnknown
	if _, err := vcall(edit, slotGetCurrentPattern, uiaValuePatternID, uintptr(unsafe.Pointer(&pat))); err != nil || pat == nil {
		return ""
	}
	defer pat.Release()
	var vp *ole.IUnknown
	if err := pat.PutQueryInterface(iidIUIAutomationValuePat, &vp); err != nil || vp == nil {
		return ""
	}
	defer vp.Release()

	var bstr *uint16
	if _, err := vcall(vp, slotValueCurrentValue, uintptr(unsafe.Pointer(&bstr))); err != nil || bstr == nil {
		return ""
	}
	defer ole.SysFreeString((*int16)(unsafe.Pointer(bstr)))
	url = strings.TrimSpace(ole.BstrToString(bstr))
	// Firefox and Chrome show the bare host without scheme for http(s) pages.
	if url != "" && !strings.Contains(url, "://") && strings.Contains(url, ".") && !strings.ContainsAny(url, " ") {
		url = "https://" + url
	}
	return url
}
