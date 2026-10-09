//go:build linux

package main

import (
	"github.com/pashagolub/pagotask/internal/gtasks"
	"github.com/pashagolub/pagotask/internal/platform"
	pl "github.com/pashagolub/pagotask/internal/platform/linux"
)

func newPlatform() (platform.PageReader, platform.Hotkey, gtasks.TokenStore) {
	return pl.New()
}

func openFile(path string) error { return pl.OpenFile(path) }

func newHotkey() platform.Hotkey { return pl.NewHotkey("tasks") }
