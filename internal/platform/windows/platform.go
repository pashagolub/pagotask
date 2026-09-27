//go:build windows

package windows

import (
	"os/exec"

	"github.com/pashagolub/pagotask/internal/gtasks"
	"github.com/pashagolub/pagotask/internal/platform"
)

// New returns the Windows implementations.
func New() (platform.PageReader, platform.Hotkey, gtasks.TokenStore) {
	return NewReader(), NewHotkey(), CredStore{}
}

// OpenFile opens a file with its default application.
func OpenFile(path string) error {
	return exec.Command("cmd", "/c", "start", "", path).Start()
}
