//go:build windows

package main

import (
	"github.com/pashagolub/pagotask/internal/gtasks"
	"github.com/pashagolub/pagotask/internal/platform"
	pw "github.com/pashagolub/pagotask/internal/platform/windows"
)

func newPlatform() (platform.PageReader, platform.Hotkey, gtasks.TokenStore) {
	return pw.New()
}

func openFile(path string) error { return pw.OpenFile(path) }

func newHotkey() platform.Hotkey { return pw.NewHotkey() }
