//go:build linux

// Package linux implements the platform pieces for Linux desktops,
// GNOME on Ubuntu first: AT-SPI page reading, GNOME custom shortcuts,
// the "pagotask <verb>" commands they run, and the Secret Service keyring.
package linux

import (
	"log/slog"
	"os/exec"
	"path/filepath"

	"github.com/pashagolub/pagotask/internal/config"
	"github.com/pashagolub/pagotask/internal/gtasks"
	"github.com/pashagolub/pagotask/internal/platform"
)

// New returns the Linux implementations and starts listening for
// "pagotask <verb>" runs.
func New() (platform.PageReader, platform.Hotkey, gtasks.TokenStore) {
	if err := listen(SocketPath()); err != nil {
		slog.Error("listen for shortcut commands", "err", err)
	}
	dir, _ := config.Dir()
	return NewReader(), NewHotkey("add"), TokenStore{File: gtasks.FileTokenStore{Path: filepath.Join(dir, "token.json")}}
}

// OpenFile opens a file with the desktop's default application.
func OpenFile(path string) error { return exec.Command("xdg-open", path).Start() }
