//go:build windows

package windows

import (
	_ "embed"
	"fmt"
	"runtime"

	"fyne.io/systray"
)

//go:embed icon.ico
var iconICO []byte

// Tray is the notification-area icon and menu.
type Tray struct {
	onAdd, onSignIn, onSignOut, onOpenConfig, onQuit func()
	mAdd, mSignIn, mSignOut, mConfig, mQuit          *systray.MenuItem
}

// NewTray returns a Tray; call Start to show it.
func NewTray() *Tray { return &Tray{} }

func (t *Tray) OnAdd(f func())        { t.onAdd = f }
func (t *Tray) OnSignIn(f func())     { t.onSignIn = f }
func (t *Tray) OnSignOut(f func())    { t.onSignOut = f }
func (t *Tray) OnOpenConfig(f func()) { t.onOpenConfig = f }
func (t *Tray) OnQuit(f func())       { t.onQuit = f }

// Start shows the icon on a dedicated, locked OS thread. Windows delivers a
// window's messages only to the thread that created it, so the tray window
// has to be created and pumped by the same thread; running it on Wails'
// thread (or with systray's external-loop mode, which pumps from a different
// goroutine) leaves the icon deaf to clicks.
func (t *Tray) Start(onReady func()) {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		systray.Run(func() {
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
	}()
}

// Stop removes the icon and ends the tray thread.
func (t *Tray) Stop() { systray.Quit() }

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
