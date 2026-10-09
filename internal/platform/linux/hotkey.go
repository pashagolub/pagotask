//go:build linux

package linux

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/pashagolub/pagotask/internal/platform"
)

// Hotkey is a GNOME custom shortcut that runs "pagotask <verb>". Wayland
// gives apps no global key grabs, so the desktop owns the key and the
// command wakes the running instance (see package instance). The shortcut
// outlives the app on purpose: pressing it with pagotask closed starts it.
type Hotkey struct{ verb string }

// NewHotkey returns the shortcut for verb ("add" or "tasks").
func NewHotkey(verb string) *Hotkey { return &Hotkey{verb: verb} }

// Register implements platform.Hotkey. fn is not called from here: the
// command reaches it through the instance package.
func (h *Hotkey) Register(combo string, fn func()) error {
	c, err := platform.ParseCombo(combo)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if !gnome() {
		return fmt.Errorf("not a GNOME desktop: bind %s to %q in your desktop's keyboard settings", combo, exe+" "+h.verb)
	}
	return setShortcut(h.verb, accel(c), quoteArg(exe)+" "+h.verb)
}

// Unregister implements platform.Hotkey. It leaves the shortcut in place.
func (h *Hotkey) Unregister() error { return nil }

func gnome() bool {
	d := strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP"))
	if !strings.Contains(d, "gnome") && !strings.Contains(d, "unity") {
		return false
	}
	_, err := exec.LookPath("gsettings")
	return err == nil
}

// accel turns a combo into GTK accelerator syntax, e.g. "<Super><Shift>t".
func accel(c platform.Combo) string {
	var b strings.Builder
	if c.Win {
		b.WriteString("<Super>")
	}
	if c.Ctrl {
		b.WriteString("<Control>")
	}
	if c.Alt {
		b.WriteString("<Alt>")
	}
	if c.Shift {
		b.WriteString("<Shift>")
	}
	b.WriteString(strings.ToLower(c.Key))
	return b.String()
}

const (
	mediaKeys  = "org.gnome.settings-daemon.plugins.media-keys"
	customKey  = mediaKeys + ".custom-keybinding"
	customBase = "/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/"
)

// setShortcut writes the pagotask-<verb> custom shortcut and adds it to
// GNOME's list, touching nothing when it is already as wanted.
func setShortcut(verb, binding, command string) error {
	path := customBase + "pagotask-" + verb + "/"
	schema := customKey + ":" + path
	want := map[string]string{"name": "pagotask " + verb, "command": command, "binding": binding}
	for _, k := range []string{"name", "command", "binding"} {
		cur, err := gsettings("get", schema, k)
		if err == nil && cur == gvString(want[k]) {
			continue
		}
		if _, err := gsettings("set", schema, k, gvString(want[k])); err != nil {
			return err
		}
	}
	list, err := gsettings("get", mediaKeys, "custom-keybindings")
	if err != nil {
		return err
	}
	paths := parseStrv(list)
	for _, p := range paths {
		if p == path {
			return nil
		}
	}
	paths = append(paths, path)
	if _, err := gsettings("set", mediaKeys, "custom-keybindings", formatStrv(paths)); err != nil {
		return err
	}
	slog.Info("added GNOME shortcut", "binding", binding, "command", command)
	return nil
}

func gsettings(args ...string) (string, error) {
	out, err := exec.Command("gsettings", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gsettings %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// gvString is s as a GVariant string literal.
func gvString(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

// parseStrv reads gsettings' print of a string array: "@as []" or
// "['/a/', '/b/']". Keybinding paths hold no quotes or commas.
func parseStrv(s string) []string {
	s = strings.TrimSpace(strings.TrimPrefix(s, "@as"))
	s = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.Trim(strings.TrimSpace(p), `'"`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func formatStrv(v []string) string {
	q := make([]string, len(v))
	for i, s := range v {
		q[i] = gvString(s)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// quoteArg quotes a path for the shortcut's command line when needed.
func quoteArg(s string) string {
	if !strings.ContainsAny(s, " \t'\"\\") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
