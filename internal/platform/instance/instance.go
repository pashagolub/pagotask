// Package instance keeps pagotask single-instance and lets "pagotask add"
// and "pagotask tasks" reach the running instance. Desktop shortcuts
// (GNOME, PowerToys, AutoHotkey...) run those commands; the command passes
// its verb over a Unix socket and exits. With no instance running it starts
// one, which then does the verb.
package instance

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	handlersMu sync.Mutex
	handlers   = map[string]func(){}
	pending    = map[string]bool{} // verbs that arrived before their handler
)

// SocketPath is where the running instance listens: XDG_RUNTIME_DIR on
// Linux, the user's local cache directory elsewhere (%LOCALAPPDATA% on
// Windows, which has Unix sockets since Windows 10 1803).
func SocketPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			base = os.TempDir()
		}
		dir = filepath.Join(base, "pagotask")
		_ = os.MkdirAll(dir, 0o700)
	}
	return filepath.Join(dir, "pagotask.sock")
}

// Forward hands verb ("" just checks for an instance) to the running
// instance and reports whether one took it. When none did, the verb runs
// in this process once its handler is registered.
func Forward(verb string) bool {
	if send(SocketPath(), verb) == nil {
		return true
	}
	if verb != "" {
		handlersMu.Lock()
		pending[verb] = true
		handlersMu.Unlock()
	}
	return false
}

func send(path, verb string) error {
	c, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		return err
	}
	allowForeground() // this process holds the user's key press; pass it on
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := fmt.Fprintln(c, verb); err != nil {
		return err
	}
	reply, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(reply) != "ok" {
		return fmt.Errorf("instance answered %q", strings.TrimSpace(reply))
	}
	return nil
}

// Listen serves verbs from later "pagotask <verb>" runs.
func Listen() error { return listen(SocketPath()) }

func listen(path string) error {
	_ = os.Remove(path) // stale: Forward found nobody listening
	l, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				slog.Warn("ipc accept", "err", err)
				return
			}
			go serve(c)
		}
	}()
	return nil
}

func serve(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		return
	}
	verb := strings.TrimSpace(line)
	if verb != "" {
		run(verb)
	}
	_, _ = fmt.Fprintln(c, "ok")
}

// run calls the verb's handler, or keeps it for when one registers.
func run(verb string) {
	handlersMu.Lock()
	fn := handlers[verb]
	if fn == nil {
		pending[verb] = true
	}
	handlersMu.Unlock()
	if fn != nil {
		go fn()
	}
}

// Handle sets the function a verb runs; nil removes it. A verb that
// arrived before its handler runs as soon as one is set.
func Handle(verb string, fn func()) {
	handlersMu.Lock()
	if fn == nil {
		delete(handlers, verb)
	} else {
		handlers[verb] = fn
	}
	due := fn != nil && pending[verb]
	delete(pending, verb)
	handlersMu.Unlock()
	if due {
		go fn()
	}
}
