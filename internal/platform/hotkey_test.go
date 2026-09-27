package platform

import "testing"

func TestParseCombo(t *testing.T) {
	c, err := ParseCombo("Win+Shift+T")
	if err != nil || !c.Win || !c.Shift || c.Ctrl || c.Key != "T" {
		t.Errorf("got %+v, %v", c, err)
	}
	c, err = ParseCombo("ctrl+alt+space")
	if err != nil || !c.Ctrl || !c.Alt || c.Key != "SPACE" {
		t.Errorf("got %+v, %v", c, err)
	}
	for _, bad := range []string{"T", "Shift+T", "Win+F13", "Win+", "Meta+T"} {
		if _, err := ParseCombo(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}
