// pagotask is a tray app that captures Google Tasks from a global hotkey.
package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/pashagolub/pagotask/internal/config"
	"github.com/pashagolub/pagotask/internal/dates"
	"github.com/pashagolub/pagotask/internal/gtasks"
	"github.com/pashagolub/pagotask/internal/platform"
	"github.com/pashagolub/pagotask/internal/queue"
	"github.com/pashagolub/pagotask/internal/rules"
)

type app struct {
	cfg    *config.Config
	reader platform.PageReader
	hotkey platform.Hotkey
	tray   platform.Tray
	auth   *gtasks.Auth
	client *gtasks.Client
	queue  *queue.Queue
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
	a := &app{cfg: cfg, reader: reader, hotkey: hk, tray: tray, auth: auth, queue: q}
	a.client = gtasks.NewClient(auth, func() map[string]string { return a.cfg.Lists })
	q.OnIdle = tray.SetPending

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel

	tray.OnAdd(a.capture)
	tray.OnSignIn(func() {
		if err := auth.SignIn(ctx); err != nil {
			slog.Error("sign in", "err", err)
			return
		}
		tray.SetSignedIn(true)
		if err := a.client.RefreshLists(ctx); err != nil {
			slog.Warn("refresh lists", "err", err)
		}
	})
	tray.OnSignOut(func() {
		_ = auth.SignOut()
		tray.SetSignedIn(false)
	})
	tray.OnOpenConfig(func() {
		p, _ := config.Path()
		_ = openFile(p)
	})
	tray.OnQuit(cancel)

	tray.Run(func() {
		tray.SetSignedIn(auth.SignedIn())
		if err := hk.Register(cfg.Hotkey, a.capture); err != nil {
			slog.Error("hotkey", "err", err)
		}
		go q.Run(ctx, a.client)
		go a.watchConfig(ctx)
		slog.Info("pagotask started", "hotkey", cfg.Hotkey)
	}, func() {
		cancel()
		_ = hk.Unregister()
	})
}

// capture runs on the hotkey: read the front window, prefill a draft and
// hand it to the editor. Until the popup lands, the draft is saved straight
// to the queue so the pipeline can be exercised end to end.
func (a *app) capture() {
	cap, err := a.reader.Foreground(a.wantsURL())
	if err != nil {
		slog.Warn("capture", "err", err)
	}
	d := rules.Prefill(a.cfg, cap)
	slog.Info("capture", "process", cap.Process, "title", cap.Title, "url", cap.URL, "draft", d)
	if d.Tag == "" && d.Title == "" {
		return // TODO(popup): open the empty editor instead
	}
	due, _ := dates.Parse("", time.Now())
	if err := a.queue.Put(queue.Item{
		ListKey: d.List,
		Title:   rules.FullTitle(a.cfg, d),
		Notes:   d.Notes,
		Due:     due,
	}); err != nil {
		slog.Error("queue put", "err", err)
	}
}

func (a *app) wantsURL() bool {
	for _, s := range a.cfg.Sources {
		if s.Read == "url" {
			return true
		}
	}
	return false
}

// watchConfig reloads config.yaml when it changes (polling; cheap and portable).
func (a *app) watchConfig(ctx context.Context) {
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
		case <-ctx.Done():
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
			if cfg.Hotkey != a.cfg.Hotkey {
				_ = a.hotkey.Unregister()
				if err := a.hotkey.Register(cfg.Hotkey, a.capture); err != nil {
					slog.Error("hotkey", "err", err)
				}
			}
			a.cfg = cfg
			slog.Info("config reloaded")
		}
	}
}
