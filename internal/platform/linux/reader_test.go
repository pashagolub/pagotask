//go:build linux

package linux

import "testing"

func TestHasState(t *testing.T) {
	s := []uint32{1 << stateActive}
	if !hasState(s, stateActive) {
		t.Fatal("active not seen")
	}
	if hasState([]uint32{0, 0}, stateShowing) {
		t.Fatal("showing seen in empty set")
	}
	if !hasState([]uint32{1 << stateShowing, 0}, stateShowing) {
		t.Fatal("showing not seen")
	}
	if hasState(nil, 40) {
		t.Fatal("out of range bit seen")
	}
}

func TestParseXprop(t *testing.T) {
	pid, title := parseXprop("_NET_WM_PID(CARDINAL) = 4242\n_NET_WM_NAME(UTF8_STRING) = \"pgwatch #345 \\\"x\\\" — Mozilla Firefox\"\n")
	if pid != 4242 || title != `pgwatch #345 "x" — Mozilla Firefox` {
		t.Fatalf("pid %d title %q", pid, title)
	}
}
