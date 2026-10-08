//go:build windows

package windows

import (
	"log/slog"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
)

// UI Automation is used to read the address bar of the focused browser, or
// one named control of another app, without touching the clipboard. Only the
// handful of COM calls needed are bound here: CUIAutomation.ElementFromHandle,
// GetFocusedElement, CreatePropertyCondition, IUIAutomationElement.FindFirst,
// FindAll, CurrentName, CurrentAutomationId and the ValuePattern's CurrentValue.

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
	uiaNameProp          = 30005
	firefoxURLBarID      = "urlbar-input"
)

// vtable slots (0-based) of the interfaces we call; IUnknown occupies 0-2.
const (
	slotElementFromHandle       = 6  // IUIAutomation
	slotGetFocusedElement       = 8  // IUIAutomation
	slotCreatePropertyCondition = 23 // IUIAutomation
	slotFindFirst               = 5  // IUIAutomationElement
	slotFindAll                 = 6  // IUIAutomationElement
	slotGetCurrentPattern       = 16 // IUIAutomationElement
	slotCurrentName             = 23 // IUIAutomationElement
	slotCurrentAutomationID     = 29 // IUIAutomationElement
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

// withUIA runs fn with a UI Automation instance and the element of hwnd,
// on one locked OS thread with COM initialised. It returns "" on any failure.
func withUIA(hwnd uintptr, fn func(auto, root *ole.IUnknown) string) (out string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("uia: panic", "err", r)
			out = ""
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
	return fn(auto, root)
}

// browserURL returns the URL shown in the address bar of hwnd, or "".
// It looks at every edit control of the window and takes the first whose
// value looks like an address: web pages have edit controls of their own
// (GitHub's search box, comment fields), so the first edit found is not
// necessarily the address bar.
func browserURL(hwnd uintptr) string {
	return withUIA(hwnd, findURL)
}

func findURL(auto, root *ole.IUnknown) string {
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

// controlText returns the text of the first control of hwnd whose
// AutomationId, or else Name, is want: its value when it has one (edit
// fields), else its name (labels). The focused control is logged either way
// so the id of a field can be found by clicking into it and pressing the hotkey.
func controlText(hwnd uintptr, want string) string {
	return withUIA(hwnd, func(auto, root *ole.IUnknown) string {
		logFocused(auto)
		if el := findFirst(auto, root, uiaAutomationIDProp, want); el != nil {
			defer el.Release()
			if v := editValue(el); v != "" {
				return v
			}
			return elementString(el, slotCurrentName)
		}
		if el := findFirst(auto, root, uiaNameProp, want); el != nil {
			defer el.Release()
			return editValue(el)
		}
		slog.Warn("uia: no control with this id or name", "control", want)
		return ""
	})
}

// logFocused logs the id, name and value of the focused control.
func logFocused(auto *ole.IUnknown) {
	var el *ole.IUnknown
	if _, err := vcall(auto, slotGetFocusedElement, uintptr(unsafe.Pointer(&el))); err != nil || el == nil {
		return
	}
	defer el.Release()
	slog.Info("uia: focused control",
		"id", elementString(el, slotCurrentAutomationID),
		"name", elementString(el, slotCurrentName),
		"value", editValue(el))
}

// elementString reads a BSTR property getter of an element, or "".
func elementString(el *ole.IUnknown, slot int) string {
	var bstr *uint16
	if _, err := vcall(el, slot, uintptr(unsafe.Pointer(&bstr))); err != nil || bstr == nil {
		return ""
	}
	defer ole.SysFreeString((*int16)(unsafe.Pointer(bstr)))
	return ole.BstrToString(bstr)
}

// findFirst returns the first descendant whose string property prop equals
// want, or nil. The caller releases it.
func findFirst(auto, root *ole.IUnknown, prop uintptr, want string) *ole.IUnknown {
	bstr := ole.SysAllocString(want)
	defer ole.SysFreeString(bstr)
	v := ole.NewVariant(ole.VT_BSTR, int64(uintptr(unsafe.Pointer(bstr))))
	var cond *ole.IUnknown
	if _, err := vcall(auto, slotCreatePropertyCondition, prop, uintptr(unsafe.Pointer(&v)), uintptr(unsafe.Pointer(&cond))); err != nil || cond == nil {
		return nil
	}
	defer cond.Release()
	var el *ole.IUnknown
	if _, err := vcall(root, slotFindFirst, treeScopeDescendants, uintptr(unsafe.Pointer(cond)), uintptr(unsafe.Pointer(&el))); err != nil || el == nil {
		return nil
	}
	return el
}

// firstValue returns the value of the first descendant whose string
// property prop equals want, or "".
func firstValue(auto, root *ole.IUnknown, prop uintptr, want string) string {
	el := findFirst(auto, root, prop, want)
	if el == nil {
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
