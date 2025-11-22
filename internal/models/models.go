package models

import "time"

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	APIToken     string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Peer struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	PublicKey  string    `json:"public_key"`
	PrivateKey string    `json:"-"` // Never exposed via API
	AssignedIP string    `json:"assigned_ip"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type RefreshToken struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// API Request/Response types
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	APIToken    string `json:"api_token"`
}

type CreatePeerRequest struct {
	Name string `json:"name" binding:"required"`
}

type PeerResponse struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	PublicKey       string    `json:"public_key"`
	AssignedIP      string    `json:"assigned_ip"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	// Real-time stats
	IsOnline        bool      `json:"is_online"`
	LatestHandshake time.Time `json:"latest_handshake,omitempty"`
	TransferRx      int64     `json:"transfer_rx"`
	TransferTx      int64     `json:"transfer_tx"`
	Endpoint        string    `json:"endpoint,omitempty"`
}

type UpdatePeerRequest struct {
	Name    *string `json:"name,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

type ConnectionLog struct {
	ID          int64     `json:"id"`
	PeerID      int64     `json:"peer_id"`
	Endpoint    string    `json:"endpoint"`
	ConnectedAt time.Time `json:"connected_at"`
}

type SettingsResponse struct {
	DNS            string `json:"dns"`
	AllowedIPs     string `json:"allowed_ips"`
	LoggingEnabled bool   `json:"logging_enabled"`
	APIToken       string `json:"api_token"`
}

type UpdateSettingsRequest struct {
	DNS            *string `json:"dns,omitempty"`
	AllowedIPs     *string `json:"allowed_ips,omitempty"`
	LoggingEnabled *bool   `json:"logging_enabled,omitempty"`
	AdminPassword  *string `json:"admin_password,omitempty"`
}
