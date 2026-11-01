package tailscale

import "testing"

func TestNormalizeVersion(t *testing.T) {
	cases := map[string]string{
		"1.80.3":                      "1.80.3",
		"1.80.3-t2a3b4c5d6-g7e8f9a0b": "1.80.3",
		"1:1.104.4-1":                 "1.104.4",
		"tailscale 1.2.10":            "1.2.10",
		"garbage":                     "",
		"":                            "",
	}
	for in, want := range cases {
		if got := normalizeVersion(in); got != want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.104.4", "1.80.3", 1},
		{"1.80.3", "1.104.4", -1},
		{"1.80.3", "1.80.3", 0},
		{"1.80.10", "1.80.9", 1},
		{"2.0.0", "1.999.999", 1},
		{"1.80.3-tabc", "1.80.3", 0},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestLastLines(t *testing.T) {
	if got := lastLines("a\nb\nc\nd", 2); got != "c | d" {
		t.Errorf("lastLines = %q", got)
	}
}

func TestExtractAuthURL(t *testing.T) {
	out := "To authenticate, visit:\n\n\thttps://login.tailscale.com/a/abc123\n"
	if got := extractAuthURL(out); got != "https://login.tailscale.com/a/abc123" {
		t.Errorf("extractAuthURL = %q", got)
	}
}
