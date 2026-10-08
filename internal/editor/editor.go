// Package editor is the capture popup (one title line, list and due chips,
// optional notes) and the open-tasks popup (check tasks off). The Windows build renders it with Wails v3 (WebView2),
// which also owns the tray icon; other platforms get a headless stand-in
// until they have a window backend.
package editor

import (
	"github.com/pashagolub/pagotask/internal/opentasks"
	"github.com/pashagolub/pagotask/internal/platform"
)

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

// RecentInfo is one recently used task: Full is the Google title (for
// filtering), Tag and Title what the popup fills in.
type RecentInfo struct {
	Full  string `json:"full"`
	Tag   string `json:"tag"`
	Title string `json:"title"`
	List  string `json:"list"`
}

// Catalog is sent to the popup on every open so config edits apply live.
type Catalog struct {
	Tags        []TagInfo    `json:"tags"`
	Lists       []ListInfo   `json:"lists"`
	DefaultList string       `json:"defaultList"`
	Recent      []RecentInfo `json:"recent"` // newest first
}

// TaskView is what the open-tasks popup renders: rows plus a status line
// (not signed in, refresh failed), empty when all is well.
type TaskView struct {
	Rows []opentasks.Row `json:"rows"`
	Note string          `json:"note"`
}

// Callbacks wire the popups to the app.
type Callbacks struct {
	Catalog func() Catalog
	OnSave  func(Draft) error // return an error to keep the popup open with a message

	Tasks       func() TaskView
	OnToggle    func(taskID string, done bool) error
	OnTasksOpen func() // the open-tasks popup was shown: refresh from Google
	OnStart     func() // runs once the window backend is up (register hotkey, start tray)
	OnStop      func()
}

// Editor is the popup window.
type Editor interface {
	// Run owns the main thread until the app quits.
	Run(cb Callbacks) error
	// Open shows the popup prefilled with d. Safe to call from any goroutine.
	Open(d Draft)
	// OpenTasks shows the open-tasks popup. Safe to call from any goroutine.
	OpenTasks()
	// TasksChanged tells the open-tasks popup to re-read Callbacks.Tasks.
	TasksChanged()
	// Quit ends Run.
	Quit()
	// Tray is the status icon, which lives in the same UI backend.
	Tray() platform.Tray
}
