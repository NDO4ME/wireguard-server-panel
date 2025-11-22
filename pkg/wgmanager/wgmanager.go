package wgmanager

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"golang.org/x/crypto/curve25519"
	"wgeasygo/internal/config"
	"wgeasygo/internal/models"
)

// Pre-compiled regex for IP validation (avoid compilation on every call)
var ipRegex = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}$`)

// Buffer pool to reduce allocations
var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

func getBuffer() *bytes.Buffer {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

func putBuffer(buf *bytes.Buffer) {
	bufferPool.Put(buf)
}

// Pre-parsed template (avoid parsing on every call)
var clientConfigTmpl = template.Must(template.New("config").Parse(clientConfigTemplate))

// PeerStats contains real-time statistics for a peer
type PeerStats struct {
	PublicKey         string    `json:"public_key"`
	Endpoint          string    `json:"endpoint"`
	LatestHandshake   time.Time `json:"latest_handshake"`
	TransferRx        int64     `json:"transfer_rx"`
	TransferTx        int64     `json:"transfer_tx"`
	IsOnline          bool      `json:"is_online"`
}

// WGManager handles WireGuard operations
type WGManager struct {
	config *config.WireGuardConfig
}

// New creates a new WGManager instance
func New(cfg *config.WireGuardConfig) *WGManager {
	return &WGManager{config: cfg}
}

// GenerateKeyPair generates a WireGuard private/public key pair using Go's crypto
// This is preferred over calling external commands for security and portability
func GenerateKeyPair() (privateKey, publicKey string, err error) {
	// Generate 32 random bytes for private key
	var privateKeyBytes [32]byte
	if _, err := rand.Read(privateKeyBytes[:]); err != nil {
		return "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Clamp the private key per WireGuard spec (Curve25519)
	privateKeyBytes[0] &= 248
	privateKeyBytes[31] &= 127
	privateKeyBytes[31] |= 64

	// Derive public key
	var publicKeyBytes [32]byte
	curve25519.ScalarBaseMult(&publicKeyBytes, &privateKeyBytes)

	privateKey = base64.StdEncoding.EncodeToString(privateKeyBytes[:])
	publicKey = base64.StdEncoding.EncodeToString(publicKeyBytes[:])

	return privateKey, publicKey, nil
}

// ValidatePublicKey checks if a string is a valid WireGuard public key
func ValidatePublicKey(key string) bool {
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return false
	}
	return len(decoded) == 32
}

// ValidateIP checks if a string is a valid IPv4 address
func ValidateIP(ip string) bool {
	if !ipRegex.MatchString(ip) {
		return false
	}

	// Validate each octet
	parts := strings.Split(ip, ".")
	for _, part := range parts {
		num, err := strconv.Atoi(part)
		if err != nil || num < 0 || num > 255 {
			return false
		}
	}
	return true
}

// AddPeer adds a peer to the WireGuard interface
// Uses exec.Command with separate arguments to prevent shell injection
func (wg *WGManager) AddPeer(publicKey, assignedIP string) error {
	if !ValidatePublicKey(publicKey) {
		return fmt.Errorf("invalid public key format")
	}
	if !ValidateIP(assignedIP) {
		return fmt.Errorf("invalid IP address format")
	}

	// Add peer to WireGuard interface
	// SECURITY: Arguments are passed separately, not concatenated into a shell command
	cmd := exec.Command("wg", "set", wg.config.Interface,
		"peer", publicKey,
		"allowed-ips", assignedIP+"/32",
	)

	stderr := getBuffer()
	defer putBuffer(stderr)
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add peer: %s: %w", stderr.String(), err)
	}

	// Save the configuration
	if err := wg.SaveConfig(); err != nil {
		return fmt.Errorf("failed to save config after adding peer: %w", err)
	}

	return nil
}

// RemovePeer removes a peer from the WireGuard interface by public key
func (wg *WGManager) RemovePeer(publicKey string) error {
	if !ValidatePublicKey(publicKey) {
		return fmt.Errorf("invalid public key format")
	}

	// Remove peer from WireGuard interface
	cmd := exec.Command("wg", "set", wg.config.Interface,
		"peer", publicKey,
		"remove",
	)

	stderr := getBuffer()
	defer putBuffer(stderr)
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove peer: %s: %w", stderr.String(), err)
	}

	// Save the configuration
	if err := wg.SaveConfig(); err != nil {
		return fmt.Errorf("failed to save config after removing peer: %w", err)
	}

	return nil
}

// SaveConfig saves the current WireGuard configuration to disk
func (wg *WGManager) SaveConfig() error {
	cmd := exec.Command("wg-quick", "save", wg.config.Interface)

	stderr := getBuffer()
	defer putBuffer(stderr)
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to save configuration: %s: %w", stderr.String(), err)
	}

	return nil
}

// GetInterfaceStatus returns the current status of the WireGuard interface
func (wg *WGManager) GetInterfaceStatus() (string, error) {
	cmd := exec.Command("wg", "show", wg.config.Interface)

	stdout := getBuffer()
	defer putBuffer(stdout)
	stderr := getBuffer()
	defer putBuffer(stderr)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get interface status: %s: %w", stderr.String(), err)
	}

	return stdout.String(), nil
}

// GetPeerStats returns real-time statistics for all peers
func (wg *WGManager) GetPeerStats() (map[string]*PeerStats, error) {
	cmd := exec.Command("wg", "show", wg.config.Interface, "dump")

	stdout := getBuffer()
	defer putBuffer(stdout)
	stderr := getBuffer()
	defer putBuffer(stderr)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to get peer stats: %s: %w", stderr.String(), err)
	}

	output := stdout.String()
	stats := make(map[string]*PeerStats)
	lines := strings.Split(output, "\n")

	for i, line := range lines {
		if i == 0 || line == "" {
			continue // Skip header line and empty lines
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 8 {
			continue
		}

		publicKey := fields[0]
		endpoint := fields[2]

		// Parse latest handshake (unix timestamp)
		var latestHandshake time.Time
		if ts, err := strconv.ParseInt(fields[4], 10, 64); err == nil && ts > 0 {
			latestHandshake = time.Unix(ts, 0)
		}

		// Parse transfer stats
		transferRx, _ := strconv.ParseInt(fields[5], 10, 64)
		transferTx, _ := strconv.ParseInt(fields[6], 10, 64)

		// Consider online if handshake was within last 3 minutes
		// WireGuard clients have PersistentKeepalive = 25s, so handshake updates every 25s when connected
		// Using 3 minutes (180s) provides buffer for network delays and missed keepalives
		isOnline := !latestHandshake.IsZero() && time.Since(latestHandshake) < 180*time.Second

		stats[publicKey] = &PeerStats{
			PublicKey:       publicKey,
			Endpoint:        endpoint,
			LatestHandshake: latestHandshake,
			TransferRx:      transferRx,
			TransferTx:      transferTx,
			IsOnline:        isOnline,
		}
	}

	return stats, nil
}

// Client configuration template
const clientConfigTemplate = `[Interface]
PrivateKey = {{.PrivateKey}}
Address = {{.Address}}/32
DNS = {{.DNS}}

[Peer]
PublicKey = {{.ServerPublicKey}}
Endpoint = {{.ServerEndpoint}}
AllowedIPs = {{.AllowedIPs}}
PersistentKeepalive = 25
`

// ClientConfig holds the data for generating client configuration
type ClientConfig struct {
	PrivateKey      string
	Address         string
	DNS             string
	ServerPublicKey string
	ServerEndpoint  string
	AllowedIPs      string
}

// GenerateClientConfig creates a WireGuard client configuration file content
func (wg *WGManager) GenerateClientConfig(peer *models.Peer) (string, error) {
	config := ClientConfig{
		PrivateKey:      peer.PrivateKey,
		Address:         peer.AssignedIP,
		DNS:             wg.config.DNS,
		ServerPublicKey: wg.config.ServerPublicKey,
		ServerEndpoint:  wg.config.ServerEndpoint,
		AllowedIPs:      wg.config.AllowedIPs,
	}

	buf := getBuffer()
	defer putBuffer(buf)

	if err := clientConfigTmpl.Execute(buf, config); err != nil {
		return "", fmt.Errorf("failed to generate config: %w", err)
	}

	return buf.String(), nil
}

// SyncPeersToInterface syncs all peers from the database to the WireGuard interface
// This is useful after a restart or to ensure consistency
func (wg *WGManager) SyncPeersToInterface(peers []models.Peer) error {
	for _, peer := range peers {
		if peer.Enabled {
			if err := wg.AddPeer(peer.PublicKey, peer.AssignedIP); err != nil {
				// Log but continue with other peers
				fmt.Printf("Warning: failed to sync peer %s: %v\n", peer.Name, err)
			}
		}
	}
	return nil
}
