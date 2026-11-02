package inbox

import "testing"

// Typed numbers are offered as numbers to text; anything else isn't.
func TestPhoneNumber(t *testing.T) {
	for query, want := range map[string]string{
		"021 238 5314":      "0212385314",
		"+64 (21) 238-5314": "+64212385314",
		"021.238.5314":      "0212385314",
		"12":                "",
		"Joshua":            "",
		"021 23x":           "",
		"64+21":             "",
	} {
		if got := phoneNumber(query); got != want {
			t.Errorf("phoneNumber(%q) = %q, want %q", query, got, want)
		}
	}
}

// Contacts match by name, or by number ignoring formatting.
func TestContactMatches(t *testing.T) {
	for _, c := range []struct {
		name, phone, query string
		want               bool
	}{
		{"Joshua Smith", "+64212385314", "josh", true},
		{"Joshua Smith", "+64212385314", "238 53", true},
		{"Joshua Smith", "+64212385314", "21", false},
		{"Joshua Smith", "+64212385314", "999", false},
		{"Ana", "", "ana", true},
	} {
		if got := contactMatches(c.name, c.phone, c.query); got != c.want {
			t.Errorf("contactMatches(%q, %q, %q) = %v", c.name, c.phone, c.query, got)
		}
	}
}
