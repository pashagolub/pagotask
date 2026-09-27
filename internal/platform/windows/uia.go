//go:build windows

package windows

import (
	"log/slog"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
)

// UI Automation is used to read the address bar of the focused browser
// without touching the clipboard. Only the handful of COM calls needed are
// bound here: CUIAutomation.ElementFromHandle, CreatePropertyCondition,
// IUIAutomationElement.FindAll and the ValuePattern's CurrentValue.

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
	uiaAutomationIDProp  = 30011
	firefoxURLBarID      = "urlbar-input"
)

// vtable slots (0-based) of the interfaces we call; IUnknown occupies 0-2.
const (
	slotElementFromHandle       = 6  // IUIAutomation
	slotCreatePropertyCondition = 23 // IUIAutomation
	slotFindFirst               = 5  // IUIAutomationElement
	slotFindAll                 = 6  // IUIAutomationElement
	slotGetCurrentPattern       = 16 // IUIAutomationElement
	slotValueCurrentValue       = 4  // IUIAutomationValuePattern
	slotArrayLength             = 3  // IUIAutomationElementArray
	slotArrayGetElement         = 4  // IUIAutomationElementArray
)

const (
	sFalse          = 0x00000001
	rpcEChangedMode = 0x80010106
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
// It looks at every edit control of the window and takes the first whose
// value looks like an address: web pages have edit controls of their own
// (GitHub's search box, comment fields), so the first edit found is not
// necessarily the address bar.
func browserURL(hwnd uintptr) (url string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("uia: panic reading address bar", "err", r)
			url = ""
		}
	}()
	// COM state belongs to the OS thread; keep this goroutine on one.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	switch err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); {
	case err == nil:
		defer ole.CoUninitialize()
	case isHRESULT(err, sFalse):
		defer ole.CoUninitialize() // already initialised; balance the call
	case isHRESULT(err, rpcEChangedMode):
		// Initialised with another model by someone else: usable, not ours to undo.
	default:
		slog.Warn("uia: CoInitializeEx", "err", err)
		return ""
	}

	auto, err := ole.CreateInstance(clsidCUIAutomation, iidIUIAutomation)
	if err != nil {
		slog.Warn("uia: CreateInstance", "err", err)
		return ""
	}
	defer auto.Release()

	var root *ole.IUnknown
	if _, err := vcall(auto, slotElementFromHandle, hwnd, uintptr(unsafe.Pointer(&root))); err != nil || root == nil {
		slog.Warn("uia: ElementFromHandle", "err", err)
		return ""
	}
	defer root.Release()

	// Fast path: Firefox names its address bar, so one lookup finds it
	// without walking the whole page.
	if u, ok := asURL(firstValue(auto, root, uiaAutomationIDProp, firefoxURLBarID)); ok {
		return u
	}

	// Condition: ControlType == Edit.
	v := ole.NewVariant(ole.VT_I4, uiaEditControlType)
	var cond *ole.IUnknown
	if _, err := vcall(auto, slotCreatePropertyCondition, uiaControlTypeProp, uintptr(unsafe.Pointer(&v)), uintptr(unsafe.Pointer(&cond))); err != nil || cond == nil {
		slog.Warn("uia: CreatePropertyCondition", "err", err)
		return ""
	}
	defer cond.Release()

	var arr *ole.IUnknown
	if _, err := vcall(root, slotFindAll, treeScopeDescendants, uintptr(unsafe.Pointer(cond)), uintptr(unsafe.Pointer(&arr))); err != nil || arr == nil {
		slog.Warn("uia: FindAll", "err", err)
		return ""
	}
	defer arr.Release()

	var n int32
	if _, err := vcall(arr, slotArrayLength, uintptr(unsafe.Pointer(&n))); err != nil {
		slog.Warn("uia: element count", "err", err)
		return ""
	}
	for i := int32(0); i < n; i++ {
		var el *ole.IUnknown
		if _, err := vcall(arr, slotArrayGetElement, uintptr(i), uintptr(unsafe.Pointer(&el))); err != nil || el == nil {
			continue
		}
		val := editValue(el)
		el.Release()
		if u, ok := asURL(val); ok {
			return u
		}
	}
	slog.Warn("uia: no edit control holds an address", "edits", n)
	return ""
}

// firstValue returns the value of the first descendant whose string
// property prop equals want, or "".
func firstValue(auto, root *ole.IUnknown, prop uintptr, want string) string {
	bstr := ole.SysAllocString(want)
	defer ole.SysFreeString(bstr)
	v := ole.NewVariant(ole.VT_BSTR, int64(uintptr(unsafe.Pointer(bstr))))
	var cond *ole.IUnknown
	if _, err := vcall(auto, slotCreatePropertyCondition, prop, uintptr(unsafe.Pointer(&v)), uintptr(unsafe.Pointer(&cond))); err != nil || cond == nil {
		return ""
	}
	defer cond.Release()
	var el *ole.IUnknown
	if _, err := vcall(root, slotFindFirst, treeScopeDescendants, uintptr(unsafe.Pointer(cond)), uintptr(unsafe.Pointer(&el))); err != nil || el == nil {
		return ""
	}
	defer el.Release()
	return editValue(el)
}

// editValue reads the ValuePattern of an edit control, or "".
func editValue(el *ole.IUnknown) string {
	var pat *ole.IUnknown
	if _, err := vcall(el, slotGetCurrentPattern, uiaValuePatternID, uintptr(unsafe.Pointer(&pat))); err != nil || pat == nil {
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
	return ole.BstrToString(bstr)
}

func isHRESULT(err error, code uintptr) bool {
	oleErr, ok := err.(*ole.OleError)
	return ok && oleErr.Code() == code
}
