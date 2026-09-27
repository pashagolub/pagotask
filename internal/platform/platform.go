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
	// Run blocks until the tray quits. onReady runs once the icon is up.
	Run(onReady func(), onExit func())
	SetPending(pending, stuck int)
	SetSignedIn(bool)
	OnAdd(func())
	OnSignIn(func())
	OnSignOut(func())
	OnOpenConfig(func())
	OnQuit(func())
	Quit()
}
