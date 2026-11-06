package config

import "testing"

func TestCheckSubnet(t *testing.T) {
	ok := []string{"10.8.0.0/24", "10.8.1.0/24", "192.168.77.0/24", "172.20.5.0/24"}
	for _, s := range ok {
		w, err := CheckSubnet(s)
		if err != nil || len(w) != 0 {
			t.Errorf("%s: err=%v warnings=%v", s, err, w)
		}
	}
	bad := []string{"", "nope", "10.8.1.5/24", "100.64.0.0/24", "100.100.0.0/16", "10.8.0.0/31", "fd00::/64"}
	for _, s := range bad {
		if _, err := CheckSubnet(s); err == nil {
			t.Errorf("%q should be rejected", s)
		}
	}
	if w, err := CheckSubnet("10.8.0.0/16"); err != nil || len(w) == 0 {
		t.Errorf("/16 should warn: %v %v", w, err)
	}
}
