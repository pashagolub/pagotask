// Package rules turns what was captured from the foreground window into a prefilled task draft.
package rules

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/pashagolub/pagotask/internal/config"
)

// Capture is what the platform layer read from the foreground window.
type Capture struct {
	Process string // e.g. "firefox.exe"
	Title   string // window title
	URL     string // address bar, empty when not a browser or not readable
	Text    string // text of the control named by a read: control source
}

// Draft is the prefilled state of the popup.
type Draft struct {
	Tag   string // tag id, "" when untagged
	Title string // text after the emoji
	List  string // list key
	Notes string
}

// browserSuffix strips " — Mozilla Firefox", " - Google Chrome" etc.
var browserSuffix = regexp.MustCompile(`\s+[-—–]\s+(Mozilla Firefox|Google Chrome|Microsoft.*Edge|Chromium|Brave|Vivaldi|Opera)(\s+\(.*\))?$`)

// Prefill applies the first matching rule; with no match it prefills
// generically from a browser (title + URL in notes) and returns an empty
// draft for any other application.
func Prefill(c *config.Config, cap Capture) Draft {
	d := Draft{List: c.DefaultList}
	src := c.Sources[cap.Process]
	if src.Read == "none" {
		return d
	}
	if src.Read == "control" && cap.Text != "" {
		cap.Title = cap.Text // rules and prefill use the control instead of the window title
	}
	for i := range c.Rules {
		r := &c.Rules[i]
		subject := cap.URL
		if r.On == "title" {
			subject = cap.Title
		}
		if subject == "" {
			continue
		}
		m := r.Regexp().FindStringSubmatch(subject)
		if m == nil {
			continue
		}
		tag := c.Tags[r.Tag]
		d.Tag = r.Tag
		d.Title = expand(r.Title, m)
		if d.Title == "" {
			d.Title = cleanTitle(cap.Title)
		}
		d.List = c.DefaultList
		if tag.List != "" {
			d.List = tag.List
		}
		if l := r.ListFor(m[0]); l != "" {
			d.List = l
		}
		d.Notes = cap.URL
		return d
	}
	switch {
	case src.Read == "url":
		d.Title = cleanTitle(cap.Title)
		d.Notes = cap.URL
	case src.Read == "control" && cap.Text != "":
		d.Title = cap.Text
	}
	return d
}

var placeholder = regexp.MustCompile(`\{(\d+)\}`)

func expand(tmpl string, m []string) string {
	return placeholder.ReplaceAllStringFunc(tmpl, func(p string) string {
		n, _ := strconv.Atoi(p[1 : len(p)-1])
		if n < len(m) {
			return m[n]
		}
		return ""
	})
}

func cleanTitle(t string) string {
	return strings.TrimSpace(browserSuffix.ReplaceAllString(t, ""))
}

// FullTitle renders the final Google Tasks title: "<emoji> <text>".
func FullTitle(c *config.Config, d Draft) string {
	if t, ok := c.Tags[d.Tag]; ok {
		return t.Emoji + " " + strings.TrimSpace(d.Title)
	}
	return strings.TrimSpace(d.Title)
}

// SplitTitle is FullTitle in reverse: a title that starts with a tag's
// emoji gives that tag and the text after it. Emoji variation selectors do
// not matter; with no matching emoji the tag is empty and the text is the
// whole title.
func SplitTitle(c *config.Config, title string) Draft {
	title = strings.TrimSpace(title)
	bare := strings.ReplaceAll(title, "️", "")
	best, bestLen := "", 0
	for id, t := range c.Tags {
		e := strings.ReplaceAll(t.Emoji, "️", "")
		if e == "" || !strings.HasPrefix(bare, e) {
			continue
		}
		// The longest emoji wins (a flag over its first letter); ties go to the lower id.
		if len(e) > bestLen || len(e) == bestLen && id < best {
			best, bestLen = id, len(e)
		}
	}
	if best == "" {
		return Draft{Title: title}
	}
	return Draft{Tag: best, Title: strings.TrimSpace(bare[bestLen:])}
}
