package handlers

import (
	"context"
	"log"
	"time"

	"github.com/ndo4me/wireguard-server-panel/internal/db"
	"github.com/ndo4me/wireguard-server-panel/pkg/wgmanager"
)

// EnforceExpiry disables every enabled client whose expiry has passed. The peer
// is removed from the live interface (other clients are not touched) and kept in
// the database, so it can be re-enabled later with a new expiry.
func EnforceExpiry(wg *wgmanager.WGManager) int {
	expired, err := db.DB.GetExpiredEnabledPeers(time.Now())
	if err != nil {
		log.Printf("expiry: lookup failed: %v", err)
		return 0
	}
	n := 0
	for _, p := range expired {
		if err := wg.RemovePeer(p.PublicKey); err != nil {
			log.Printf("expiry: could not remove %s from WireGuard: %v", p.AssignedIP, err)
			continue // retry on the next tick, never mark disabled while still active
		}
		off := false
		if _, err := db.DB.UpdatePeerFields(p.AssignedIP, db.PeerUpdate{Enabled: &off}); err != nil {
			log.Printf("expiry: could not update %s: %v", p.AssignedIP, err)
			continue
		}
		log.Printf("expiry: client %q (%s) expired and was disabled", p.Name, p.AssignedIP)
		n++
	}
	return n
}

// RunExpiryEnforcer checks once at start and then every minute until ctx ends.
func RunExpiryEnforcer(ctx context.Context, wg *wgmanager.WGManager) {
	EnforceExpiry(wg)
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			EnforceExpiry(wg)
		}
	}
}
