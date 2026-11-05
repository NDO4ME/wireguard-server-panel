package handlers

import (
	"context"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ndo4me/wireguard-server-panel/internal/db"
	"github.com/ndo4me/wireguard-server-panel/internal/models"
	"github.com/ndo4me/wireguard-server-panel/pkg/tailscale"
	"github.com/ndo4me/wireguard-server-panel/pkg/wgmanager"
	"github.com/ndo4me/wireguard-server-panel/pkg/wgupdate"
)

// SystemHandler exposes versions, health and the WireGuard tools updater.
type SystemHandler struct {
	version string
	started time.Time
	wg      *wgmanager.WGManager
	ts      *tailscale.Manager
	updater *wgupdate.Updater
}

func NewSystemHandler(version string, wg *wgmanager.WGManager, ts *tailscale.Manager, up *wgupdate.Updater) *SystemHandler {
	return &SystemHandler{version: version, started: time.Now(), wg: wg, ts: ts, updater: up}
}

// GetStatus returns a read-only overview for the System page.
func (h *SystemHandler) GetStatus(c *gin.Context) {
	peers, _ := db.DB.GetAllPeers()
	enabled, online, expired := 0, 0, 0
	stats, _ := h.wg.GetPeerStats()
	for _, p := range peers {
		if p.Enabled {
			enabled++
		}
		if p.ExpiresAt != nil && !p.ExpiresAt.After(time.Now()) {
			expired++
		}
		if s, ok := stats[p.PublicKey]; ok && s.IsOnline {
			online++
		}
	}
	kernel := ""
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		kernel = strings.TrimSpace(string(b))
	}
	tsVersion, _ := h.ts.CurrentVersion()
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"panel_version":     h.version,
		"go_version":        runtime.Version(),
		"uptime_seconds":    int64(time.Since(h.started).Seconds()),
		"kernel":            kernel,
		"wireguard_tools":   wgupdate.ToolsVersion(),
		"tailscale_version": tsVersion,
		"interface":         h.wg.GetInterfaceInfo(),
		"clients": gin.H{
			"total": len(peers), "enabled": enabled, "online": online, "expired": expired,
		},
	})
}

func (h *SystemHandler) GetWireGuardUpdate(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, h.updater.Status())
}

func (h *SystemHandler) CheckWireGuardUpdate(c *gin.Context) {
	if st := h.updater.Status(); st.Checking || st.Updating {
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "A check or update is already running"})
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		if err := h.updater.Check(ctx); err != nil {
			log.Printf("wireguard: update check: %v", err)
		}
	}()
	c.JSON(http.StatusAccepted, gin.H{"success": true, "message": "Checking for updates"})
}

func (h *SystemHandler) ApplyWireGuardUpdate(c *gin.Context) {
	st := h.updater.Status()
	if !st.Supported {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "Updates are not supported here: " + st.Unsupported})
		return
	}
	if st.Checking || st.Updating {
		c.JSON(http.StatusConflict, models.ErrorResponse{Error: "A check or update is already running"})
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		if _, err := h.updater.Apply(ctx, false); err != nil {
			log.Printf("wireguard: manual update: %v", err)
		}
	}()
	c.JSON(http.StatusAccepted, gin.H{"success": true, "message": "Update started"})
}

func (h *SystemHandler) SetWireGuardAutoUpdate(c *gin.Context) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Error: "Invalid request"})
		return
	}
	h.updater.SetAuto(*req.Enabled)
	c.JSON(http.StatusOK, gin.H{"success": true, "auto_update": *req.Enabled})
}
