//go:build windows || linux

package editor

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/pashagolub/pagotask/internal/platform"
)

//go:embed frontend
var assets embed.FS

const (
	winWidth  = 560
	winHeight = 150 // first guess; the page resizes the window to fit

	tasksWidth  = 640
	tasksHeight = 460
)

// wailsEditor is one Wails v3 application that owns both the popup window
// and the tray icon, so both live on the app's UI thread.
type wailsEditor struct {
	cb    Callbacks
	app   *application.App
	win   *application.WebviewWindow
	tasks *application.WebviewWindow
	tray  *tray

	mu       sync.Mutex
	started  bool
	ready    map[*application.WebviewWindow]bool // page loaded; safe to show
	pending  map[*application.WebviewWindow]bool // shown before it was ready
	quitting atomic.Bool                         // closing windows really closes them
	current  *Draft                              // the draft on screen, for a page that loads after Open
}

// New returns the Wails-backed editor.
func New() Editor {
	return &wailsEditor{tray: &tray{},
		ready:   map[*application.WebviewWindow]bool{},
		pending: map[*application.WebviewWindow]bool{}}
}

func (e *wailsEditor) Tray() platform.Tray { return e.tray }

// App is the service bound into the page; the frontend calls it by name,
// e.g. "github.com/pashagolub/pagotask/internal/editor.App.Save".
type App struct{ e *wailsEditor }

// Catalog returns tags and lists for the page.
func (a *App) Catalog() Catalog { return a.e.cb.Catalog() }

// Current returns the draft on screen, or nil when the popup is hidden.
func (a *App) Current() *Draft {
	a.e.mu.Lock()
	defer a.e.mu.Unlock()
	return a.e.current
}

// Save is called on Enter. A non-empty return keeps the popup open and shows the text.
func (a *App) Save(d Draft) string {
	if err := a.e.cb.OnSave(d); err != nil {
		return err.Error()
	}
	a.e.hide()
	return ""
}

// Cancel is called on Esc.
func (a *App) Cancel() { a.e.hide() }

// Tasks returns the rows for the open-tasks popup.
func (a *App) Tasks() TaskView { return a.e.cb.Tasks() }

// Toggle checks (done) or unchecks a task. A non-empty return is shown as an error.
func (a *App) Toggle(id string, done bool) string {
	if err := a.e.cb.OnToggle(id, done); err != nil {
		return err.Error()
	}
	return ""
}

// OpenLink opens url in the default browser and hides the tasks popup.
func (a *App) OpenLink(url string) string {
	if err := a.e.app.Browser.OpenURL(url); err != nil {
		return err.Error()
	}
	a.e.hideTasks()
	return ""
}

// CloseTasks is called on Esc in the tasks popup.
func (a *App) CloseTasks() { a.e.hideTasks() }

func (e *wailsEditor) Run(cb Callbacks) error {
	e.cb = cb
	sub, err := fs.Sub(assets, "frontend")
	if err != nil {
		return err
	}
	e.app = application.New(application.Options{
		Name:        "pagotask",
		Description: "Quick capture for Google Tasks",
		Services:    []application.Service{application.NewService(&App{e: e})},
		Assets:      application.AssetOptions{Handler: application.BundledAssetFileServer(sub)},
		Windows:     application.WindowsOptions{DisableQuitOnLastWindowClosed: true},
		Linux:       application.LinuxOptions{DisableQuitOnLastWindowClosed: true, ProgramName: "pagotask"},
		OnShutdown: func() {
			if cb.OnStop != nil {
				cb.OnStop()
			}
		},
	})

	e.win = e.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:          "popup",
		Title:         "pagotask",
		Width:         winWidth,
		Height:        winHeight,
		Frameless:     true,
		AlwaysOnTop:   true,
		DisableResize: true,
		Hidden:        true,
		Windows:       application.WindowsWindow{HiddenOnTaskbar: true},
	})
	// Closing (Alt+F4) only hides the popup; the app lives in the tray.
	e.win.RegisterHook(events.Common.WindowClosing, func(ev *application.WindowEvent) {
		if e.quitting.Load() {
			return
		}
		ev.Cancel()
		e.hide()
	})

	e.tasks = e.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:          "tasks",
		Title:         "pagotask: open tasks",
		URL:           "/tasks.html",
		Width:         tasksWidth,
		Height:        tasksHeight,
		MinWidth:      tasksWidth,
		MinHeight:     tasksHeight,
		Frameless:     true,
		AlwaysOnTop:   true,
		DisableResize: true,
		Hidden:        true,
		Windows:       application.WindowsWindow{HiddenOnTaskbar: true},
	})
	e.tasks.RegisterHook(events.Common.WindowClosing, func(ev *application.WindowEvent) {
		if e.quitting.Load() {
			return
		}
		ev.Cancel()
		e.hideTasks()
	})

	for _, w := range []*application.WebviewWindow{e.win, e.tasks} {
		w.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
			e.mu.Lock()
			e.ready[w] = true
			due := e.pending[w]
			delete(e.pending, w)
			e.mu.Unlock()
			if due {
				present(w)
			}
		})
	}

	e.tray.build(e.app)

	e.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		e.mu.Lock()
		e.started = true
		e.mu.Unlock()
		if cb.OnStart != nil {
			cb.OnStart()
		}
	})
	return e.app.Run()
}

func (e *wailsEditor) Open(d Draft) {
	e.mu.Lock()
	e.current = &d
	started := e.started
	e.mu.Unlock()
	if !started {
		return // the page asks for Current once it loads
	}
	e.app.Event.Emit("draft", d)
	e.show(e.win)
	slog.Debug("editor opened", "draft", d)
}

func (e *wailsEditor) OpenTasks() {
	e.mu.Lock()
	started := e.started
	e.mu.Unlock()
	if !started {
		return
	}
	if e.cb.OnTasksOpen != nil {
		go e.cb.OnTasksOpen()
	}
	e.app.Event.Emit("tasks-open")
	e.show(e.tasks)
}

// show brings w up, or, when its page has not loaded yet (a "pagotask add"
// that started the app), as soon as it has: showing a window before then
// creates it hidden.
func (e *wailsEditor) show(w *application.WebviewWindow) {
	e.mu.Lock()
	if !e.ready[w] {
		e.pending[w] = true
		e.mu.Unlock()
		return
	}
	e.mu.Unlock()
	present(w)
}

func present(w *application.WebviewWindow) {
	w.Center()
	w.Show()
	if runtime.GOOS == "linux" {
		w.Center() // GTK knows the window's size only once it is shown
	}
	w.Focus()
}

func (e *wailsEditor) TasksChanged() {
	e.mu.Lock()
	started := e.started
	e.mu.Unlock()
	if started {
		e.app.Event.Emit("tasks-changed")
	}
}

func (e *wailsEditor) hide() {
	e.mu.Lock()
	e.current = nil
	e.mu.Unlock()
	if e.win != nil {
		e.win.Hide()
		application.InvokeAsync(showPointer)
	}
}

func (e *wailsEditor) Quit() {
	e.quitting.Store(true)
	if e.app != nil {
		e.app.Quit()
	}
}

// tray is the notification-area icon: left click opens the popup, right
// click shows the menu.
type tray struct {
	onAdd, onTasks, onSignIn, onSignOut, onOpenConfig, onQuit func()

	t                 *application.SystemTray
	menu              *application.Menu
	mSignIn, mSignOut *application.MenuItem
}

func (t *tray) build(app *application.App) {
	t.menu = app.Menu.New()
	t.menu.Add("Add task").OnClick(func(*application.Context) { call(t.onAdd) })
	t.menu.Add("Open tasks").OnClick(func(*application.Context) { call(t.onTasks) })
	t.menu.AddSeparator()
	t.mSignIn = t.menu.Add("Sign in to Google").OnClick(func(*application.Context) { call(t.onSignIn) })
	t.mSignOut = t.menu.Add("Sign out").OnClick(func(*application.Context) { call(t.onSignOut) })
	t.menu.Add("Open config.yaml").OnClick(func(*application.Context) { call(t.onOpenConfig) })
	t.addRunAtLogin(app)
	t.menu.AddSeparator()
	t.menu.Add("Quit").OnClick(func(*application.Context) { call(t.onQuit) })

	t.t = app.SystemTray.New()
	t.t.SetIcon(trayIcon)
	t.t.SetTooltip("pagotask")
	t.t.SetMenu(t.menu)
	t.t.OnClick(func() { call(t.onAdd) })
	if runtime.GOOS == "windows" {
		// On Linux the desktop's tray host opens the menu itself.
		t.t.OnRightClick(func() {
			showPointer()
			t.t.OpenMenu()
		})
	}
}

// addRunAtLogin adds a checkbox that registers pagotask to start at login
// (Windows: the Run registry key; Linux: ~/.config/autostart).
func (t *tray) addRunAtLogin(app *application.App) {
	on, err := app.Autostart.IsEnabled()
	if err != nil {
		slog.Warn("run at login", "err", err)
	}
	item := t.menu.AddCheckbox("Run at login", on)
	item.OnClick(func(*application.Context) {
		want := item.Checked() // already flipped by the click
		err := app.Autostart.Disable()
		if want {
			err = app.Autostart.Enable()
		}
		if err != nil {
			slog.Error("run at login", "on", want, "err", err)
			item.SetChecked(!want)
		}
	})
}

func call(f func()) {
	if f != nil {
		go f()
	}
}

func (t *tray) OnAdd(f func())        { t.onAdd = f }
func (t *tray) OnTasks(f func())      { t.onTasks = f }
func (t *tray) OnSignIn(f func())     { t.onSignIn = f }
func (t *tray) OnSignOut(f func())    { t.onSignOut = f }
func (t *tray) OnOpenConfig(f func()) { t.onOpenConfig = f }
func (t *tray) OnQuit(f func())       { t.onQuit = f }

// Start runs onReady; the icon itself appears with the application.
func (t *tray) Start(onReady func()) {
	if onReady != nil {
		onReady()
	}
}

// Stop is a no-op: the icon goes away when the application quits.
func (t *tray) Stop() {}

// SetPending reflects queue state in the tooltip.
func (t *tray) SetPending(pending, stuck int) {
	if t.t == nil {
		return
	}
	switch {
	case stuck > 0:
		t.t.SetTooltip(fmt.Sprintf("pagotask: %d task(s) could not be saved, %d pending", stuck, pending))
	case pending > 0:
		t.t.SetTooltip(fmt.Sprintf("pagotask: %d task(s) pending", pending))
	default:
		t.t.SetTooltip("pagotask")
	}
}

// SetSignedIn shows either "Sign in to Google" or "Sign out".
func (t *tray) SetSignedIn(in bool) {
	if t.mSignIn == nil {
		return
	}
	application.InvokeSync(func() {
		t.mSignIn.SetHidden(in)
		t.mSignOut.SetHidden(!in)
	})
}

func (e *wailsEditor) hideTasks() {
	e.tasks.Hide()
	application.InvokeAsync(showPointer)
}
