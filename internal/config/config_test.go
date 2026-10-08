package config

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultParses(t *testing.T) {
	c, err := Parse(Default)
	if err != nil {
		t.Fatal(err)
	}
	if c.Hotkey != "Win+Shift+T" {
		t.Errorf("hotkey = %q", c.Hotkey)
	}
	if c.Tasks.Hotkey != "Win+Shift+D" || c.Tasks.Refresh != 5*time.Minute || len(c.TaskLists()) != 4 {
		t.Errorf("tasks = %+v", c.Tasks)
	}
	if len(c.Tags) != 23 {
		t.Errorf("tags = %d, want 23", len(c.Tags))
	}
	if id, _, ok := c.TagByKey("PR"); !ok || id != "pr" {
		t.Errorf("TagByKey(PR) = %q, %v", id, ok)
	}
	if id, _, ok := c.TagByKey("swim"); !ok || id != "walk" {
		t.Errorf("TagByKey(swim) = %q, %v", id, ok)
	}
	if c.Tags["mail"].Key != "mail" {
		t.Errorf("key should default to id, got %q", c.Tags["mail"].Key)
	}
	if got := c.Rules[0].ListFor("cybertec-postgresql/pgwatch"); got != "w" {
		t.Errorf("ListFor = %q, want w", got)
	}
	if got := c.Rules[0].ListFor("other/repo"); got != "" {
		t.Errorf("ListFor = %q, want empty", got)
	}
}

func TestValidation(t *testing.T) {
	cases := map[string]string{
		"lists: {}\ndefault_list: p":                                                                          "lists must not be empty",
		"lists: {p: Personal}\ndefault_list: x":                                                               "not in lists",
		"lists: {p: Personal}\ndefault_list: p\ntags: {a: {emoji: x, key: 'a b'}}":                            "without spaces",
		"lists: {p: Personal}\ndefault_list: p\ntags: {a: {emoji: x, key: pr}, b: {emoji: y, key: PR}}":       "share key",
		"lists: {p: Personal}\ndefault_list: p\ntags: {a: {emoji: x, key: pr}, b: {emoji: y, aliases: [pr]}}": "share key",
		"lists: {p: Personal}\ndefault_list: p\nrules: [{match: '(', tag: a}]":                                "unknown tag",
		"lists: {p: Personal}\ndefault_list: p\ntags: {a: {emoji: x, key: a}}\nrules: [{match: '(', tag: a}]": "missing closing",
		"lists: {p: Personal}\ndefault_list: p\nsources: {x.exe: {read: clipboard}}":                          "read must be",
		"lists: {p: Personal}\ndefault_list: p\nsources: {x.exe: {read: control}}":                            "needs control",
		"lists: {p: Personal}\ndefault_list: p\ntasks: {lists: [w]}":                                          "unknown list",
		"lists: {p: Personal}\ndefault_list: p\ntasks: {refresh: 10s}":                                        "below the minimum",
	}
	for in, want := range cases {
		_, err := Parse([]byte(in))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want containing %q", in, err, want)
		}
	}
}

func TestTaskListsSubset(t *testing.T) {
	c, err := Parse([]byte("lists: {p: Personal, w: Work}\ndefault_list: p\ntasks: {lists: [w], refresh: 2m}"))
	if err != nil {
		t.Fatal(err)
	}
	if l := c.TaskLists(); len(l) != 1 || l["w"] != "Work" || c.Tasks.Refresh != 2*time.Minute {
		t.Errorf("task lists = %v, refresh = %s", l, c.Tasks.Refresh)
	}
}
