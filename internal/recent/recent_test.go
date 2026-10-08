package recent

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestMergeNewestFirstDistinct(t *testing.T) {
	t0 := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
	s := Open(filepath.Join(t.TempDir(), "recent.json"))
	if err := s.SetGoogle([]Entry{
		{Title: "⏱️ Walking", List: "p", Used: t0},
		{Title: "🛒 Milk", List: "f", Used: t0.Add(2 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("⏱ walking", "p", t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got := s.List()
	if len(got) != 2 || got[0].Title != "🛒 Milk" || got[1].Title != "⏱ walking" {
		t.Fatalf("got %+v", got)
	}

	// It survives a restart.
	again := Open(s.path).List()
	if len(again) != 2 || again[1].Title != "⏱ walking" {
		t.Fatalf("reloaded %+v", again)
	}
}

func TestMax(t *testing.T) {
	s := Open("")
	t0 := time.Now()
	for i := 0; i < Max+5; i++ {
		_ = s.Add(fmt.Sprintf("task %d", i), "p", t0.Add(time.Duration(i)*time.Minute))
	}
	got := s.List()
	if len(got) != Max || got[0].Title != fmt.Sprintf("task %d", Max+4) {
		t.Fatalf("got %d entries, first %q", len(got), got[0].Title)
	}
}
