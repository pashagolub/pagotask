// pagotask is a tray app that captures Google Tasks from a global hotkey.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/pashagolub/pagotask/internal/config"
	"github.com/pashagolub/pagotask/internal/dates"
	"github.com/pashagolub/pagotask/internal/editor"
	"github.com/pashagolub/pagotask/internal/gtasks"
	"github.com/pashagolub/pagotask/internal/platform"
	"github.com/pashagolub/pagotask/internal/queue"
	"github.com/pashagolub/pagotask/internal/rules"
)

type app struct {
	mu     sync.RWMutex
	cfg    *config.Config
	reader platform.PageReader
	hotkey platform.Hotkey
	tray   platform.Tray
	editor editor.Editor
	auth   *gtasks.Auth
	client *gtasks.Client
	queue  *queue.Queue
	ctx    context.Context
	cancel context.CancelFunc
}

func main() {
	dir, err := config.Dir()
	if err != nil {
		slog.Error("config dir", "err", err)
		os.Exit(1)
	}
	logFile, err := os.OpenFile(filepath.Join(dir, "pagotask.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(logFile, nil)))
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	reader, hk, tray, store := newPlatform()
	auth, err := gtasks.NewAuth(cfg.Google.ClientID, cfg.Google.ClientSecret, store)
	if err != nil {
		slog.Error("auth", "err", err)
		os.Exit(1)
	}
	q, err := queue.Open(filepath.Join(dir, "queue"))
	if err != nil {
		slog.Error("queue", "err", err)
		os.Exit(1)
	}
	a := &app{cfg: cfg, reader: reader, hotkey: hk, tray: tray, editor: editor.New(), auth: auth, queue: q}
	a.client = gtasks.NewClient(auth, func() map[string]string { return a.config().Lists })
	q.OnIdle = tray.SetPending
	a.ctx, a.cancel = context.WithCancel(context.Background())

	tray.OnAdd(a.capture)
	tray.OnSignIn(a.signIn)
	tray.OnSignOut(func() {
		_ = auth.SignOut()
		tray.SetSignedIn(false)
	})
	tray.OnOpenConfig(func() {
		p, _ := config.Path()
		_ = openFile(p)
	})
	tray.OnQuit(a.editor.Quit)

	err = a.editor.Run(editor.Callbacks{
		Catalog: a.catalog,
		OnSave:  a.save,
		OnStart: func() {
			tray.Start(func() { tray.SetSignedIn(auth.SignedIn()) })
			if err := hk.Register(cfg.Hotkey, a.capture); err != nil {
				slog.Error("hotkey", "err", err)
			}
			go q.Run(a.ctx, a.client)
			go a.watchConfig()
			slog.Info("pagotask started", "hotkey", cfg.Hotkey)
		},
		OnStop: func() {
			a.cancel()
			_ = hk.Unregister()
			tray.Stop()
		},
	})
	if err != nil {
		slog.Error("editor", "err", err)
		os.Exit(1)
	}
}

func (a *app) config() *config.Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

func (a *app) signIn() {
	if err := a.auth.SignIn(a.ctx); err != nil {
		slog.Error("sign in", "err", err)
		return
	}
	a.tray.SetSignedIn(true)
	if err := a.client.RefreshLists(a.ctx); err != nil {
		slog.Warn("refresh lists", "err", err)
	}
}

// capture runs on the hotkey: read the front window, prefill and open the editor.
func (a *app) capture() {
	cfg := a.config()
	cap, err := a.reader.Foreground(wantsURL(cfg))
	if err != nil {
		slog.Warn("capture", "err", err)
	}
	d := rules.Prefill(cfg, cap)
	slog.Info("capture", "process", cap.Process, "title", cap.Title, "url", cap.URL, "draft", d)
	a.editor.Open(editor.Draft{Tag: d.Tag, Title: d.Title, List: d.List, Due: "tod", Notes: d.Notes})
}

// save validates the editor draft and writes it to the queue.
func (a *app) save(d editor.Draft) error {
	cfg := a.config()
	if _, ok := cfg.Lists[d.List]; !ok {
		return fmt.Errorf("unknown list %q", d.List)
	}
	due, err := dates.Parse(d.Due, time.Now())
	if err != nil {
		return err
	}
	title := rules.FullTitle(cfg, rules.Draft{Tag: d.Tag, Title: d.Title})
	if title == "" {
		return fmt.Errorf("title is empty")
	}
	return a.queue.Put(queue.Item{ListKey: d.List, Title: title, Notes: d.Notes, Due: due})
}

func (a *app) catalog() editor.Catalog {
	cfg := a.config()
	c := editor.Catalog{DefaultList: cfg.DefaultList}
	for id, t := range cfg.Tags {
		c.Tags = append(c.Tags, editor.TagInfo{ID: id, Emoji: t.Emoji, Key: t.Key, List: t.List})
	}
	sort.Slice(c.Tags, func(i, j int) bool { return c.Tags[i].ID < c.Tags[j].ID })
	for k, title := range cfg.Lists {
		c.Lists = append(c.Lists, editor.ListInfo{Key: k, Title: title})
	}
	sort.Slice(c.Lists, func(i, j int) bool { return c.Lists[i].Title < c.Lists[j].Title })
	return c
}

func wantsURL(cfg *config.Config) bool {
	for _, s := range cfg.Sources {
		if s.Read == "url" {
			return true
		}
	}
	return false
}

// watchConfig reloads config.yaml when it changes (polling; cheap and portable).
func (a *app) watchConfig() {
	path, err := config.Path()
	if err != nil {
		return
	}
	var last time.Time
	if st, err := os.Stat(path); err == nil {
		last = st.ModTime()
	}
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
			st, err := os.Stat(path)
			if err != nil || !st.ModTime().After(last) {
				continue
			}
			last = st.ModTime()
			cfg, err := config.Load()
			if err != nil {
				slog.Warn("config reload", "err", err)
				continue
			}
			if cfg.Hotkey != a.config().Hotkey {
				_ = a.hotkey.Unregister()
				if err := a.hotkey.Register(cfg.Hotkey, a.capture); err != nil {
					slog.Error("hotkey", "err", err)
				}
			}
			a.mu.Lock()
			a.cfg = cfg
			a.mu.Unlock()
			slog.Info("config reloaded")
		}
	}
}
