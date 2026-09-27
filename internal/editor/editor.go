// Package editor is the capture popup: one title line, list and due chips,
// optional notes. The Windows build renders it with Wails v3 (WebView2),
// which also owns the tray icon; other platforms get a headless stand-in
// until they have a window backend.
package editor

import "github.com/pashagolub/pagotask/internal/platform"

// Draft is what the popup shows and returns. Tag is a tag id, List a list
// key, Due a keyword such as "tod", "tom", "fri", "+3" or a date.
type Draft struct {
	Tag   string `json:"tag"`
	Title string `json:"title"`
	List  string `json:"list"`
	Due   string `json:"due"`
	Notes string `json:"notes"`
}

// TagInfo and ListInfo are the catalog the popup renders from.
type TagInfo struct {
	ID      string   `json:"id"`
	Emoji   string   `json:"emoji"`
	Key     string   `json:"key"`
	Aliases []string `json:"aliases"`
	List    string   `json:"list"`
}

type ListInfo struct {
	Key   string `json:"key"`
	Title string `json:"title"`
}

// Catalog is sent to the popup on every open so config edits apply live.
type Catalog struct {
	Tags        []TagInfo  `json:"tags"`
	Lists       []ListInfo `json:"lists"`
	DefaultList string     `json:"defaultList"`
}

// Callbacks wire the popup to the app.
type Callbacks struct {
	Catalog func() Catalog
	OnSave  func(Draft) error // return an error to keep the popup open with a message
	OnStart func()            // runs once the window backend is up (register hotkey, start tray)
	OnStop  func()
}

// Editor is the popup window.
type Editor interface {
	// Run owns the main thread until the app quits.
	Run(cb Callbacks) error
	// Open shows the popup prefilled with d. Safe to call from any goroutine.
	Open(d Draft)
	// Quit ends Run.
	Quit()
	// Tray is the status icon, which lives in the same UI backend.
	Tray() platform.Tray
}
