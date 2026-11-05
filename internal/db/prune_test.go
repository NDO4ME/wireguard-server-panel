package db

import (
	"path/filepath"
	"testing"

	"github.com/ndo4me/wireguard-server-panel/internal/models"
)

func TestPruneConnectionLogs(t *testing.T) {
	d, err := Initialize(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := d.CreatePeer(&models.Peer{Name: "a", PublicKey: "k", PrivateKey: "p", AssignedIP: "10.8.0.2", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if _, err := d.conn.Exec("INSERT INTO connection_logs (peer_id, endpoint, connected_at) VALUES (?, ?, datetime('now', ?))",
			p.ID, "1.1.1.1:1", "-"+string(rune('0'+i))+" hours"); err != nil {
			t.Fatal(err)
		}
	}
	// one entry far older than the retention window
	if _, err := d.conn.Exec("INSERT INTO connection_logs (peer_id, endpoint, connected_at) VALUES (?, 'old', datetime('now', '-400 days'))", p.ID); err != nil {
		t.Fatal(err)
	}
	if err := d.PruneConnectionLogs(5, 180); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM connection_logs").Scan(&n); err != nil || n != 5 {
		t.Fatalf("want 5 rows, got %d (%v)", n, err)
	}
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM connection_logs WHERE endpoint='old'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("old row kept: %d", n)
	}
}
