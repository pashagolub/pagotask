package queue

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeSender struct {
	fail map[string]error
	sent []string
}

func (f *fakeSender) Send(_ context.Context, it Item) error {
	if err, ok := f.fail[it.Title]; ok {
		return err
	}
	f.sent = append(f.sent, it.Title)
	return nil
}

func TestPutListDrain(t *testing.T) {
	q, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now()
	for i, title := range []string{"ok", "flaky", "dead"} {
		if err := q.Put(Item{ID: title, Title: title, Created: base.Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	items, _ := q.List()
	if len(items) != 3 || items[0].Title != "ok" || items[2].Title != "dead" {
		t.Fatalf("list = %+v", items)
	}
	s := &fakeSender{fail: map[string]error{
		"flaky": errors.New("network"),
		"dead":  &Permanent{Err: errors.New("no such list")},
	}}
	pending, stuck := q.drain(context.Background(), s)
	if pending != 1 || stuck != 1 {
		t.Errorf("pending=%d stuck=%d", pending, stuck)
	}
	if len(s.sent) != 1 || s.sent[0] != "ok" {
		t.Errorf("sent = %v", s.sent)
	}
	items, _ = q.List()
	if len(items) != 2 {
		t.Fatalf("after drain: %d items", len(items))
	}
	for _, it := range items {
		switch it.Title {
		case "flaky":
			if it.Attempts != 1 || it.LastErr != "network" {
				t.Errorf("flaky = %+v", it)
			}
		case "dead":
			if it.Attempts != MaxAttempts {
				t.Errorf("dead = %+v", it)
			}
		}
	}
	// Second pass: flaky recovers, dead is skipped.
	delete(s.fail, "flaky")
	pending, stuck = q.drain(context.Background(), s)
	if pending != 0 || stuck != 1 {
		t.Errorf("second pass pending=%d stuck=%d", pending, stuck)
	}
}
