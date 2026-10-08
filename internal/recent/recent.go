// Package recent keeps the most recently used tasks for the add popup:
// the ones added through pagotask and the ones touched in Google Tasks,
// merged newest first.
package recent

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Max is how many distinct tasks the list keeps.
const Max = 20

// Entry is one recent task. Title is the full Google Tasks title, emoji
// included; List is a config list key.
type Entry struct {
	Title string    `json:"title"`
	List  string    `json:"list"`
	Used  time.Time `json:"used"`
}

// Store is the recent list, persisted as JSON so it survives restarts.
type Store struct {
	path string

	mu     sync.Mutex
	local  []Entry // added through pagotask
	google []Entry // from the last Google fetch
}

type snapshot struct {
	Local  []Entry `json:"local"`
	Google []Entry `json:"google"`
}

// Open loads the list at path; a missing or unreadable file gives an empty store.
func Open(path string) *Store {
	s := &Store{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	var snap snapshot
	if json.Unmarshal(data, &snap) == nil {
		s.local, s.google = snap.Local, snap.Google
	}
	return s
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	data, err := json.Marshal(snapshot{Local: s.local, Google: s.google})
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Add records a task saved in the popup.
func (s *Store) Add(title, list string, at time.Time) error {
	if strings.TrimSpace(title) == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.local = merge(append([]Entry{{Title: title, List: list, Used: at}}, s.local...))
	return s.saveLocked()
}

// SetGoogle replaces the entries fetched from Google.
func (s *Store) SetGoogle(entries []Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.google = merge(entries)
	return s.saveLocked()
}

// List is the merged list, newest first, at most Max entries.
func (s *Store) List() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := make([]Entry, 0, len(s.local)+len(s.google))
	all = append(append(all, s.local...), s.google...)
	return merge(all)
}

// merge sorts newest first, drops repeats of a title (keeping the newest)
// and keeps Max.
func merge(in []Entry) []Entry {
	sorted := append([]Entry(nil), in...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Used.After(sorted[j].Used) })
	seen := map[string]bool{}
	out := make([]Entry, 0, Max)
	for _, e := range sorted {
		k := Key(e.Title)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e)
		if len(out) == Max {
			break
		}
	}
	return out
}

// Key is what makes two titles the same task: case, spacing and emoji
// variation selectors do not matter.
func Key(title string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(title, "️", "")), " "))
}
