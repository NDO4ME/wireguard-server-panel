package config

import (
	"fmt"
	"net/netip"
)

var tailscaleCGNAT = netip.MustParsePrefix("100.64.0.0/10")

// CheckSubnet validates the VPN subnet. It returns a fatal error for values the
// panel cannot work with and a list of non-fatal warnings for unusual ones.
// The recommended form is a private /24 such as 10.8.0.0/24 or 10.8.1.0/24.
func CheckSubnet(s string) (warnings []string, err error) {
	p, perr := netip.ParsePrefix(s)
	if perr != nil || !p.Addr().Is4() {
		return nil, fmt.Errorf("WG_NETWORK %q is not a valid IPv4 network (example: 10.8.1.0/24)", s)
	}
	if p.Masked().Addr() != p.Addr() {
		return nil, fmt.Errorf("WG_NETWORK %q has host bits set; use the network address (did you mean %s?)", s, p.Masked())
	}
	if p.Bits() > 30 {
		return nil, fmt.Errorf("WG_NETWORK %q is too small", s)
	}
	if p.Overlaps(tailscaleCGNAT) {
		return nil, fmt.Errorf("WG_NETWORK %q overlaps the Tailscale range %s; choose another subnet", s, tailscaleCGNAT)
	}
	if p.Bits() != 24 {
		warnings = append(warnings, fmt.Sprintf("WG_NETWORK %s is not a /24; client addresses are only handed out from the first /24 (253 clients)", s))
	}
	if !p.Addr().IsPrivate() {
		warnings = append(warnings, fmt.Sprintf("WG_NETWORK %s is not a private range (10/8, 172.16/12, 192.168/16)", s))
	}
	return warnings, nil
}
