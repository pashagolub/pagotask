//go:build linux

// Package linux implements the platform pieces for Linux desktops,
// GNOME on Ubuntu first: GNOME custom shortcuts and the "pagotask <verb>"
// commands they run.
package linux

import (
	"log/slog"
	"os/exec"
	"path/filepath"

	"github.com/pashagolub/pagotask/internal/config"
	"github.com/pashagolub/pagotask/internal/gtasks"
	"github.com/pashagolub/pagotask/internal/platform"
	"github.com/pashagolub/pagotask/internal/rules"
)

// New returns the Linux implementations and starts listening for
// "pagotask <verb>" runs.
func New() (platform.PageReader, platform.Hotkey, gtasks.TokenStore) {
	if err := listen(SocketPath()); err != nil {
		slog.Error("listen for shortcut commands", "err", err)
	}
	dir, _ := config.Dir()
	return reader{}, NewHotkey("add"), gtasks.FileTokenStore{Path: filepath.Join(dir, "token.json")}
}

// reader reads nothing yet; the popup opens empty.
type reader struct{}

func (reader) Foreground(func(string) config.Source) (rules.Capture, error) {
	return rules.Capture{}, nil
}

// OpenFile opens a file with the desktop's default application.
func OpenFile(path string) error { return exec.Command("xdg-open", path).Start() }
