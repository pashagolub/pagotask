package dates

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	now := time.Date(2026, 9, 27, 15, 4, 0, 0, time.UTC) // a Sunday
	cases := map[string]string{
		"":           "2026-09-27",
		"tod":        "2026-09-27",
		"tom":        "2026-09-28",
		"+3":         "2026-09-30",
		"sun":        "2026-09-27",
		"mon":        "2026-09-28",
		"sat":        "2026-10-03",
		"2026-12-24": "2026-12-24",
		"24.12":      "2026-12-24",
		"1.1":        "2027-01-01",
	}
	for in, want := range cases {
		got, err := Parse(in, now)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if got.Format("2006-01-02") != want {
			t.Errorf("%q = %s, want %s", in, got.Format("2006-01-02"), want)
		}
	}
	for _, bad := range []string{"+x", "someday", "-1"} {
		if _, err := Parse(bad, now); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
	if got := RFC3339(now); got != "2026-09-27T00:00:00Z" {
		t.Errorf("RFC3339 = %s", got)
	}
}

func TestRFC3339KeepsLocalDay(t *testing.T) {
	kyiv := time.FixedZone("UTC+3", 3*3600)
	// Local midnight is still the previous day in UTC; the due date must not be.
	d := time.Date(2026, 9, 27, 0, 0, 0, 0, kyiv)
	if got := RFC3339(d); got != "2026-09-27T00:00:00Z" {
		t.Errorf("RFC3339 = %q, want 2026-09-27T00:00:00Z", got)
	}
}
