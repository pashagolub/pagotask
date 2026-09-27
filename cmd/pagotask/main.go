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
	"github.com/pashagolub/pagotask/internal/opentasks"
	"github.com/pashagolub/pagotask/internal/platform"
	"github.com/pashagolub/pagotask/internal/queue"
	"github.com/pashagolub/pagotask/internal/rules"
)

type app struct {
	mu     sync.RWMutex
	cfg    *config.Config
	reader platform.PageReader
	hotkey platform.Hotkey
	tasksK platform.Hotkey // opens the open-tasks popup
	tray   platform.Tray
	editor editor.Editor
	auth   *gtasks.Auth
	client *gtasks.Client
	queue  *queue.Queue
	store  *opentasks.Store
	ctx    context.Context
	cancel context.CancelFunc

	refreshNow chan struct{}
	noteMu     sync.Mutex
	note       string // why the open-tasks list may be stale, "" when fine
}

// refreshEvery is how often the open-tasks copy is refreshed in the background.
const refreshEvery = 5 * time.Minute

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

	reader, hk, store := newPlatform()
	ed := editor.New()
	tray := ed.Tray()
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
	a := &app{cfg: cfg, reader: reader, hotkey: hk, tasksK: newHotkey(), tray: tray, editor: ed, auth: auth, queue: q,
		store: opentasks.Open(filepath.Join(dir, "tasks.json")), refreshNow: make(chan struct{}, 1)}
	a.client = gtasks.NewClient(auth, func() map[string]string { return a.config().Lists })
	q.OnIdle = func(pending, stuck int) {
		tray.SetPending(pending, stuck)
		a.refreshSoon() // pick up what was just delivered, with its Google id
	}
	a.ctx, a.cancel = context.WithCancel(context.Background())

	tray.OnAdd(a.capture)
	tray.OnTasks(a.editor.OpenTasks)
	tray.OnSignIn(a.signIn)
	tray.OnSignOut(func() {
		_ = auth.SignOut()
		tray.SetSignedIn(false)
		a.editor.TasksChanged()
	})
	tray.OnOpenConfig(func() {
		p, _ := config.Path()
		_ = openFile(p)
	})
	tray.OnQuit(func() {
		// Quit must never leave a live icon that ignores it: if shutdown
		// hangs, exit anyway. Queued work is on disk and survives.
		time.AfterFunc(3*time.Second, func() {
			slog.Warn("quit: shutdown did not finish, exiting")
			os.Exit(0)
		})
		a.editor.Quit()
	})

	err = a.editor.Run(editor.Callbacks{
		Catalog:     a.catalog,
		OnSave:      a.save,
		Tasks:       a.tasks,
		OnToggle:    a.toggle,
		OnTasksOpen: a.refreshSoon,
		OnStart: func() {
			tray.Start(func() { tray.SetSignedIn(auth.SignedIn()) })
			if err := hk.Register(cfg.Hotkey, a.capture); err != nil {
				slog.Error("hotkey", "err", err)
			}
			if err := a.tasksK.Register(cfg.TasksHotkey, a.editor.OpenTasks); err != nil {
				slog.Error("tasks hotkey", "err", err)
			}
			go q.Run(a.ctx, a.client)
			go a.watchConfig()
			go a.refreshLoop()
			slog.Info("pagotask started", "hotkey", cfg.Hotkey, "tasks_hotkey", cfg.TasksHotkey)
		},
		OnStop: func() {
			a.cancel()
			_ = hk.Unregister()
			_ = a.tasksK.Unregister()
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
	a.refreshSoon()
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
	if err := a.queue.Put(queue.Item{ListKey: d.List, Title: title, Notes: d.Notes, Due: due}); err != nil {
		return err
	}
	a.editor.TasksChanged() // it shows in the open-tasks popup at once
	return nil
}

// tasks is the open-tasks popup's view: the local copy plus what is still queued.
func (a *app) tasks() editor.TaskView {
	queued, err := a.queue.List()
	if err != nil {
		slog.Warn("queue list", "err", err)
	}
	v := editor.TaskView{Rows: a.store.Rows(a.config().Lists, queued, time.Now().Format("2006-01-02"))}
	a.noteMu.Lock()
	v.Note = a.note
	a.noteMu.Unlock()
	if !a.auth.SignedIn() {
		v.Note = "Not signed in: use Sign in to Google in the tray menu"
	}
	return v
}

// toggle queues a check or uncheck and shows it locally at once.
func (a *app) toggle(id string, done bool) error {
	t, ok := a.store.Task(id)
	if !ok {
		return fmt.Errorf("this task is not in Google yet; try again in a moment")
	}
	op := queue.OpReopen
	if done {
		op = queue.OpComplete
	}
	if err := a.queue.Put(queue.Item{ID: queue.ToggleID(id), Op: op, TaskID: id, ListKey: t.ListKey, Title: t.Title}); err != nil {
		return err
	}
	return a.store.Mark(id, done)
}

func (a *app) refreshSoon() {
	select {
	case a.refreshNow <- struct{}{}:
	default:
	}
}

// refreshLoop keeps the open-tasks copy fresh: on start, on every popup
// open and every refreshEvery.
func (a *app) refreshLoop() {
	t := time.NewTicker(refreshEvery)
	defer t.Stop()
	a.refresh()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
		case <-a.refreshNow:
		}
		a.refresh()
	}
}

func (a *app) refresh() {
	if !a.auth.SignedIn() {
		return
	}
	started := time.Now()
	pending := map[string]bool{}
	if items, err := a.queue.List(); err == nil {
		for _, it := range items {
			if it.TaskID != "" {
				pending[it.TaskID] = true
			}
		}
	}
	fetched, err := a.client.OpenTasks(a.ctx)
	note := ""
	if err != nil {
		slog.Warn("refresh open tasks", "err", err)
		note = "Could not refresh from Google; showing the last copy"
	} else if err := a.store.Replace(fetched, started, pending); err != nil {
		slog.Warn("save open tasks", "err", err)
	}
	a.noteMu.Lock()
	a.note = note
	a.noteMu.Unlock()
	a.editor.TasksChanged()
}

func (a *app) catalog() editor.Catalog {
	cfg := a.config()
	c := editor.Catalog{DefaultList: cfg.DefaultList}
	for id, t := range cfg.Tags {
		c.Tags = append(c.Tags, editor.TagInfo{ID: id, Emoji: t.Emoji, Key: t.Key, Aliases: t.Aliases, List: t.List})
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
			if cfg.TasksHotkey != a.config().TasksHotkey {
				_ = a.tasksK.Unregister()
				if err := a.tasksK.Register(cfg.TasksHotkey, a.editor.OpenTasks); err != nil {
					slog.Error("tasks hotkey", "err", err)
				}
			}
			a.mu.Lock()
			a.cfg = cfg
			a.mu.Unlock()
			slog.Info("config reloaded")
		}
	}
}
