package tailscale

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Status represents Tailscale connection status
type Status struct {
	Connected    bool        `json:"connected"`
	BackendState string      `json:"backend_state"`
	AuthURL      string      `json:"auth_url,omitempty"`
	Self         *PeerInfo   `json:"self,omitempty"`
	Peers        []PeerInfo  `json:"peers,omitempty"`
	Routes       []RouteInfo `json:"routes,omitempty"`
}

// PeerInfo represents a Tailscale peer
type PeerInfo struct {
	Name          string   `json:"name"`
	HostName      string   `json:"hostname"`
	TailscaleIP   string   `json:"tailscale_ip"`
	AllowedIPs    []string `json:"allowed_ips"`
	PrimaryRoutes []string `json:"primary_routes,omitempty"`
	Online        bool     `json:"online"`
}

// RouteInfo represents a route advertised by a peer
type RouteInfo struct {
	Subnet   string `json:"subnet"`
	PeerName string `json:"peer_name"`
}

// TailscaleStatus raw JSON structure
type tailscaleStatusJSON struct {
	BackendState string                       `json:"BackendState"`
	Self         *tailscaleSelfJSON           `json:"Self"`
	Peer         map[string]tailscalePeerJSON `json:"Peer"`
}

type tailscaleSelfJSON struct {
	HostName   string   `json:"HostName"`
	TailscaleIPs []string `json:"TailscaleIPs"`
	Online     bool     `json:"Online"`
}

type tailscalePeerJSON struct {
	HostName      string   `json:"HostName"`
	TailscaleIPs  []string `json:"TailscaleIPs"`
	AllowedIPs    []string `json:"AllowedIPs"`
	PrimaryRoutes []string `json:"PrimaryRoutes"`
	Online        bool     `json:"Online"`
}

// Manager handles Tailscale operations
type Manager struct{}

// NewManager creates a new Tailscale manager
func NewManager() *Manager {
	return &Manager{}
}

// IsInstalled checks if Tailscale is installed
func (m *Manager) IsInstalled() bool {
	_, err := exec.LookPath("tailscale")
	return err == nil
}

// GetStatus returns current Tailscale status
func (m *Manager) GetStatus() (*Status, error) {
	if !m.IsInstalled() {
		return &Status{
			Connected:    false,
			BackendState: "not_installed",
		}, nil
	}

	cmd := exec.Command("tailscale", "status", "--json")
	output, err := cmd.Output()
	if err != nil {
		// Check if it's because tailscaled is not running
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			if strings.Contains(stderr, "not running") {
				return &Status{
					Connected:    false,
					BackendState: "stopped",
				}, nil
			}
		}
		return nil, fmt.Errorf("failed to get tailscale status: %w", err)
	}

	var rawStatus tailscaleStatusJSON
	if err := json.Unmarshal(output, &rawStatus); err != nil {
		return nil, fmt.Errorf("failed to parse tailscale status: %w", err)
	}

	status := &Status{
		BackendState: rawStatus.BackendState,
		Connected:    rawStatus.BackendState == "Running",
		Peers:        make([]PeerInfo, 0),
		Routes:       make([]RouteInfo, 0),
	}

	// Parse self info
	if rawStatus.Self != nil {
		tailscaleIP := ""
		if len(rawStatus.Self.TailscaleIPs) > 0 {
			tailscaleIP = rawStatus.Self.TailscaleIPs[0]
		}
		status.Self = &PeerInfo{
			Name:        rawStatus.Self.HostName,
			HostName:    rawStatus.Self.HostName,
			TailscaleIP: tailscaleIP,
			Online:      rawStatus.Self.Online,
		}
	}

	// Parse peers
	for _, peer := range rawStatus.Peer {
		tailscaleIP := ""
		if len(peer.TailscaleIPs) > 0 {
			tailscaleIP = peer.TailscaleIPs[0]
		}

		peerInfo := PeerInfo{
			Name:          peer.HostName,
			HostName:      peer.HostName,
			TailscaleIP:   tailscaleIP,
			AllowedIPs:    peer.AllowedIPs,
			PrimaryRoutes: peer.PrimaryRoutes,
			Online:        peer.Online,
		}
		status.Peers = append(status.Peers, peerInfo)

		// Extract routes
		for _, route := range peer.PrimaryRoutes {
			status.Routes = append(status.Routes, RouteInfo{
				Subnet:   route,
				PeerName: peer.HostName,
			})
		}
	}

	return status, nil
}

// Up starts Tailscale and returns auth URL if needed
func (m *Manager) Up() (*Status, error) {
	if !m.IsInstalled() {
		return nil, fmt.Errorf("tailscale is not installed")
	}

	// First check current status
	currentStatus, _ := m.GetStatus()
	if currentStatus != nil && currentStatus.Connected {
		return currentStatus, nil
	}

	// Run tailscale up with --accept-routes to accept advertised routes
	// Use timeout context to not wait forever
	cmd := exec.Command("timeout", "5", "tailscale", "up", "--accept-routes", "--reset")
	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Check for auth URL in output
	authURL := extractAuthURL(outputStr)

	// If no auth URL in output, check the control log
	if authURL == "" {
		// Try to get auth URL from tailscale status
		statusCmd := exec.Command("tailscale", "status", "--json")
		statusOutput, _ := statusCmd.Output()

		var rawStatus map[string]interface{}
		if json.Unmarshal(statusOutput, &rawStatus) == nil {
			if state, ok := rawStatus["BackendState"].(string); ok && state == "NeedsLogin" {
				// Get auth URL using debug command
				debugCmd := exec.Command("tailscale", "debug", "prefs")
				debugOutput, _ := debugCmd.Output()
				authURL = extractAuthURL(string(debugOutput))
			}
		}
	}

	// If still no auth URL, try to read from control server logs
	if authURL == "" {
		// Check journal/logs for AuthURL
		journalCmd := exec.Command("sh", "-c", "journalctl -u tailscaled -n 50 --no-pager 2>/dev/null | grep -o 'https://login.tailscale.com/[^ ]*' | tail -1")
		journalOutput, _ := journalCmd.Output()
		if url := strings.TrimSpace(string(journalOutput)); url != "" {
			authURL = url
		}
	}

	// If still no auth URL, try docker logs approach (for containerized environment)
	if authURL == "" {
		// Read from tailscaled stdout/stderr which goes to docker logs
		grepCmd := exec.Command("sh", "-c", "grep -r 'AuthURL' /var/log/ 2>/dev/null | grep -o 'https://[^ ]*' | tail -1")
		grepOutput, _ := grepCmd.Output()
		if url := strings.TrimSpace(string(grepOutput)); url != "" {
			authURL = url
		}
	}

	// Get current status
	status, statusErr := m.GetStatus()
	if statusErr != nil {
		status = &Status{
			Connected:    false,
			BackendState: "NeedsLogin",
		}
	}

	// If status shows NeedsLogin but we don't have auth URL, get it from tailscale
	if status.BackendState == "NeedsLogin" && authURL == "" {
		// Last resort: run tailscale up again and capture stderr
		upCmd := exec.Command("tailscale", "up", "--accept-routes")
		upOutput, _ := upCmd.CombinedOutput()
		authURL = extractAuthURL(string(upOutput))
	}

	status.AuthURL = authURL

	return status, nil
}

// extractAuthURL extracts auth URL from text
func extractAuthURL(text string) string {
	if !strings.Contains(text, "https://") {
		return ""
	}

	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if strings.Contains(line, "https://login.tailscale.com") || strings.Contains(line, "https://") {
			start := strings.Index(line, "https://")
			if start != -1 {
				rest := line[start:]
				end := strings.IndexAny(rest, " \n\t\r\"'")
				if end == -1 {
					return strings.TrimSpace(rest)
				}
				return strings.TrimSpace(rest[:end])
			}
		}
	}
	return ""
}

// Down stops Tailscale
func (m *Manager) Down() error {
	if !m.IsInstalled() {
		return fmt.Errorf("tailscale is not installed")
	}

	cmd := exec.Command("tailscale", "down")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop tailscale: %s", string(output))
	}

	return nil
}

// GetRoutingCommands returns iptables commands needed to route WireGuard traffic to Tailscale
func (m *Manager) GetRoutingCommands(wgNetwork string) ([]string, error) {
	status, err := m.GetStatus()
	if err != nil {
		return nil, err
	}

	if !status.Connected {
		return nil, fmt.Errorf("tailscale is not connected")
	}

	commands := make([]string, 0)

	// Enable forwarding from WireGuard to Tailscale interface
	commands = append(commands,
		fmt.Sprintf("iptables -I FORWARD -s %s -o tailscale0 -j ACCEPT", wgNetwork),
		fmt.Sprintf("iptables -I FORWARD -i tailscale0 -d %s -m state --state RELATED,ESTABLISHED -j ACCEPT", wgNetwork),
	)

	// Add MASQUERADE for each Tailscale route
	for _, route := range status.Routes {
		commands = append(commands,
			fmt.Sprintf("iptables -t nat -A POSTROUTING -s %s -d %s -o tailscale0 -j MASQUERADE", wgNetwork, route.Subnet),
		)
	}

	// Add MASQUERADE for Tailscale IPs (100.x.x.x range)
	commands = append(commands,
		fmt.Sprintf("iptables -t nat -A POSTROUTING -s %s -d 100.64.0.0/10 -o tailscale0 -j MASQUERADE", wgNetwork),
	)

	return commands, nil
}

// SetupRouting configures iptables for WireGuard to Tailscale routing
func (m *Manager) SetupRouting(wgNetwork string) error {
	commands, err := m.GetRoutingCommands(wgNetwork)
	if err != nil {
		return err
	}

	for _, cmdStr := range commands {
		parts := strings.Fields(cmdStr)
		if len(parts) < 2 {
			continue
		}

		cmd := exec.Command(parts[0], parts[1:]...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Ignore "already exists" errors
			if !strings.Contains(string(output), "already exists") {
				return fmt.Errorf("failed to execute %s: %s", cmdStr, string(output))
			}
		}
	}

	return nil
}

// ClearRouting removes Tailscale routing rules
func (m *Manager) ClearRouting(wgNetwork string) error {
	// Remove forwarding rules
	exec.Command("iptables", "-D", "FORWARD", "-s", wgNetwork, "-o", "tailscale0", "-j", "ACCEPT").Run()
	exec.Command("iptables", "-D", "FORWARD", "-i", "tailscale0", "-d", wgNetwork, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT").Run()

	// Remove NAT rules - we need to find and delete them
	// This is a simplified version; production would need more robust cleanup
	exec.Command("sh", "-c", fmt.Sprintf("iptables -t nat -S POSTROUTING | grep '%s.*tailscale0' | sed 's/-A/-D/' | xargs -I {} sh -c 'iptables -t nat {}'", wgNetwork)).Run()

	return nil
}
