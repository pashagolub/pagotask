package opentasks

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/pashagolub/pagotask/internal/queue"
)

var lists = map[string]string{"w": "Work", "p": "Personal"}

func ids(rows []Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

func TestRowsOrderAndWhen(t *testing.T) {
	now := time.Now()
	s := &Store{over: map[string]override{}, tasks: []Task{
		{ID: "undated-old", ListKey: "p", Title: "old idea", Updated: now.Add(-time.Hour)},
		{ID: "later", ListKey: "w", Title: "later", Due: "2026-10-01"},
		{ID: "today-p", ListKey: "p", Title: "today personal", Due: "2026-09-27"},
		{ID: "today-w2", ListKey: "w", Title: "today work 2", Due: "2026-09-27", Position: "2"},
		{ID: "today-w1", ListKey: "w", Title: "today work 1", Due: "2026-09-27", Position: "1"},
		{ID: "overdue", ListKey: "w", Title: "overdue", Due: "2026-09-20"},
		{ID: "undated-new", ListKey: "p", Title: "new idea", Updated: now},
		{ID: "sub", ListKey: "w", Title: "subtask", ParentID: "overdue", Due: "2026-09-27"},
		{ID: "other", ListKey: "x", Title: "not configured", Due: "2026-09-27"},
	}}
	rows := s.Rows(lists, nil, "2026-09-27")
	got := ids(rows)
	// "sub" has no position, so it sorts first among Work tasks due today.
	want := []string{"overdue", "today-p", "sub", "today-w1", "today-w2", "later", "undated-new", "undated-old"}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows = %v, want %v", got, want)
		}
	}
	whens := map[string]string{}
	for _, r := range rows {
		whens[r.ID] = r.When
		if r.ID == "sub" && r.Parent != "overdue" {
			t.Errorf("sub parent = %q", r.Parent)
		}
	}
	if whens["overdue"] != Overdue || whens["today-p"] != Today || whens["later"] != Later || whens["undated-new"] != Undated {
		t.Errorf("when = %v", whens)
	}
}

func TestQueuedCreatesShowAsSyncing(t *testing.T) {
	s := &Store{over: map[string]override{}}
	due := time.Date(2026, 9, 27, 0, 0, 0, 0, time.Local)
	queued := []queue.Item{
		{ID: "1", ListKey: "w", Title: "🇵🇷 pgwatch #345", Due: due, Notes: "see https://github.com/cybertec-postgresql/pgwatch/pull/345 now"},
		{ID: "2", Op: queue.OpComplete, TaskID: "x", ListKey: "w"},
		{ID: "3", ListKey: "w", Title: "stuck", Attempts: queue.MaxAttempts},
	}
	rows := s.Rows(lists, queued, "2026-09-27")
	if len(rows) != 1 || !rows[0].Syncing || rows[0].When != Today {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].Link != "https://github.com/cybertec-postgresql/pgwatch/pull/345" {
		t.Errorf("link = %q", rows[0].Link)
	}
}

func TestOverridesSurviveUntilGoogleHasThem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	s := Open(path)
	task := Task{ID: "a", ListKey: "w", Title: "a", Due: "2026-09-27"}
	if err := s.Replace([]Task{task}, time.Now(), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Mark("a", true); err != nil {
		t.Fatal(err)
	}
	done := func(s *Store) bool {
		rows := s.Rows(lists, nil, "2026-09-27")
		return len(rows) == 1 && rows[0].Done
	}
	if !done(s) {
		t.Fatal("mark not applied")
	}
	// Survives a restart.
	s = Open(path)
	if !done(s) {
		t.Fatal("mark lost on reload")
	}
	// A fetch that started before the check keeps it.
	if err := s.Replace([]Task{task}, time.Now().Add(-time.Minute), nil); err != nil {
		t.Fatal(err)
	}
	if !done(s) {
		t.Fatal("mark dropped by an older fetch")
	}
	// A later fetch while the check was still queued keeps it too.
	if err := s.Replace([]Task{task}, time.Now().Add(time.Minute), map[string]bool{"a": true}); err != nil {
		t.Fatal(err)
	}
	if !done(s) {
		t.Fatal("mark dropped while still queued")
	}
	// A later fetch after delivery is the truth.
	if err := s.Replace(nil, time.Now().Add(time.Minute), nil); err != nil {
		t.Fatal(err)
	}
	if len(s.over) != 0 || len(s.Rows(lists, nil, "2026-09-27")) != 0 {
		t.Fatalf("override kept: %v", s.over)
	}
}

func TestFirstLink(t *testing.T) {
	if got := FirstLink("no link", []string{"", "https://mail.google.com/x"}); got != "https://mail.google.com/x" {
		t.Errorf("got %q", got)
	}
	if got := FirstLink(`"https://a.b/c" and https://d.e`, nil); got != "https://a.b/c" {
		t.Errorf("got %q", got)
	}
}
