// Package queue is the on-disk outbox: the popup writes here and returns
// at once; a pusher drains it to Google Tasks with retries.
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Op says what an item does in Google Tasks.
const (
	OpCreate   = ""         // create a new task from Title, Notes, Due
	OpComplete = "complete" // check off TaskID
	OpReopen   = "reopen"   // uncheck TaskID
)

// Item is one change waiting to reach Google Tasks: a new task, or a check
// or uncheck of an existing one.
type Item struct {
	ID       string    `json:"id"`
	Op       string    `json:"op,omitempty"`
	TaskID   string    `json:"task_id,omitempty"`
	ListKey  string    `json:"list_key"`
	Title    string    `json:"title"`
	Notes    string    `json:"notes,omitempty"`
	Due      time.Time `json:"due"`
	Created  time.Time `json:"created"`
	Attempts int       `json:"attempts"`
	LastErr  string    `json:"last_err,omitempty"`
}

// Sender creates the task remotely. A *Permanent error stops retries.
type Sender interface {
	Send(ctx context.Context, it Item) error
}

// Permanent marks an error that will not succeed on retry (unknown list, rejected request).
type Permanent struct{ Err error }

func (p *Permanent) Error() string { return p.Err.Error() }
func (p *Permanent) Unwrap() error { return p.Err }

// Queue stores one JSON file per item in dir.
type Queue struct {
	dir    string
	mu     sync.Mutex
	wake   chan struct{}
	OnIdle func(pending, stuck int) // called after each drain pass, e.g. to update the tray badge
}

// Open creates the queue directory if needed.
func Open(dir string) (*Queue, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Queue{dir: dir, wake: make(chan struct{}, 1)}, nil
}

// Put writes the item durably and wakes the pusher. An item with the ID of
// one still waiting replaces it.
func (q *Queue) Put(it Item) error {
	if it.ID == "" {
		it.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if it.Created.IsZero() {
		it.Created = time.Now()
	}
	if err := q.write(it); err != nil {
		return err
	}
	select {
	case q.wake <- struct{}{}:
	default:
	}
	return nil
}

func (q *Queue) path(id string) string { return filepath.Join(q.dir, id+".json") }

// ToggleID is the item id for checking or unchecking a task, so a later
// toggle of the same task replaces the one still waiting.
func ToggleID(taskID string) string { return "toggle-" + taskID }

// same reports whether the stored item is still the one drain picked up,
// i.e. no newer Put replaced it meanwhile. Call with q.mu held.
func (q *Queue) same(it Item) bool {
	data, err := os.ReadFile(q.path(it.ID))
	if err != nil {
		return false
	}
	var cur Item
	return json.Unmarshal(data, &cur) == nil && cur.Created.Equal(it.Created) && cur.Op == it.Op
}

func (q *Queue) write(it Item) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.writeLocked(it)
}

func (q *Queue) writeLocked(it Item) error {
	data, err := json.Marshal(it)
	if err != nil {
		return err
	}
	tmp := q.path(it.ID) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, q.path(it.ID))
}

// List returns pending items oldest first.
func (q *Queue) List() ([]Item, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		return nil, err
	}
	var items []Item
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(q.dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var it Item
		if err := json.Unmarshal(data, &it); err != nil {
			slog.Warn("queue: skipping unreadable item", "file", e.Name(), "err", err)
			continue
		}
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Created.Before(items[j].Created) })
	return items, nil
}

// Remove deletes a delivered item.
func (q *Queue) Remove(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	err := os.Remove(q.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// finish removes a delivered item, or records a failed attempt, unless a
// newer Put replaced it while it was being sent.
func (q *Queue) finish(it Item, delivered bool) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.same(it) {
		return nil
	}
	if delivered {
		err := os.Remove(q.path(it.ID))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return q.writeLocked(it)
}

// MaxAttempts is where an item is considered stuck and left for the user.
const MaxAttempts = 20

// Run drains the queue until ctx ends: at once when Put wakes it, and on a
// backoff timer while anything is pending.
func (q *Queue) Run(ctx context.Context, s Sender) {
	backoff := 5 * time.Second
	for {
		pending, stuck := q.drain(ctx, s)
		if q.OnIdle != nil {
			q.OnIdle(pending, stuck)
		}
		wait := time.Hour
		if pending > 0 {
			wait = backoff
			if backoff < 5*time.Minute {
				backoff *= 2
			}
		} else {
			backoff = 5 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-q.wake:
			backoff = 5 * time.Second
		case <-time.After(wait):
		}
	}
}

func (q *Queue) drain(ctx context.Context, s Sender) (pending, stuck int) {
	items, err := q.List()
	if err != nil {
		slog.Error("queue: list", "err", err)
		return 0, 0
	}
	for _, it := range items {
		if it.Attempts >= MaxAttempts {
			stuck++
			continue
		}
		err := s.Send(ctx, it)
		if err == nil {
			if err := q.finish(it, true); err != nil {
				slog.Error("queue: remove", "id", it.ID, "err", err)
			}
			continue
		}
		it.Attempts++
		it.LastErr = err.Error()
		var perm *Permanent
		if errors.As(err, &perm) {
			it.Attempts = MaxAttempts
			stuck++
			slog.Warn("queue: item stuck", "title", it.Title, "err", err)
		} else {
			pending++
			slog.Info("queue: retry later", "title", it.Title, "attempt", it.Attempts, "err", err)
		}
		if werr := q.finish(it, false); werr != nil {
			slog.Error("queue: update", "id", it.ID, "err", werr)
		}
		if ctx.Err() != nil {
			return pending, stuck
		}
	}
	return pending, stuck
}
