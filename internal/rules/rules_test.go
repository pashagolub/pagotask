package rules

import (
	"strings"
	"testing"

	"github.com/pashagolub/pagotask/internal/config"
)

func TestPrefill(t *testing.T) {
	c, err := config.Parse(config.Default)
	if err != nil {
		t.Fatal(err)
	}
	pr := Capture{
		Process: "firefox.exe",
		Title:   "Add metric X by foo · Pull Request #345 · cybertec-postgresql/pgwatch — Mozilla Firefox",
		URL:     "https://github.com/cybertec-postgresql/pgwatch/pull/345/files",
	}
	d := Prefill(c, pr)
	if d.Tag != "pr" || d.Title != "pgwatch #345" || d.List != "w" || d.Notes != pr.URL {
		t.Errorf("pr draft = %+v", d)
	}
	if got := FullTitle(c, d); got != "🇵🇷 pgwatch #345" {
		t.Errorf("FullTitle = %q", got)
	}

	// Address bar unreadable: the tab title still identifies the PR.
	noURL := Capture{
		Process: "firefox.exe",
		Title:   "Switch to Wails v3: popup and tray in one app by pashagolub · Pull Request #6 · pashagolub/pagotask — Mozilla Firefox",
	}
	d = Prefill(c, noURL)
	if d.Tag != "pr" || d.Title != "pagotask #6" || d.List != "c" {
		t.Errorf("title-only pr draft = %+v", d)
	}
	d = Prefill(c, Capture{Process: "firefox.exe", Title: "Crash on start · Issue #12 · cybertec-postgresql/pgwatch — Mozilla Firefox"})
	if d.Tag != "is" || d.Title != "pgwatch #12" || d.List != "w" {
		t.Errorf("title-only issue draft = %+v", d)
	}

	issue := Capture{Process: "chrome.exe", URL: "https://github.com/someone/tool/issues/7"}
	d = Prefill(c, issue)
	if d.Tag != "is" || d.Title != "tool #7" || d.List != "c" {
		t.Errorf("issue draft = %+v", d)
	}

	generic := Capture{Process: "msedge.exe", Title: "Some article - Google Chrome", URL: "https://example.com/a"}
	d = Prefill(c, generic)
	if d.Tag != "" || d.Title != "Some article" || d.Notes != generic.URL || d.List != "p" {
		t.Errorf("generic draft = %+v", d)
	}

	other := Capture{Process: "WindowsTerminal.exe", Title: "pwsh"}
	d = Prefill(c, other)
	if d.Tag != "" || d.Title != "" || d.Notes != "" {
		t.Errorf("other draft = %+v", d)
	}

	// read: none skips rules even when the title would match one.
	c.Sources["Thunderbird.exe"] = config.Source{Read: "none"}
	d = Prefill(c, Capture{Process: "Thunderbird.exe", Title: noURL.Title})
	if d.Tag != "" || d.Title != "" || d.List != c.DefaultList {
		t.Errorf("read none draft = %+v", d)
	}

	// read: control: the control's text stands in for the window title.
	c.Sources["MailClient.exe"] = config.Source{Read: "control", Control: "subject"}
	d = Prefill(c, Capture{Process: "MailClient.exe", Title: "Inbox - MailClient", Text: "Quarterly report"})
	if d.Tag != "" || d.Title != "Quarterly report" || d.Notes != "" {
		t.Errorf("read control draft = %+v", d)
	}
	d = Prefill(c, Capture{Process: "MailClient.exe", Title: "Inbox - MailClient", Text: noURL.Title})
	if d.Tag != "pr" || d.Title != "pagotask #6" {
		t.Errorf("read control rule draft = %+v", d)
	}
	d = Prefill(c, Capture{Process: "MailClient.exe", Title: "Inbox - MailClient"})
	if d.Title != "" {
		t.Errorf("read control, control not found: draft = %+v", d)
	}
}

func TestSplitTitle(t *testing.T) {
	c, err := config.Parse(config.Default)
	if err != nil {
		t.Fatal(err)
	}
	pr := FullTitle(c, Draft{Tag: "pr", Title: "pgwatch #345"})
	for _, tc := range []struct{ in, tag, text string }{
		{pr, "pr", "pgwatch #345"},
		{"plain title", "", "plain title"},
	} {
		d := SplitTitle(c, tc.in)
		if d.Tag != tc.tag || d.Title != tc.text {
			t.Errorf("SplitTitle(%q) = %+v, want %q %q", tc.in, d, tc.tag, tc.text)
		}
	}
	// Every tag round-trips, with or without the variation selector.
	for id, tag := range c.Tags {
		full := FullTitle(c, Draft{Tag: id, Title: "x"})
		d := SplitTitle(c, strings.ReplaceAll(full, "️", ""))
		if d.Title != "x" || c.Tags[d.Tag].Emoji != tag.Emoji {
			t.Errorf("tag %s: SplitTitle(%q) = %+v", id, full, d)
		}
	}
}
