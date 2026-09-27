// Package platform defines the OS-specific pieces. Each OS provides its own
// implementation; Windows is the first.
package platform

import "github.com/pashagolub/pagotask/internal/rules"

// PageReader reads process name, window title and (for browsers) the URL of
// the foreground window. wantURL says whether the caller needs the URL at all.
type PageReader interface {
	Foreground(wantURL bool) (rules.Capture, error)
}

// Hotkey registers one global shortcut such as "Win+Shift+T".
type Hotkey interface {
	Register(combo string, fn func()) error
	Unregister() error
}

// Tray shows the status icon and menu.
type Tray interface {
	// Start shows the icon without blocking; onReady runs once the menu is up.
	Start(onReady func())
	// Stop removes the icon.
	Stop()
	SetPending(pending, stuck int)
	SetSignedIn(bool)
	OnAdd(func())
	OnTasks(func())
	OnSignIn(func())
	OnSignOut(func())
	OnOpenConfig(func())
	OnQuit(func())
}
