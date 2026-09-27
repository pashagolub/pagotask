//go:build !windows

package main

import (
	"github.com/pashagolub/pagotask/internal/gtasks"
	"github.com/pashagolub/pagotask/internal/platform"
	"github.com/pashagolub/pagotask/internal/platform/other"
)

func newPlatform() (platform.PageReader, platform.Hotkey, gtasks.TokenStore) {
	return other.New()
}

func openFile(path string) error { return other.OpenFile(path) }

func newHotkey() platform.Hotkey { return other.NewHotkey() }
