//go:build !windows

// Package other is a stand-in for platforms not yet implemented. It lets
// the app compile and run headless for development on Linux/macOS: no
// hotkey and no page reading.
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

func (reader) Foreground(func(string) config.Source) (rules.Capture, error) {
	return rules.Capture{}, nil
}

type hotkey struct{}

func (hotkey) Register(string, func()) error {
	return errors.New("global hotkey is not implemented on this OS yet")
}
func (hotkey) Unregister() error { return nil }

// New returns the headless implementations.
func New() (platform.PageReader, platform.Hotkey, gtasks.TokenStore) {
	dir, _ := config.Dir()
	return reader{}, hotkey{}, gtasks.FileTokenStore{Path: filepath.Join(dir, "token.json")}
}

// NewHotkey returns another (unimplemented) hotkey.
func NewHotkey() platform.Hotkey { return hotkey{} }

// OpenFile opens a file with the desktop's default application.
func OpenFile(path string) error { return exec.Command("xdg-open", path).Start() }
