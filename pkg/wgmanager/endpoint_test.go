package wgmanager

import "testing"

func TestNormalizeEndpointHost(t *testing.T) {
	ok := map[string]string{"": "", " vpn.Example.com ": "vpn.example.com", "vpn.example.com.": "vpn.example.com", "1.2.3.4": "1.2.3.4", "[2001:db8::1]": "2001:db8::1", "localhost": "localhost", "a-b.c1.io": "a-b.c1.io"}
	for in, want := range ok {
		if got, err := NormalizeEndpointHost(in); err != nil || got != want {
			t.Errorf("%q => %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"vpn.example.com:1881", "https://vpn.example.com", "vpn example.com", "-bad.com", "bad-.com", "a..b", "vpn.example.com/x", "user@host"} {
		if _, err := NormalizeEndpointHost(in); err == nil {
			t.Errorf("%q should be rejected", in)
		}
	}
}

func TestClientEndpoint(t *testing.T) {
	cases := [][3]string{
		{"203.0.113.5:1881", "", "203.0.113.5:1881"},
		{"203.0.113.5:1881", "vpn.example.com", "vpn.example.com:1881"},
		{"203.0.113.5:1881", "2001:db8::1", "[2001:db8::1]:1881"},
		{"[2001:db8::9]:51820", "vpn.example.com", "vpn.example.com:51820"},
	}
	for _, c := range cases {
		if got := ClientEndpoint(c[0], c[1]); got != c[2] {
			t.Errorf("%v => %q", c, got)
		}
	}
}
