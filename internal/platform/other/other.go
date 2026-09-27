//go:build !windows

// Package other is a stand-in for platforms not yet implemented. It lets
// the app compile and run headless for development on Linux/macOS: no
// tray, no hotkey, no page reading.
package other

import (
	"errors"
	"os/exec"
	"path/filepath"

	"github.com/pashagolub/pagotask/internal/config"
	"github.com/pashagolub/pagotask/internal/gtasks"
	"github.com/pashagolub/pagotask/internal/platform"
	"github.com/pashagolub/pagotask/internal/rules"
)

type reader struct{}

func (reader) Foreground(bool) (rules.Capture, error) { return rules.Capture{}, nil }

type hotkey struct{}

func (hotkey) Register(string, func()) error {
	return errors.New("global hotkey is not implemented on this OS yet")
}
func (hotkey) Unregister() error { return nil }

type tray struct{}

func (t *tray) Start(onReady func()) {
	if onReady != nil {
		onReady()
	}
}
func (t *tray) Stop()               {}
func (t *tray) SetPending(int, int) {}
func (t *tray) SetSignedIn(bool)    {}
func (t *tray) OnAdd(func())        {}
func (t *tray) OnSignIn(func())     {}
func (t *tray) OnSignOut(func())    {}
func (t *tray) OnOpenConfig(func()) {}
func (t *tray) OnQuit(func())       {}

// New returns the headless implementations.
func New() (platform.PageReader, platform.Hotkey, platform.Tray, gtasks.TokenStore) {
	dir, _ := config.Dir()
	return reader{}, hotkey{}, &tray{}, gtasks.FileTokenStore{Path: filepath.Join(dir, "token.json")}
}

// OpenFile opens a file with the desktop's default application.
func OpenFile(path string) error { return exec.Command("xdg-open", path).Start() }
