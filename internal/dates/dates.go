// Package dates parses the short due-date keywords typed in the popup.
package dates

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

var weekdays = map[string]time.Weekday{
	"mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday, "thu": time.Thursday,
	"fri": time.Friday, "sat": time.Saturday, "sun": time.Sunday,
}

// Parse turns a keyword into a date relative to now. Accepted forms:
// "" or "tod" (today), "tom" (tomorrow), "mon".."sun" (next such weekday,
// today if it is that weekday), "+N" (N days ahead), "YYYY-MM-DD", "DD.MM".
// The result is midnight local time.
func Parse(s string, now time.Time) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch {
	case s == "" || s == "tod" || s == "today":
		return today, nil
	case s == "tom" || s == "tomorrow":
		return today.AddDate(0, 0, 1), nil
	case strings.HasPrefix(s, "+"):
		n, err := strconv.Atoi(s[1:])
		if err != nil || n < 0 {
			return time.Time{}, fmt.Errorf("bad offset %q", s)
		}
		return today.AddDate(0, 0, n), nil
	}
	if wd, ok := weekdays[s]; ok {
		delta := (int(wd) - int(today.Weekday()) + 7) % 7
		return today.AddDate(0, 0, delta), nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, now.Location()); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2.1", s, now.Location()); err == nil {
		t = time.Date(now.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location())
		if t.Before(today) {
			t = t.AddDate(1, 0, 0)
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unknown date %q", s)
}

// RFC3339 formats a date the way the Tasks API expects the due field.
func RFC3339(t time.Time) string {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
}
