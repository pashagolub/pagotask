package windows

import "testing"

func TestAsURL(t *testing.T) {
	cases := map[string]string{
		"github.com/pashagolub/pagotask/pull/6":         "https://github.com/pashagolub/pagotask/pull/6",
		"https://github.com/pashagolub/pagotask/pull/6": "https://github.com/pashagolub/pagotask/pull/6",
		"  about:blank ":           "",
		"Search or jump to...":     "",
		"":                         "",
		"LGTM, thanks":             "",
		"v1.2":                     "",
		"en.wikipedia.org/wiki/Go": "https://en.wikipedia.org/wiki/Go",
		"pgwatch":                  "",
	}
	for in, want := range cases {
		got, ok := asURL(in)
		if got != want || ok != (want != "") {
			t.Errorf("asURL(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}
