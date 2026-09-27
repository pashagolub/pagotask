// Package opentasks keeps a local copy of the open Google Tasks so the
// open-tasks popup shows instantly, and turns it into the rows the popup
// renders: sorted by due date, with checks the user made that Google has
// not seen yet already applied.
package opentasks

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/pashagolub/pagotask/internal/queue"
)

// Task is one open task as fetched from Google.
type Task struct {
	ID       string    `json:"id"`
	ListKey  string    `json:"list"`
	Title    string    `json:"title"`
	Notes    string    `json:"notes,omitempty"`
	Due      string    `json:"due,omitempty"` // YYYY-MM-DD; Google keeps no time
	ParentID string    `json:"parent,omitempty"`
	Link     string    `json:"link,omitempty"`
	Position string    `json:"position,omitempty"`
	Updated  time.Time `json:"updated"`
}

// When a row is due, relative to today.
const (
	Overdue = "overdue"
	Today   = "today"
	Later   = "later"
	Undated = "none"
)

// Row is what the popup shows for one task.
type Row struct {
	ID        string `json:"id"`
	List      string `json:"list"`
	ListTitle string `json:"listTitle"`
	Title     string `json:"title"`
	Parent    string `json:"parent,omitempty"` // parent task title, for subtasks
	Due       string `json:"due,omitempty"`
	When      string `json:"when"`
	Link      string `json:"link,omitempty"`
	Done      bool   `json:"done,omitempty"`    // checked here, Google not refreshed since
	Syncing   bool   `json:"syncing,omitempty"` // added in pagotask, not in Google yet
}

type override struct {
	Done bool      `json:"done"`
	At   time.Time `json:"at"`
}

// Store is the local copy, persisted as JSON so it survives restarts.
type Store struct {
	path string

	mu      sync.Mutex
	tasks   []Task
	fetched time.Time
	over    map[string]override // task id -> check state set here
}

type snapshot struct {
	Fetched time.Time           `json:"fetched"`
	Tasks   []Task              `json:"tasks"`
	Over    map[string]override `json:"overrides,omitempty"`
}

// Open loads the copy at path; a missing or unreadable file gives an empty store.
func Open(path string) *Store {
	s := &Store{path: path, over: map[string]override{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	var snap snapshot
	if json.Unmarshal(data, &snap) == nil {
		s.tasks, s.fetched = snap.Tasks, snap.Fetched
		if snap.Over != nil {
			s.over = snap.Over
		}
	}
	return s
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	data, err := json.Marshal(snapshot{Fetched: s.fetched, Tasks: s.tasks, Over: s.over})
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Replace installs a fresh fetch that started at started. A check made here
// is kept on top of it while Google may not have it yet: when it was made
// after the fetch started, or its queue item was still waiting then.
func (s *Store) Replace(tasks []Task, started time.Time, pendingAtStart map[string]bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks, s.fetched = tasks, started
	for id, o := range s.over {
		if o.At.Before(started) && !pendingAtStart[id] {
			delete(s.over, id)
		}
	}
	return s.saveLocked()
}

// Mark records a check or uncheck made in the popup.
func (s *Store) Mark(taskID string, done bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.over[taskID] = override{Done: done, At: time.Now()}
	return s.saveLocked()
}

// Task finds a task by id.
func (s *Store) Task(id string) (Task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tasks {
		if t.ID == id {
			return t, true
		}
	}
	return Task{}, false
}

// Fetched is when the copy was last refreshed; zero if never.
func (s *Store) Fetched() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fetched
}

// Rows builds the popup rows for the configured lists (key -> title),
// adding tasks still waiting in the queue. today is YYYY-MM-DD, local.
func (s *Store) Rows(lists map[string]string, queued []queue.Item, today string) []Row {
	s.mu.Lock()
	defer s.mu.Unlock()
	return buildRows(s.tasks, s.over, lists, queued, today)
}

func buildRows(tasks []Task, over map[string]override, lists map[string]string, queued []queue.Item, today string) []Row {
	titles := make(map[string]string, len(tasks))
	for _, t := range tasks {
		titles[t.ID] = t.Title
	}
	var rows []Row
	pos := map[string]string{}
	updated := map[string]time.Time{}
	for _, t := range tasks {
		lt, ok := lists[t.ListKey]
		if !ok || t.Title == "" {
			continue
		}
		r := Row{ID: t.ID, List: t.ListKey, ListTitle: lt, Title: t.Title, Due: t.Due, When: when(t.Due, today), Link: t.Link}
		if t.ParentID != "" {
			r.Parent = titles[t.ParentID]
		}
		if o, ok := over[t.ID]; ok {
			r.Done = o.Done
		}
		pos[t.ID], updated[t.ID] = t.Position, t.Updated
		rows = append(rows, r)
	}
	for _, it := range queued {
		lt, ok := lists[it.ListKey]
		if it.Op != queue.OpCreate || !ok || it.Attempts >= queue.MaxAttempts {
			continue
		}
		due := ""
		if !it.Due.IsZero() {
			due = it.Due.Format("2006-01-02")
		}
		id := "queued-" + it.ID
		rows = append(rows, Row{ID: id, List: it.ListKey, ListTitle: lt, Title: it.Title, Due: due, When: when(due, today), Link: FirstLink(it.Notes, nil), Syncing: true})
		updated[id] = it.Created
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if (a.Due == "") != (b.Due == "") {
			return a.Due != ""
		}
		if a.Due != b.Due {
			return a.Due < b.Due
		}
		if a.ListTitle != b.ListTitle {
			return a.ListTitle < b.ListTitle
		}
		if a.Due == "" {
			return updated[a.ID].After(updated[b.ID]) // undated: newest first
		}
		return pos[a.ID] < pos[b.ID]
	})
	return rows
}

func when(due, today string) string {
	switch {
	case due == "":
		return Undated
	case due < today:
		return Overdue
	case due == today:
		return Today
	default:
		return Later
	}
}

var urlRe = regexp.MustCompile(`https?://[^\s<>"'\x60]+`)

// FirstLink is the first URL in the notes, else the first of links.
func FirstLink(notes string, links []string) string {
	if u := urlRe.FindString(notes); u != "" {
		return u
	}
	for _, l := range links {
		if l != "" {
			return l
		}
	}
	return ""
}
