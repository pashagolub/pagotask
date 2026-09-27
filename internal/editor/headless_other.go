//go:build !windows

package editor

import "log/slog"

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

func (h *headless) Quit() { close(h.quit) }
