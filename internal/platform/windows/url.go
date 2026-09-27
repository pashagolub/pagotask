package windows

import "strings"

// asURL reports whether an address-bar value looks like a web address and
// returns it with a scheme. Firefox and Chrome show http(s) pages without
// one ("github.com/owner/repo/pull/6").
func asURL(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" || strings.ContainsAny(v, " \t\n") {
		return "", false
	}
	if strings.Contains(v, "://") {
		return v, true
	}
	host, _, _ := strings.Cut(v, "/")
	dot := strings.LastIndexByte(host, '.')
	if dot < 0 || !isTLD(host[dot+1:]) {
		return "", false
	}
	return "https://" + v, true
}

// isTLD accepts a letters-only top-level domain, so "v1.2" or "3.14" in
// some text field is not taken for an address.
func isTLD(s string) bool {
	if len(s) < 2 {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}
