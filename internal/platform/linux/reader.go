//go:build linux

package linux

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/pashagolub/pagotask/internal/config"
	"github.com/pashagolub/pagotask/internal/rules"
)

// Reader reads the foreground window through AT-SPI, the Linux
// accessibility bus. It works on Wayland, where apps cannot see each
// other's windows otherwise. For X11 windows the X server also names the
// active window's process and title, used when AT-SPI has nothing.
type Reader struct {
	mu   sync.Mutex
	conn *dbus.Conn // the accessibility bus, opened on first use
}

// NewReader returns a Reader and switches desktop accessibility on, which
// browsers need to publish their pages; browsers started before that
// show up after a restart.
func NewReader() *Reader {
	if err := enableA11y(); err != nil {
		slog.Warn("could not switch accessibility on", "err", err)
	}
	return &Reader{}
}

// AT-SPI roles and states used here (atspi-constants.h).
const (
	roleDocumentFrame = 82
	roleDocumentWeb   = 95

	stateActive  = 1
	stateFocused = 12
	stateShowing = 25
)

const (
	registry = "org.a11y.atspi.Registry"
	rootPath = "/org/a11y/atspi/accessible/root"
	ifAcc    = "org.a11y.atspi.Accessible"

	maxNodes = 600 // per capture; browser trees are big
)

type ref struct {
	Name string
	Path dbus.ObjectPath
}

// Foreground implements platform.PageReader.
func (r *Reader) Foreground(source func(process string) config.Source) (rules.Capture, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	xpid, xtitle := x11Active()
	cap, err := r.atspi(ctx, source, xpid, xtitle)
	if err != nil {
		r.reset()
	}
	if cap.Process == "" && xpid > 0 {
		cap.Process, cap.Title = processName(xpid), xtitle
	}
	return cap, err
}

func (r *Reader) atspi(ctx context.Context, source func(string) config.Source, xpid int, xtitle string) (rules.Capture, error) {
	conn, err := r.bus()
	if err != nil {
		return rules.Capture{}, err
	}
	app, frame, ok := activeFrame(ctx, conn, xpid, xtitle)
	if !ok {
		return rules.Capture{}, ctx.Err()
	}
	cap := rules.Capture{Title: name(ctx, conn, frame), Process: processOf(ctx, conn, app.Name)}
	switch src := source(cap.Process); src.Read {
	case "url":
		cap.URL = documentURL(ctx, conn, frame)
	case "control":
		cap.Text = controlText(ctx, conn, frame, src.Control)
	}
	return cap, nil
}

func (r *Reader) bus() (*dbus.Conn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn != nil {
		return r.conn, nil
	}
	session, err := dbus.SessionBus()
	if err != nil {
		return nil, err
	}
	var addr string
	if err := session.Object("org.a11y.Bus", "/org/a11y/bus").Call("org.a11y.Bus.GetAddress", 0).Store(&addr); err != nil {
		return nil, err
	}
	conn, err := dbus.Connect(addr)
	if err != nil {
		return nil, err
	}
	r.conn = conn
	return conn, nil
}

func (r *Reader) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn != nil {
		_ = r.conn.Close()
		r.conn = nil
	}
}

// activeFrame finds the top-level window with the "active" state,
// skipping pagotask's own windows. Not every app keeps that state (Chrome
// may not), so when none has it the X server's active window, if any,
// picks the app by process id and the window by title.
func activeFrame(ctx context.Context, conn *dbus.Conn, xpid int, xtitle string) (app, frame ref, ok bool) {
	self := uint32(os.Getpid())
	var xapp ref
	var xframes []ref
	for _, a := range children(ctx, conn, ref{registry, rootPath}) {
		if ctx.Err() != nil {
			return ref{}, ref{}, false
		}
		p := pid(ctx, conn, a.Name)
		if p == self {
			continue
		}
		frames := children(ctx, conn, a)
		for _, w := range frames {
			if hasState(states(ctx, conn, w), stateActive) {
				return a, w, true
			}
		}
		if xpid > 0 && int(p) == xpid {
			xapp, xframes = a, frames
		}
	}
	var showing []ref
	for _, w := range xframes {
		if !hasState(states(ctx, conn, w), stateShowing) {
			continue
		}
		if name(ctx, conn, w) == xtitle {
			return xapp, w, true
		}
		showing = append(showing, w)
	}
	if len(showing) == 1 {
		return xapp, showing[0], true
	}
	return ref{}, ref{}, false
}

// documentURL walks the window's visible tree to the web document and
// reads its address.
func documentURL(ctx context.Context, conn *dbus.Conn, frame ref) string {
	var url string
	walk(ctx, conn, frame, func(n ref) bool {
		switch role(ctx, conn, n) {
		case roleDocumentWeb, roleDocumentFrame:
			for _, attr := range []string{"DocURL", "URI"} {
				var v string
				if conn.Object(n.Name, n.Path).CallWithContext(ctx, "org.a11y.atspi.Document.GetAttributeValue", 0, attr).Store(&v) == nil && v != "" {
					url = v
					return false
				}
			}
			return true // a document without an address: skip its content
		}
		return true
	})
	return url
}

// controlText finds the element whose accessible id or name is want and
// returns its text, or its name when it has no text. When none matches it
// logs the focused element, so the right id can be put in config.yaml.
func controlText(ctx context.Context, conn *dbus.Conn, frame ref, want string) string {
	var text string
	found := false
	walk(ctx, conn, frame, func(n ref) bool {
		nm := name(ctx, conn, n)
		id := property(ctx, conn, n, ifAcc+".AccessibleId")
		if nm != want && id != want {
			if hasState(states(ctx, conn, n), stateFocused) {
				slog.Info("focused control", "id", id, "name", nm)
			}
			return true
		}
		found = true
		var s string
		if conn.Object(n.Name, n.Path).CallWithContext(ctx, "org.a11y.atspi.Text.GetText", 0, int32(0), int32(-1)).Store(&s) == nil && s != "" {
			text = s
		} else {
			text = nm
		}
		return false
	})
	if !found {
		slog.Info("control not found", "control", want)
	}
	return text
}

// walk visits the showing descendants of root breadth first until visit
// returns false, the node budget runs out or ctx ends. A node is visited
// before its children; documents are not descended into.
func walk(ctx context.Context, conn *dbus.Conn, root ref, visit func(ref) bool) {
	queue := children(ctx, conn, root)
	for seen := 0; len(queue) > 0 && seen < maxNodes && ctx.Err() == nil; seen++ {
		n := queue[0]
		queue = queue[1:]
		if !hasState(states(ctx, conn, n), stateShowing) {
			continue
		}
		if !visit(n) {
			return
		}
		if r := role(ctx, conn, n); r == roleDocumentWeb || r == roleDocumentFrame {
			continue
		}
		queue = append(queue, children(ctx, conn, n)...)
	}
}

func children(ctx context.Context, conn *dbus.Conn, n ref) []ref {
	var out []ref
	_ = conn.Object(n.Name, n.Path).CallWithContext(ctx, ifAcc+".GetChildren", 0).Store(&out)
	return out
}

func states(ctx context.Context, conn *dbus.Conn, n ref) []uint32 {
	var s []uint32
	_ = conn.Object(n.Name, n.Path).CallWithContext(ctx, ifAcc+".GetState", 0).Store(&s)
	return s
}

func hasState(s []uint32, bit uint) bool {
	i := bit / 32
	return int(i) < len(s) && s[i]&(1<<(bit%32)) != 0
}

func role(ctx context.Context, conn *dbus.Conn, n ref) uint32 {
	var r uint32
	_ = conn.Object(n.Name, n.Path).CallWithContext(ctx, ifAcc+".GetRole", 0).Store(&r)
	return r
}

func name(ctx context.Context, conn *dbus.Conn, n ref) string {
	return property(ctx, conn, n, ifAcc+".Name")
}

func property(ctx context.Context, conn *dbus.Conn, n ref, prop string) string {
	i := strings.LastIndex(prop, ".")
	var v dbus.Variant
	if conn.Object(n.Name, n.Path).CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, prop[:i], prop[i+1:]).Store(&v) != nil {
		return ""
	}
	s, _ := v.Value().(string)
	return s
}

func pid(ctx context.Context, conn *dbus.Conn, busName string) uint32 {
	var p uint32
	_ = conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetConnectionUnixProcessID", 0, busName).Store(&p)
	return p
}

func processOf(ctx context.Context, conn *dbus.Conn, busName string) string {
	return processName(int(pid(ctx, conn, busName)))
}

// processName is the executable's file name ("firefox", "chrome", "code").
func processName(pid int) string {
	if pid <= 0 {
		return ""
	}
	if exe, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe"); err == nil {
		return filepath.Base(strings.TrimSuffix(exe, " (deleted)"))
	}
	comm, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(comm))
}

// enableA11y sets org.a11y.Status.IsEnabled, GNOME's accessibility switch.
func enableA11y() error {
	session, err := dbus.SessionBus()
	if err != nil {
		return err
	}
	obj := session.Object("org.a11y.Bus", "/org/a11y/bus")
	v, err := obj.GetProperty("org.a11y.Status.IsEnabled")
	if err != nil {
		return err
	}
	if on, _ := v.Value().(bool); on {
		return nil
	}
	if err := obj.SetProperty("org.a11y.Status.IsEnabled", dbus.MakeVariant(true)); err != nil {
		return err
	}
	slog.Info("switched desktop accessibility on so browsers publish their address")
	return nil
}

// x11Active reads the X server's active window: its process id and title.
// It finds X11 and XWayland windows; on a pure Wayland desktop it finds
// nothing. Needs xprop (x11-utils).
func x11Active() (pid int, title string) {
	if os.Getenv("DISPLAY") == "" {
		return 0, ""
	}
	out, err := exec.Command("xprop", "-root", "_NET_ACTIVE_WINDOW").Output()
	if err != nil {
		return 0, ""
	}
	f := strings.Fields(string(out))
	if len(f) == 0 {
		return 0, ""
	}
	id := strings.TrimSuffix(f[len(f)-1], ",")
	if id == "0x0" {
		return 0, ""
	}
	out, err = exec.Command("xprop", "-id", id, "_NET_WM_PID", "_NET_WM_NAME").Output()
	if err != nil {
		return 0, ""
	}
	return parseXprop(string(out))
}

func parseXprop(out string) (pid int, title string) {
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(line, " = ")
		if !ok {
			continue
		}
		switch {
		case strings.HasPrefix(k, "_NET_WM_PID"):
			pid, _ = strconv.Atoi(strings.TrimSpace(v))
		case strings.HasPrefix(k, "_NET_WM_NAME"):
			if s, err := strconv.Unquote(strings.TrimSpace(v)); err == nil {
				title = s
			}
		}
	}
	return pid, title
}
