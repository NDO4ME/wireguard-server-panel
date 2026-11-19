package handlers

import "testing"

func TestSettingsListValidation(t *testing.T) {
	for _, ok := range []string{"", "1.1.1.1", "1.1.1.1, 2606:4700::1111", "dns.example.com"} {
		if !validDNSList(ok) {
			t.Errorf("dns %q should be valid", ok)
		}
	}
	for _, bad := range []string{"1.1.1.1\nPostUp = id", "1.1.1.1;x", "a b", "-x.com"} {
		if validDNSList(bad) {
			t.Errorf("dns %q should be rejected", bad)
		}
	}
	if !validCIDRList("0.0.0.0/0, ::/0") || !validCIDRList("10.0.0.5") {
		t.Error("valid AllowedIPs rejected")
	}
	for _, bad := range []string{"", "0.0.0.0/0\nPostUp = id", "10.0.0.0/99"} {
		if validCIDRList(bad) {
			t.Errorf("allowed %q should be rejected", bad)
		}
	}
}
