package handlers

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxPeerNameRunes = 64
	maxPeerNoteRunes = 200
)

// cleanText trims s and rejects control characters and over-long values.
func cleanText(s, field string, max int, required bool) (string, error) {
	s = strings.TrimSpace(s)
	if required && s == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	if utf8.RuneCountInString(s) > max {
		return "", fmt.Errorf("%s must be at most %d characters", field, max)
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%s contains invalid characters", field)
		}
	}
	return s, nil
}

// parseExpiry accepts RFC 3339 or YYYY-MM-DD (end of that day, UTC) and
// rejects moments in the past.
func parseExpiry(v string, now time.Time) (time.Time, error) {
	v = strings.TrimSpace(v)
	var t time.Time
	if d, err := time.Parse("2006-01-02", v); err == nil {
		t = d.Add(24*time.Hour - time.Second)
	} else if t, err = time.Parse(time.RFC3339, v); err != nil {
		return time.Time{}, fmt.Errorf("expires_at must be YYYY-MM-DD or an RFC 3339 timestamp")
	}
	if !t.After(now) {
		return time.Time{}, fmt.Errorf("expires_at must be in the future")
	}
	return t.UTC(), nil
}

var unsafeFilename = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// configFilename builds a safe download name (no quotes, separators or CR/LF).
func configFilename(name string) string {
	n := strings.Trim(unsafeFilename.ReplaceAllString(name, "_"), "._-")
	if n == "" {
		n = "wireguard"
	}
	if len(n) > 48 {
		n = n[:48]
	}
	return n + ".conf"
}
