package wgmanager

import (
	"strings"
	"testing"

	"github.com/ndo4me/wireguard-server-panel/internal/config"
	"github.com/ndo4me/wireguard-server-panel/internal/models"
)

func TestGenerateClientConfigUsesEndpointHost(t *testing.T) {
	cfg := &config.WireGuardConfig{ServerEndpoint: "203.0.113.5:1881", ServerPublicKey: "PUB", AllowedIPs: "0.0.0.0/0"}
	wg := &WGManager{config: cfg}
	peer := &models.Peer{PrivateKey: "PRIV", AssignedIP: "10.8.0.2"}

	out, err := wg.GenerateClientConfig(peer)
	if err != nil || !strings.Contains(out, "Endpoint = 203.0.113.5:1881\n") {
		t.Fatalf("default endpoint wrong: %v\n%s", err, out)
	}
	cfg.EndpointHost = "vpn.example.com"
	out, _ = wg.GenerateClientConfig(peer)
	if !strings.Contains(out, "Endpoint = vpn.example.com:1881\n") || strings.Contains(out, "203.0.113.5") {
		t.Fatalf("hostname not used:\n%s", out)
	}
	if !strings.Contains(out, "PrivateKey = PRIV") || !strings.Contains(out, "PublicKey = PUB") {
		t.Fatalf("keys changed:\n%s", out)
	}
}
