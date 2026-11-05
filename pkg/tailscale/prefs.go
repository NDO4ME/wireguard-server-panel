package tailscale

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os/exec"
	"strings"
	"time"
)

// Prefs are the runtime preferences the panel exposes. They are applied with
// `tailscale set`, which updates the running daemon without a reconnect.
type Prefs struct {
	AdvertiseExitNode bool     `json:"advertise_exit_node"`
	AdvertiseRoutes   []string `json:"advertise_routes"`
	ShieldsUp         bool     `json:"shields_up"`
	AcceptRoutes      bool     `json:"accept_routes"`
}

// ValidateRoutes normalises a list of CIDRs (masked, deduplicated, max 32).
// Exit-node default routes are managed by AdvertiseExitNode, so they are rejected here.
func ValidateRoutes(in []string) ([]string, error) {
	if len(in) > 32 {
		return nil, fmt.Errorf("too many routes (max 32)")
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("invalid route %q (use CIDR, e.g. 192.168.1.0/24)", s)
		}
		p = p.Masked()
		if p.Bits() == 0 {
			return nil, fmt.Errorf("route %q is a default route; use the exit node option instead", s)
		}
		k := p.String()
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out, nil
}

type prefsJSON struct {
	AdvertiseRoutes []string
	ShieldsUp       bool
	RouteAll        bool
}

// GetPrefs reads the current preferences from the daemon.
func (m *Manager) GetPrefs() (*Prefs, error) {
	if !m.IsInstalled() {
		return nil, fmt.Errorf("tailscale is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tailscale", "debug", "prefs").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read tailscale preferences: %w", err)
	}
	var raw prefsJSON
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("unexpected tailscale preferences output: %w", err)
	}
	p := &Prefs{ShieldsUp: raw.ShieldsUp, AcceptRoutes: raw.RouteAll, AdvertiseRoutes: []string{}}
	for _, r := range raw.AdvertiseRoutes {
		if r == "0.0.0.0/0" || r == "::/0" {
			p.AdvertiseExitNode = true
			continue
		}
		p.AdvertiseRoutes = append(p.AdvertiseRoutes, r)
	}
	return p, nil
}

// buildSetArgs returns the argument list for `tailscale set`.
func buildSetArgs(p Prefs) []string {
	return []string{
		"set",
		fmt.Sprintf("--advertise-exit-node=%t", p.AdvertiseExitNode),
		"--advertise-routes=" + strings.Join(p.AdvertiseRoutes, ","),
		fmt.Sprintf("--shields-up=%t", p.ShieldsUp),
		fmt.Sprintf("--accept-routes=%t", p.AcceptRoutes),
	}
}

// SetPrefs validates and applies preferences to the running daemon.
func (m *Manager) SetPrefs(p Prefs) (*Prefs, error) {
	if !m.IsInstalled() {
		return nil, fmt.Errorf("tailscale is not installed")
	}
	routes, err := ValidateRoutes(p.AdvertiseRoutes)
	if err != nil {
		return nil, err
	}
	p.AdvertiseRoutes = routes
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "tailscale", buildSetArgs(p)...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("tailscale set failed: %s", strings.TrimSpace(string(out)))
	}
	return m.GetPrefs()
}
