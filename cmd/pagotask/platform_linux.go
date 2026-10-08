//go:build linux

package main

import (
	"fmt"
	"os"

	"github.com/pashagolub/pagotask/internal/gtasks"
	"github.com/pashagolub/pagotask/internal/platform"
	pl "github.com/pashagolub/pagotask/internal/platform/linux"
)

func newPlatform() (platform.PageReader, platform.Hotkey, gtasks.TokenStore) {
	return pl.New()
}

func openFile(path string) error { return pl.OpenFile(path) }

func newHotkey() platform.Hotkey { return pl.NewHotkey("tasks") }

// forward handles "pagotask [add|tasks]": when an instance is already
// running it gets the verb and this process exits. Otherwise this
// process starts and does the verb itself once it is up.
func forward(args []string) bool {
	verb := ""
	if len(args) > 1 {
		verb = args[1]
	}
	switch verb {
	case "", "add", "tasks":
	default:
		fmt.Fprintln(os.Stderr, "usage: pagotask [add|tasks]")
		os.Exit(2)
	}
	return pl.Forward(verb)
}
