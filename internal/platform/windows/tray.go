//go:build windows

package windows

import (
	_ "embed"
	"fmt"

	"fyne.io/systray"
)

//go:embed icon.ico
var iconICO []byte

// Tray is the notification-area icon and menu.
type Tray struct {
	onAdd, onSignIn, onSignOut, onOpenConfig, onQuit func()
	mAdd, mSignIn, mSignOut, mConfig, mQuit          *systray.MenuItem
	end                                              func()
}

// NewTray returns a Tray; call Run to show it.
func NewTray() *Tray { return &Tray{} }

func (t *Tray) OnAdd(f func())        { t.onAdd = f }
func (t *Tray) OnSignIn(f func())     { t.onSignIn = f }
func (t *Tray) OnSignOut(f func())    { t.onSignOut = f }
func (t *Tray) OnOpenConfig(f func()) { t.onOpenConfig = f }
func (t *Tray) OnQuit(f func())       { t.onQuit = f }

// Start registers the icon with systray's external-loop mode, which is what
// Wails' own message loop needs, and shows it.
func (t *Tray) Start(onReady func()) {
	start, end := systray.RunWithExternalLoop(func() {
		systray.SetIcon(iconICO)
		systray.SetTitle("pagotask")
		systray.SetTooltip("pagotask")
		t.mAdd = systray.AddMenuItem("Add task", "Open the capture popup")
		systray.AddSeparator()
		t.mSignIn = systray.AddMenuItem("Sign in to Google", "")
		t.mSignOut = systray.AddMenuItem("Sign out", "")
		t.mConfig = systray.AddMenuItem("Open config.yaml", "")
		systray.AddSeparator()
		t.mQuit = systray.AddMenuItem("Quit", "")
		go t.loop()
		if onReady != nil {
			onReady()
		}
	}, nil)
	t.end = end
	start()
}

// Stop removes the icon.
func (t *Tray) Stop() {
	if t.end != nil {
		t.end()
	}
}

func (t *Tray) loop() {
	for {
		select {
		case <-t.mAdd.ClickedCh:
			call(t.onAdd)
		case <-t.mSignIn.ClickedCh:
			call(t.onSignIn)
		case <-t.mSignOut.ClickedCh:
			call(t.onSignOut)
		case <-t.mConfig.ClickedCh:
			call(t.onOpenConfig)
		case <-t.mQuit.ClickedCh:
			call(t.onQuit)
			return
		}
	}
}

func call(f func()) {
	if f != nil {
		go f()
	}
}

// SetPending reflects queue state in the tooltip.
func (t *Tray) SetPending(pending, stuck int) {
	switch {
	case stuck > 0:
		systray.SetTooltip(fmt.Sprintf("pagotask: %d task(s) could not be saved, %d pending", stuck, pending))
	case pending > 0:
		systray.SetTooltip(fmt.Sprintf("pagotask: %d task(s) pending", pending))
	default:
		systray.SetTooltip("pagotask")
	}
}

// SetSignedIn toggles the sign-in menu items.
func (t *Tray) SetSignedIn(in bool) {
	if t.mSignIn == nil {
		return
	}
	if in {
		t.mSignIn.Hide()
		t.mSignOut.Show()
	} else {
		t.mSignIn.Show()
		t.mSignOut.Hide()
	}
}
