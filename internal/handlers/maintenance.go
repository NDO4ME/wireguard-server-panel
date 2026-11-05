package handlers

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ndo4me/wireguard-server-panel/internal/auth"
	"github.com/ndo4me/wireguard-server-panel/internal/db"
	"github.com/ndo4me/wireguard-server-panel/internal/models"
)

// RotateAPIToken replaces the admin API token with a new random one. The old
// token stops working immediately; browser sessions (JWT) are unaffected.
func (h *SettingsHandler) RotateAPIToken(c *gin.Context) {
	user, err := db.DB.GetUserByUsername(h.config.Admin.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "Failed to load user"})
		return
	}
	tok, err := auth.GenerateAPIToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "Failed to generate token"})
		return
	}
	if err := db.DB.UpdateUserAPIToken(user.ID, tok); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "Failed to save token"})
		return
	}
	log.Printf("API token rotated by admin session from %s", c.ClientIP())
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"api_token": tok})
}

// DownloadBackup streams a consistent SQLite snapshot (clients, keys, settings).
// The file contains private keys, so it is only served to authenticated admins
// and never cached.
func (h *SettingsHandler) DownloadBackup(c *gin.Context) {
	dir, err := os.MkdirTemp("", "ndo4wg-backup-")
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "Failed to prepare backup"})
		return
	}
	defer os.RemoveAll(dir)
	dest := filepath.Join(dir, "backup.db")
	if err := db.DB.Backup(dest); err != nil {
		log.Printf("backup failed: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Error: "Failed to create backup"})
		return
	}
	_ = os.Chmod(dest, 0o600)
	name := "ndo4wg-backup-" + time.Now().UTC().Format("20060102-150405") + ".db"
	log.Printf("database backup downloaded from %s", c.ClientIP())
	_ = db.DB.AddAudit(c.ClientIP(), "GET", "/api/v1/settings/backup", http.StatusOK)
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Header("Content-Type", "application/octet-stream")
	c.File(dest)
}
