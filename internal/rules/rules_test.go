package rules

import (
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

	issue := Capture{Process: "chrome.exe", URL: "https://github.com/someone/tool/issues/7"}
	d = Prefill(c, issue)
	if d.Tag != "issue" || d.Title != "tool #7" || d.List != "c" {
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
}
