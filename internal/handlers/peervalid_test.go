package handlers

import (
	"strings"
	"testing"
	"time"
)

func TestCleanText(t *testing.T) {
	if v, err := cleanText("  Phone  ", "name", 64, true); err != nil || v != "Phone" {
		t.Fatal(v, err)
	}
	for _, in := range []string{"", "   ", "a\nb", "a\x00b", strings.Repeat("x", 65)} {
		if _, err := cleanText(in, "name", 64, true); err == nil {
			t.Errorf("%q should fail", in)
		}
	}
	if _, err := cleanText("", "note", 200, false); err != nil {
		t.Error("empty optional must pass")
	}
}

func TestParseExpiry(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	if got, err := parseExpiry("2026-11-06", now); err != nil || got.Hour() != 23 {
		t.Fatal(got, err)
	}
	if _, err := parseExpiry("2026-11-05", now); err == nil {
		t.Error("past date must fail")
	}
	if _, err := parseExpiry("2026-12-01T10:00:00+02:00", now); err != nil {
		t.Error(err)
	}
	if _, err := parseExpiry("tomorrow", now); err == nil {
		t.Error("garbage must fail")
	}
}

func TestConfigFilename(t *testing.T) {
	cases := map[string]string{"Phone": "Phone.conf", "my phone\r\nX: y": "my_phone_X_y.conf", `a"b/../c`: "a_b_.._c.conf", "***": "wireguard.conf", "Ünï": "n.conf"}
	for in, want := range cases {
		if got := configFilename(in); got != want {
			t.Errorf("%q => %q want %q", in, got, want)
		}
	}
}
