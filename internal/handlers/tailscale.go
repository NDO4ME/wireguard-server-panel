package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"wgeasygo/internal/config"
	"wgeasygo/internal/db"
	"wgeasygo/internal/models"
	"wgeasygo/pkg/tailscale"
)

type TailscaleHandler struct {
	config  *config.Config
	manager *tailscale.Manager
}

func NewTailscaleHandler(cfg *config.Config) *TailscaleHandler {
	return &TailscaleHandler{
		config:  cfg,
		manager: tailscale.NewManager(),
	}
}

// GetStatus returns current Tailscale status
func (h *TailscaleHandler) GetStatus(c *gin.Context) {
	status, err := h.manager.GetStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: err.Error(),
		})
		return
	}

	// Get routing enabled status from settings
	routingEnabled := false
	if val, _ := db.DB.GetSetting("tailscale_routing_enabled"); val == "true" {
		routingEnabled = true
	}

	c.JSON(http.StatusOK, gin.H{
		"installed":       h.manager.IsInstalled(),
		"connected":       status.Connected,
		"backend_state":   status.BackendState,
		"auth_url":        status.AuthURL,
		"self":            status.Self,
		"peers":           status.Peers,
		"routes":          status.Routes,
		"routing_enabled": routingEnabled,
	})
}

// Connect starts Tailscale connection
func (h *TailscaleHandler) Connect(c *gin.Context) {
	status, err := h.manager.Up()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: err.Error(),
		})
		return
	}

	response := gin.H{
		"success":       true,
		"connected":     status.Connected,
		"backend_state": status.BackendState,
	}

	if status.AuthURL != "" {
		response["auth_url"] = status.AuthURL
		response["message"] = "Please authenticate using the URL"
	} else if status.Connected {
		response["message"] = "Tailscale connected successfully"
	} else {
		response["message"] = "Tailscale is connecting..."
	}

	c.JSON(http.StatusOK, response)
}

// Disconnect stops Tailscale connection
func (h *TailscaleHandler) Disconnect(c *gin.Context) {
	// First disable routing if enabled
	if val, _ := db.DB.GetSetting("tailscale_routing_enabled"); val == "true" {
		h.manager.ClearRouting(h.config.WireGuard.Subnet)
		db.DB.SetSetting("tailscale_routing_enabled", "false")
	}

	if err := h.manager.Down(); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Tailscale disconnected",
	})
}

// EnableRouting sets up iptables for WireGuard to Tailscale routing
func (h *TailscaleHandler) EnableRouting(c *gin.Context) {
	// Check if Tailscale is connected
	status, err := h.manager.GetStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: err.Error(),
		})
		return
	}

	if !status.Connected {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "Tailscale must be connected before enabling routing",
		})
		return
	}

	// Setup routing
	if err := h.manager.SetupRouting(h.config.WireGuard.Subnet); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: err.Error(),
		})
		return
	}

	// Save setting
	if err := db.DB.SetSetting("tailscale_routing_enabled", "true"); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Failed to save routing setting",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Routing enabled - WireGuard clients can now access Tailscale network",
		"routes":  status.Routes,
	})
}

// DisableRouting removes iptables routing rules
func (h *TailscaleHandler) DisableRouting(c *gin.Context) {
	if err := h.manager.ClearRouting(h.config.WireGuard.Subnet); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: err.Error(),
		})
		return
	}

	// Save setting
	if err := db.DB.SetSetting("tailscale_routing_enabled", "false"); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Failed to save routing setting",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Routing disabled",
	})
}

// GetRoutes returns available Tailscale routes/subnets
func (h *TailscaleHandler) GetRoutes(c *gin.Context) {
	status, err := h.manager.GetStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"routes": status.Routes,
		"peers":  status.Peers,
	})
}
