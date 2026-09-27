package platform

import (
	"fmt"
	"strings"
)

// Combo is a parsed shortcut.
type Combo struct {
	Win, Ctrl, Alt, Shift bool
	Key                   string // single upper-case letter, digit, or "SPACE"
}

// ParseCombo parses "Win+Shift+T" style shortcuts.
func ParseCombo(s string) (Combo, error) {
	var c Combo
	parts := strings.Split(s, "+")
	for i, p := range parts {
		p = strings.TrimSpace(p)
		mod := strings.ToLower(p)
		last := i == len(parts)-1
		switch {
		case !last && (mod == "win" || mod == "super" || mod == "cmd"):
			c.Win = true
		case !last && mod == "ctrl":
			c.Ctrl = true
		case !last && mod == "alt":
			c.Alt = true
		case !last && mod == "shift":
			c.Shift = true
		case last:
			k := strings.ToUpper(p)
			if k == "SPACE" || (len(k) == 1 && (k[0] >= 'A' && k[0] <= 'Z' || k[0] >= '0' && k[0] <= '9')) {
				c.Key = k
			} else {
				return c, fmt.Errorf("hotkey %q: unsupported key %q", s, p)
			}
		default:
			return c, fmt.Errorf("hotkey %q: unknown modifier %q", s, p)
		}
	}
	if c.Key == "" {
		return c, fmt.Errorf("hotkey %q: no key", s)
	}
	if !c.Win && !c.Ctrl && !c.Alt {
		return c, fmt.Errorf("hotkey %q: needs Win, Ctrl or Alt", s)
	}
	return c, nil
}
