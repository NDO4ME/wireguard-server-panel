package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/skip2/go-qrcode"
	"wgeasygo/internal/config"
	"wgeasygo/internal/db"
	"wgeasygo/internal/models"
	"wgeasygo/pkg/wgmanager"
)

type PeerHandler struct {
	config    *config.Config
	wgManager *wgmanager.WGManager
}

func NewPeerHandler(cfg *config.Config, wg *wgmanager.WGManager) *PeerHandler {
	return &PeerHandler{
		config:    cfg,
		wgManager: wg,
	}
}

// CreatePeer creates a new WireGuard peer
func (h *PeerHandler) CreatePeer(c *gin.Context) {
	var req models.CreatePeerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "Invalid request body",
		})
		return
	}

	// Generate key pair
	privateKey, publicKey, err := wgmanager.GenerateKeyPair()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Failed to generate key pair",
		})
		return
	}

	// Get next available IP
	assignedIP, err := db.DB.GetNextAvailableIP(h.config.WireGuard.Subnet)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "Failed to assign IP address",
			Message: err.Error(),
		})
		return
	}

	// Create peer in database
	peer := &models.Peer{
		Name:       req.Name,
		PublicKey:  publicKey,
		PrivateKey: privateKey,
		AssignedIP: assignedIP,
		Enabled:    true,
	}

	createdPeer, err := db.DB.CreatePeer(peer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "Failed to create peer",
			Message: err.Error(),
		})
		return
	}

	// Add peer to WireGuard interface
	if err := h.wgManager.AddPeer(publicKey, assignedIP); err != nil {
		// Rollback database entry on failure
		db.DB.DeletePeer(assignedIP)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "Failed to add peer to WireGuard",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, models.PeerResponse{
		ID:         createdPeer.ID,
		Name:       createdPeer.Name,
		PublicKey:  createdPeer.PublicKey,
		AssignedIP: createdPeer.AssignedIP,
		Enabled:    createdPeer.Enabled,
		CreatedAt:  createdPeer.CreatedAt,
	})
}

// ListPeers returns all managed peers with real-time stats
func (h *PeerHandler) ListPeers(c *gin.Context) {
	peers, err := db.DB.GetAllPeers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Failed to retrieve peers",
		})
		return
	}

	// Get real-time stats from WireGuard
	stats, _ := h.wgManager.GetPeerStats() // Ignore error, stats are optional

	// Check if logging is enabled
	loggingEnabled := false
	if logSetting, _ := db.DB.GetSetting("logging_enabled"); logSetting == "true" {
		loggingEnabled = true
	}

	// Convert to response format (without private keys)
	// Pre-allocate slice to avoid repeated allocations
	response := make([]models.PeerResponse, 0, len(peers))
	for _, peer := range peers {
		resp := models.PeerResponse{
			ID:         peer.ID,
			Name:       peer.Name,
			PublicKey:  peer.PublicKey,
			AssignedIP: peer.AssignedIP,
			Enabled:    peer.Enabled,
			CreatedAt:  peer.CreatedAt,
		}

		// Add real-time stats if available
		if stats != nil {
			if peerStats, ok := stats[peer.PublicKey]; ok {
				resp.IsOnline = peerStats.IsOnline
				resp.LatestHandshake = peerStats.LatestHandshake
				resp.TransferRx = peerStats.TransferRx
				resp.TransferTx = peerStats.TransferTx
				resp.Endpoint = peerStats.Endpoint

				// Log connection if enabled and peer is online with endpoint
				if loggingEnabled && peerStats.IsOnline && peerStats.Endpoint != "" {
					// AddConnectionLog already handles duplicate prevention
					_ = db.DB.AddConnectionLog(peer.ID, peerStats.Endpoint)
				}
			}
		}

		response = append(response, resp)
	}

	c.JSON(http.StatusOK, response)
}

// UpdatePeer updates a peer's name or enabled status
func (h *PeerHandler) UpdatePeer(c *gin.Context) {
	ip := c.Param("ip")
	if !wgmanager.ValidateIP(ip) {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "Invalid IP address format",
		})
		return
	}

	var req models.UpdatePeerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "Invalid request body",
		})
		return
	}

	// Get current peer
	peer, err := db.DB.GetPeerByIP(ip)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "Peer not found",
		})
		return
	}

	// Handle enable/disable in WireGuard
	if req.Enabled != nil {
		if *req.Enabled && !peer.Enabled {
			// Enable: add peer back to WireGuard
			if err := h.wgManager.AddPeer(peer.PublicKey, peer.AssignedIP); err != nil {
				c.JSON(http.StatusInternalServerError, models.ErrorResponse{
					Error:   "Failed to enable peer",
					Message: err.Error(),
				})
				return
			}
		} else if !*req.Enabled && peer.Enabled {
			// Disable: remove peer from WireGuard
			if err := h.wgManager.RemovePeer(peer.PublicKey); err != nil {
				c.JSON(http.StatusInternalServerError, models.ErrorResponse{
					Error:   "Failed to disable peer",
					Message: err.Error(),
				})
				return
			}
		}
	}

	// Update in database
	updatedPeer, err := db.DB.UpdatePeer(ip, req.Name, req.Enabled)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Failed to update peer",
		})
		return
	}

	c.JSON(http.StatusOK, models.PeerResponse{
		ID:         updatedPeer.ID,
		Name:       updatedPeer.Name,
		PublicKey:  updatedPeer.PublicKey,
		AssignedIP: updatedPeer.AssignedIP,
		Enabled:    updatedPeer.Enabled,
		CreatedAt:  updatedPeer.CreatedAt,
	})
}

// DeletePeer removes a peer by its assigned IP
func (h *PeerHandler) DeletePeer(c *gin.Context) {
	ip := c.Param("ip")
	if !wgmanager.ValidateIP(ip) {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "Invalid IP address format",
		})
		return
	}

	// Get peer from database to get public key
	peer, err := db.DB.GetPeerByIP(ip)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "Peer not found",
		})
		return
	}

	// Remove from WireGuard interface
	if err := h.wgManager.RemovePeer(peer.PublicKey); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "Failed to remove peer from WireGuard",
			Message: err.Error(),
		})
		return
	}

	// Delete from database
	if err := db.DB.DeletePeer(ip); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Failed to delete peer from database",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Peer deleted successfully"})
}

// GetPeerConfig returns the client configuration file for a peer
func (h *PeerHandler) GetPeerConfig(c *gin.Context) {
	ip := c.Param("ip")
	if !wgmanager.ValidateIP(ip) {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "Invalid IP address format",
		})
		return
	}

	peer, err := db.DB.GetPeerByIP(ip)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "Peer not found",
		})
		return
	}

	configContent, err := h.wgManager.GenerateClientConfig(peer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Failed to generate configuration",
		})
		return
	}

	// Set headers for file download
	filename := peer.Name + ".conf"
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", "text/plain")
	c.String(http.StatusOK, configContent)
}

// GetPeerQRCode generates a QR code image for the peer's configuration
func (h *PeerHandler) GetPeerQRCode(c *gin.Context) {
	ip := c.Param("ip")
	if !wgmanager.ValidateIP(ip) {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "Invalid IP address format",
		})
		return
	}

	peer, err := db.DB.GetPeerByIP(ip)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "Peer not found",
		})
		return
	}

	configContent, err := h.wgManager.GenerateClientConfig(peer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Failed to generate configuration",
		})
		return
	}

	// Generate QR code
	qr, err := qrcode.Encode(configContent, qrcode.Medium, 256)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Failed to generate QR code",
		})
		return
	}

	c.Header("Content-Type", "image/png")
	c.Data(http.StatusOK, "image/png", qr)
}
