package instance

import (
	"path/filepath"
	"testing"
	"time"
)

func TestVerbReachesHandler(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.sock")
	if err := send(path, "add"); err == nil {
		t.Fatal("send with nobody listening should fail")
	}
	if err := listen(path); err != nil {
		t.Fatal(err)
	}
	got := make(chan string, 2)
	Handle("add", func() { got <- "add" })
	if err := send(path, "add"); err != nil {
		t.Fatal(err)
	}
	select {
	case v := <-got:
		if v != "add" {
			t.Fatalf("got %q", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler not called")
	}
}

func TestPendingVerbRunsOnRegister(t *testing.T) {
	run("tasks")
	got := make(chan bool, 1)
	Handle("tasks", func() { got <- true })
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("pending verb not run")
	}
	Handle("tasks", nil)
}
