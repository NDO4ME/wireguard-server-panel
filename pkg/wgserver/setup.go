package wgserver

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

// ServerConfig holds WireGuard server configuration
type ServerConfig struct {
	Interface   string
	Port        int
	PrivateKey  string
	PublicKey   string
	Address     string
	Network     string
	DNS         string
	PublicIP    string
	PostUp      string
	PostDown    string
}

// Setup initializes WireGuard server on first run
type Setup struct {
	config *ServerConfig
}

// NewSetup creates a new server setup instance
func NewSetup() *Setup {
	return &Setup{}
}

// IsConfigured checks if WireGuard server is already configured
func (s *Setup) IsConfigured() bool {
	_, err := os.Stat("/etc/wireguard/wg0.conf")
	return err == nil
}

// Configure sets up WireGuard server from scratch
func (s *Setup) Configure(port int, network string, dns string) error {
	// Generate server keys
	privateKey, err := s.generatePrivateKey()
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	publicKey, err := s.derivePublicKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to derive public key: %w", err)
	}

	// Get public IP
	publicIP, err := s.getPublicIP()
	if err != nil {
		return fmt.Errorf("failed to get public IP: %w", err)
	}

	// Get default network interface
	netInterface, err := s.getDefaultInterface()
	if err != nil {
		return fmt.Errorf("failed to get default interface: %w", err)
	}

	// Parse network to get server address
	serverAddr, err := s.getServerAddress(network)
	if err != nil {
		return fmt.Errorf("failed to parse network: %w", err)
	}

	s.config = &ServerConfig{
		Interface:  "wg0",
		Port:       port,
		PrivateKey: privateKey,
		PublicKey:  publicKey,
		Address:    serverAddr,
		Network:    network,
		DNS:        dns,
		PublicIP:   publicIP,
		PostUp:     fmt.Sprintf("iptables -t nat -A POSTROUTING -s %s -o %s -j MASQUERADE; iptables -A INPUT -p udp -m udp --dport %d -j ACCEPT; iptables -A FORWARD -i wg0 -j ACCEPT; iptables -A FORWARD -o wg0 -j ACCEPT", network, netInterface, port),
		PostDown:   fmt.Sprintf("iptables -t nat -D POSTROUTING -s %s -o %s -j MASQUERADE; iptables -D INPUT -p udp -m udp --dport %d -j ACCEPT; iptables -D FORWARD -i wg0 -j ACCEPT; iptables -D FORWARD -o wg0 -j ACCEPT", network, netInterface, port),
	}

	// Enable IP forwarding
	if err := s.enableIPForwarding(); err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %w", err)
	}

	// Create WireGuard directory
	if err := os.MkdirAll("/etc/wireguard", 0700); err != nil {
		return fmt.Errorf("failed to create wireguard directory: %w", err)
	}

	// Write server configuration
	if err := s.writeServerConfig(); err != nil {
		return fmt.Errorf("failed to write server config: %w", err)
	}

	// Start WireGuard interface
	if err := s.startInterface(); err != nil {
		return fmt.Errorf("failed to start interface: %w", err)
	}

	return nil
}

// GetConfig returns the current server configuration
func (s *Setup) GetConfig() *ServerConfig {
	return s.config
}

// LoadExistingConfig loads configuration from existing wg0.conf
func (s *Setup) LoadExistingConfig() error {
	// Read private key
	data, err := os.ReadFile("/etc/wireguard/wg0.conf")
	if err != nil {
		return err
	}

	config := &ServerConfig{
		Interface: "wg0",
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "PrivateKey") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				config.PrivateKey = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(line, "Address") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				config.Address = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(line, "ListenPort") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &config.Port)
			}
		}
	}

	if config.PrivateKey != "" {
		publicKey, err := s.derivePublicKey(config.PrivateKey)
		if err == nil {
			config.PublicKey = publicKey
		}
	}

	publicIP, _ := s.getPublicIP()
	config.PublicIP = publicIP

	s.config = config
	return nil
}

func (s *Setup) generatePrivateKey() (string, error) {
	cmd := exec.Command("wg", "genkey")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (s *Setup) derivePublicKey(privateKey string) (string, error) {
	cmd := exec.Command("wg", "pubkey")
	cmd.Stdin = strings.NewReader(privateKey)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (s *Setup) getPublicIP() (string, error) {
	// Try multiple services
	services := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
	}

	for _, service := range services {
		cmd := exec.Command("curl", "-s", "-4", "--max-time", "5", service)
		output, err := cmd.Output()
		if err == nil {
			ip := strings.TrimSpace(string(output))
			if net.ParseIP(ip) != nil {
				return ip, nil
			}
		}
	}

	// Fallback to hostname -I
	cmd := exec.Command("hostname", "-I")
	output, err := cmd.Output()
	if err == nil {
		ips := strings.Fields(string(output))
		if len(ips) > 0 {
			return ips[0], nil
		}
	}

	return "", fmt.Errorf("could not determine public IP")
}

func (s *Setup) getDefaultInterface() (string, error) {
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		return "eth0", nil // Default fallback
	}

	// Parse "default via X.X.X.X dev ethX"
	fields := strings.Fields(string(output))
	for i, field := range fields {
		if field == "dev" && i+1 < len(fields) {
			return fields[i+1], nil
		}
	}

	return "eth0", nil
}

func (s *Setup) getServerAddress(network string) (string, error) {
	_, ipnet, err := net.ParseCIDR(network)
	if err != nil {
		return "", err
	}

	// Get first usable IP (network + 1)
	ip := ipnet.IP.To4()
	if ip == nil {
		return "", fmt.Errorf("invalid IPv4 network")
	}

	ip[3]++
	mask, _ := ipnet.Mask.Size()
	return fmt.Sprintf("%s/%d", ip.String(), mask), nil
}

func (s *Setup) enableIPForwarding() error {
	// Enable IPv4 forwarding
	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0644); err != nil {
		// Try sysctl as fallback
		cmd := exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1")
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Setup) writeServerConfig() error {
	tmpl := `[Interface]
Address = {{ .Address }}
ListenPort = {{ .Port }}
PrivateKey = {{ .PrivateKey }}
PostUp = {{ .PostUp }}
PostDown = {{ .PostDown }}
`

	t, err := template.New("wg").Parse(tmpl)
	if err != nil {
		return err
	}

	f, err := os.OpenFile("/etc/wireguard/wg0.conf", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	return t.Execute(f, s.config)
}

func (s *Setup) startInterface() error {
	// Check if already running
	cmd := exec.Command("wg", "show", "wg0")
	if cmd.Run() == nil {
		// Already running, sync config instead
		cmd = exec.Command("wg", "syncconf", "wg0", "/etc/wireguard/wg0.conf")
		return cmd.Run()
	}

	// Bring up interface
	cmd = exec.Command("wg-quick", "up", "wg0")
	return cmd.Run()
}

// StopInterface stops the WireGuard interface
func (s *Setup) StopInterface() error {
	cmd := exec.Command("wg-quick", "down", "wg0")
	return cmd.Run()
}

// RestartInterface restarts the WireGuard interface
func (s *Setup) RestartInterface() error {
	s.StopInterface()
	return s.startInterface()
}
