//go:build !windows

package editor

import (
	"log/slog"

	"github.com/pashagolub/pagotask/internal/platform"
)

type headless struct {
	cb   Callbacks
	quit chan struct{}
}

// New returns a stand-in that logs drafts instead of showing a window.
func New() Editor { return &headless{quit: make(chan struct{})} }

func (h *headless) Run(cb Callbacks) error {
	h.cb = cb
	if cb.OnStart != nil {
		cb.OnStart()
	}
	<-h.quit
	if cb.OnStop != nil {
		cb.OnStop()
	}
	return nil
}

func (h *headless) Open(d Draft) {
	slog.Info("editor (headless): would open", "draft", d)
}

func (h *headless) OpenTasks() {
	if h.cb.OnTasksOpen != nil {
		h.cb.OnTasksOpen()
	}
	slog.Info("editor (headless): would show open tasks", "rows", len(h.cb.Tasks().Rows))
}

func (h *headless) TasksChanged() {}

func (h *headless) Quit() { close(h.quit) }

func (h *headless) Tray() platform.Tray { return nopTray{} }

// nopTray has no icon: the headless build has nothing to click.
type nopTray struct{}

func (nopTray) Start(onReady func()) {
	if onReady != nil {
		onReady()
	}
}
func (nopTray) Stop()               {}
func (nopTray) SetPending(int, int) {}
func (nopTray) SetSignedIn(bool)    {}
func (nopTray) OnAdd(func())        {}
func (nopTray) OnTasks(func())      {}
func (nopTray) OnSignIn(func())     {}
func (nopTray) OnSignOut(func())    {}
func (nopTray) OnOpenConfig(func()) {}
func (nopTray) OnQuit(func())       {}
