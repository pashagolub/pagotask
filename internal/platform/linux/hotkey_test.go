//go:build linux

package linux

import (
	"reflect"
	"testing"

	"github.com/pashagolub/pagotask/internal/platform"
)

func TestAccel(t *testing.T) {
	c, err := platform.ParseCombo("Win+Shift+T")
	if err != nil {
		t.Fatal(err)
	}
	if got := accel(c); got != "<Super><Shift>t" {
		t.Fatalf("got %q", got)
	}
	c, _ = platform.ParseCombo("Ctrl+Alt+Space")
	if got := accel(c); got != "<Control><Alt>space" {
		t.Fatalf("got %q", got)
	}
}

func TestStrv(t *testing.T) {
	if got := parseStrv("@as []"); len(got) != 0 {
		t.Fatalf("empty: %v", got)
	}
	in := "['/org/a/custom0/', '/org/a/pagotask-add/']"
	want := []string{"/org/a/custom0/", "/org/a/pagotask-add/"}
	got := parseStrv(in)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	if formatStrv(got) != in {
		t.Fatalf("format: %s", formatStrv(got))
	}
}

func TestQuoteArg(t *testing.T) {
	if quoteArg("/usr/bin/pagotask") != "/usr/bin/pagotask" {
		t.Fatal("plain path quoted")
	}
	if got := quoteArg("/home/me/my apps/pagotask"); got != "'/home/me/my apps/pagotask'" {
		t.Fatalf("got %s", got)
	}
}
