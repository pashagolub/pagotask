//go:build windows

package editor

import (
	"context"
	"embed"
	"log/slog"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed frontend
var assets embed.FS

const (
	winWidth  = 560
	winHeight = 190
)

// wailsEditor renders frontend/ in a small frameless always-on-top window.
type wailsEditor struct {
	cb  Callbacks
	mu  sync.Mutex
	ctx context.Context
	// pending holds a draft opened before the window was ready.
	pending *Draft
}

// New returns the Wails-backed editor.
func New() Editor { return &wailsEditor{} }

// App is the struct bound into the page as window.go.editor.App.
type App struct{ e *wailsEditor }

// Catalog returns tags and lists for the page.
func (a *App) Catalog() Catalog { return a.e.cb.Catalog() }

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

func (e *wailsEditor) Run(cb Callbacks) error {
	e.cb = cb
	return wails.Run(&options.App{
		Title:             "pagotask",
		Width:             winWidth,
		Height:            winHeight,
		MinWidth:          winWidth,
		MinHeight:         winHeight,
		Frameless:         true,
		AlwaysOnTop:       true,
		StartHidden:       true,
		HideWindowOnClose: true,
		DisableResize:     true,
		AssetServer:       &assetserver.Options{Assets: assets},
		Bind:              []interface{}{&App{e: e}},
		OnStartup: func(ctx context.Context) {
			e.mu.Lock()
			e.ctx = ctx
			p := e.pending
			e.pending = nil
			e.mu.Unlock()
			if cb.OnStart != nil {
				cb.OnStart()
			}
			if p != nil {
				e.Open(*p)
			}
		},
		OnShutdown: func(context.Context) {
			if cb.OnStop != nil {
				cb.OnStop()
			}
		},
		Windows: &windows.Options{
			DisableWindowIcon:                 true,
			DisableFramelessWindowDecorations: true,
			Theme:                             windows.SystemDefault,
		},
	})
}

func (e *wailsEditor) Open(d Draft) {
	e.mu.Lock()
	ctx := e.ctx
	if ctx == nil {
		e.pending = &d
		e.mu.Unlock()
		return
	}
	e.mu.Unlock()
	runtime.EventsEmit(ctx, "draft", d)
	runtime.WindowCenter(ctx)
	runtime.WindowShow(ctx)
	slog.Debug("editor opened", "draft", d)
}

func (e *wailsEditor) hide() {
	e.mu.Lock()
	ctx := e.ctx
	e.mu.Unlock()
	if ctx != nil {
		runtime.WindowHide(ctx)
	}
}

func (e *wailsEditor) Quit() {
	e.mu.Lock()
	ctx := e.ctx
	e.mu.Unlock()
	if ctx != nil {
		runtime.Quit(ctx)
	}
}
