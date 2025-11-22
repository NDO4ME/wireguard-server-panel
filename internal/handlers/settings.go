package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"wgeasygo/internal/auth"
	"wgeasygo/internal/config"
	"wgeasygo/internal/db"
	"wgeasygo/internal/models"
)

type SettingsHandler struct {
	config *config.Config
}

func NewSettingsHandler(cfg *config.Config) *SettingsHandler {
	return &SettingsHandler{config: cfg}
}

func (h *SettingsHandler) GetSettings(c *gin.Context) {
	settings, err := db.DB.GetAllSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Failed to get settings",
		})
		return
	}

	// Default values
	dns := h.config.WireGuard.DNS
	if val, ok := settings["dns"]; ok && val != "" {
		dns = val
	}

	allowedIPs := h.config.WireGuard.AllowedIPs
	if val, ok := settings["allowed_ips"]; ok && val != "" {
		allowedIPs = val
	}

	loggingEnabled := false
	if val, ok := settings["logging_enabled"]; ok && val == "true" {
		loggingEnabled = true
	}

	// Get API token from user
	apiToken := ""
	user, err := db.DB.GetUserByUsername(h.config.Admin.Username)
	if err == nil {
		apiToken = user.APIToken
	}

	c.JSON(http.StatusOK, models.SettingsResponse{
		DNS:            dns,
		AllowedIPs:     allowedIPs,
		LoggingEnabled: loggingEnabled,
		APIToken:       apiToken,
	})
}

func (h *SettingsHandler) UpdateSettings(c *gin.Context) {
	var req models.UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "Invalid request",
		})
		return
	}

	// Update DNS
	if req.DNS != nil {
		if err := db.DB.SetSetting("dns", *req.DNS); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error: "Failed to update DNS",
			})
			return
		}
		// Update config in memory
		h.config.WireGuard.DNS = *req.DNS
	}

	// Update AllowedIPs
	if req.AllowedIPs != nil {
		if err := db.DB.SetSetting("allowed_ips", *req.AllowedIPs); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error: "Failed to update AllowedIPs",
			})
			return
		}
		// Update config in memory
		h.config.WireGuard.AllowedIPs = *req.AllowedIPs
	}

	// Update logging enabled
	if req.LoggingEnabled != nil {
		value := "false"
		if *req.LoggingEnabled {
			value = "true"
		}
		if err := db.DB.SetSetting("logging_enabled", value); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error: "Failed to update logging setting",
			})
			return
		}
	}

	// Update admin password
	if req.AdminPassword != nil && *req.AdminPassword != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(*req.AdminPassword), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error: "Failed to hash password",
			})
			return
		}

		// Update password in database
		user, err := db.DB.GetUserByUsername(h.config.Admin.Username)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error: "Failed to find admin user",
			})
			return
		}

		if err := db.DB.UpdateUserPassword(user.ID, string(hashedPassword)); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error: "Failed to update password",
			})
			return
		}

		// Generate new API token from new password
		newAPIToken := auth.GenerateAPIToken(*req.AdminPassword, h.config.JWT.AccessSecret)
		if err := db.DB.UpdateUserAPIToken(user.ID, newAPIToken); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error: "Failed to update API token",
			})
			return
		}

		// Invalidate all existing refresh tokens (force re-login)
		db.DB.DeleteUserRefreshTokens(user.ID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Settings updated"})
}

func (h *SettingsHandler) GetPeerLogs(c *gin.Context) {
	ip := c.Param("ip")
	if ip == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "IP address required",
		})
		return
	}

	// Get peer by IP
	peer, err := db.DB.GetPeerByIP(ip)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "Peer not found",
		})
		return
	}

	// Get logs (limit to 100)
	logs, err := db.DB.GetConnectionLogs(peer.ID, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Failed to get logs",
		})
		return
	}

	if logs == nil {
		logs = []models.ConnectionLog{}
	}

	c.JSON(http.StatusOK, logs)
}
