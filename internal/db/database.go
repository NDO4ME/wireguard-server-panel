package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"wgeasygo/internal/models"
)

type Database struct {
	conn *sql.DB
}

var DB *Database

func Initialize(dbPath string) (*Database, error) {
	// Create directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Optimized SQLite settings for better performance
	// auto_vacuum=INCREMENTAL automatically reclaims space when data is deleted
	dsn := dbPath + "?_journal_mode=WAL&_foreign_keys=on&_synchronous=NORMAL&_cache_size=-64000&_busy_timeout=5000&_auto_vacuum=INCREMENTAL"
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	conn.SetMaxOpenConns(1) // SQLite supports only one writer
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(time.Hour)

	// Verify connection
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &Database{conn: conn}

	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	DB = db
	return db, nil
}

func (d *Database) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS peers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			public_key TEXT UNIQUE NOT NULL,
			private_key TEXT NOT NULL,
			assigned_ip TEXT UNIQUE NOT NULL,
			enabled INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS refresh_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token TEXT UNIQUE NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS connection_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			peer_id INTEGER NOT NULL,
			endpoint TEXT NOT NULL,
			connected_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (peer_id) REFERENCES peers(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_peers_assigned_ip ON peers(assigned_ip)`,
		`CREATE INDEX IF NOT EXISTS idx_refresh_tokens_token ON refresh_tokens(token)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_logs_peer_id ON connection_logs(peer_id)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_logs_connected_at ON connection_logs(connected_at)`,
	}

	for _, migration := range migrations {
		if _, err := d.conn.Exec(migration); err != nil {
			return err
		}
	}

	// Add api_token column if it doesn't exist (for existing databases)
	// This must run before creating the index on api_token
	_, _ = d.conn.Exec("ALTER TABLE users ADD COLUMN api_token TEXT DEFAULT ''")

	// Create index on api_token after the column exists
	_, _ = d.conn.Exec("CREATE INDEX IF NOT EXISTS idx_users_api_token ON users(api_token)")

	return nil
}

func (d *Database) Close() error {
	return d.conn.Close()
}

// User operations
func (d *Database) CreateUser(username, passwordHash string) (*models.User, error) {
	result, err := d.conn.Exec(
		"INSERT INTO users (username, password_hash, api_token) VALUES (?, ?, '')",
		username, passwordHash,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return d.GetUserByID(id)
}

func (d *Database) GetUserByID(id int64) (*models.User, error) {
	var user models.User
	err := d.conn.QueryRow(
		"SELECT id, username, password_hash, COALESCE(api_token, ''), created_at, updated_at FROM users WHERE id = ?",
		id,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.APIToken, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (d *Database) GetUserByUsername(username string) (*models.User, error) {
	var user models.User
	err := d.conn.QueryRow(
		"SELECT id, username, password_hash, COALESCE(api_token, ''), created_at, updated_at FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.APIToken, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByAPIToken finds a user by their API token
func (d *Database) GetUserByAPIToken(token string) (*models.User, error) {
	// Prevent empty token lookups
	if token == "" {
		return nil, sql.ErrNoRows
	}
	var user models.User
	err := d.conn.QueryRow(
		"SELECT id, username, password_hash, COALESCE(api_token, ''), created_at, updated_at FROM users WHERE api_token = ? AND api_token != ''",
		token,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.APIToken, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdateUserAPIToken updates the user's API token
func (d *Database) UpdateUserAPIToken(userID int64, apiToken string) error {
	_, err := d.conn.Exec(
		"UPDATE users SET api_token = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		apiToken, userID,
	)
	return err
}

func (d *Database) UserExists(username string) (bool, error) {
	var count int
	err := d.conn.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (d *Database) UpdateUserPassword(userID int64, passwordHash string) error {
	_, err := d.conn.Exec(
		"UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		passwordHash, userID,
	)
	return err
}

// Peer operations
func (d *Database) CreatePeer(peer *models.Peer) (*models.Peer, error) {
	result, err := d.conn.Exec(
		"INSERT INTO peers (name, public_key, private_key, assigned_ip, enabled) VALUES (?, ?, ?, ?, ?)",
		peer.Name, peer.PublicKey, peer.PrivateKey, peer.AssignedIP, peer.Enabled,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return d.GetPeerByID(id)
}

func (d *Database) GetPeerByID(id int64) (*models.Peer, error) {
	var peer models.Peer
	err := d.conn.QueryRow(
		"SELECT id, name, public_key, private_key, assigned_ip, enabled, created_at, updated_at FROM peers WHERE id = ?",
		id,
	).Scan(&peer.ID, &peer.Name, &peer.PublicKey, &peer.PrivateKey, &peer.AssignedIP, &peer.Enabled, &peer.CreatedAt, &peer.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &peer, nil
}

func (d *Database) GetPeerByIP(ip string) (*models.Peer, error) {
	var peer models.Peer
	err := d.conn.QueryRow(
		"SELECT id, name, public_key, private_key, assigned_ip, enabled, created_at, updated_at FROM peers WHERE assigned_ip = ?",
		ip,
	).Scan(&peer.ID, &peer.Name, &peer.PublicKey, &peer.PrivateKey, &peer.AssignedIP, &peer.Enabled, &peer.CreatedAt, &peer.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &peer, nil
}

func (d *Database) UpdatePeer(ip string, name *string, enabled *bool) (*models.Peer, error) {
	if name != nil {
		_, err := d.conn.Exec("UPDATE peers SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE assigned_ip = ?", *name, ip)
		if err != nil {
			return nil, err
		}
	}
	if enabled != nil {
		_, err := d.conn.Exec("UPDATE peers SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE assigned_ip = ?", *enabled, ip)
		if err != nil {
			return nil, err
		}
	}
	return d.GetPeerByIP(ip)
}

func (d *Database) GetAllPeers() ([]models.Peer, error) {
	rows, err := d.conn.Query(
		"SELECT id, name, public_key, private_key, assigned_ip, enabled, created_at, updated_at FROM peers ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peers []models.Peer
	for rows.Next() {
		var peer models.Peer
		if err := rows.Scan(&peer.ID, &peer.Name, &peer.PublicKey, &peer.PrivateKey, &peer.AssignedIP, &peer.Enabled, &peer.CreatedAt, &peer.UpdatedAt); err != nil {
			return nil, err
		}
		peers = append(peers, peer)
	}

	return peers, rows.Err()
}

func (d *Database) DeletePeer(ip string) error {
	result, err := d.conn.Exec("DELETE FROM peers WHERE assigned_ip = ?", ip)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (d *Database) GetNextAvailableIP(subnet string) (string, error) {
	// Parse subnet to get base IP (e.g., "10.8.0.0/24" -> "10.8.0")
	baseIP := subnet
	if idx := len(baseIP) - 1; idx > 0 {
		for i := len(baseIP) - 1; i >= 0; i-- {
			if baseIP[i] == '/' {
				baseIP = baseIP[:i]
				break
			}
		}
	}

	// Get the base (first 3 octets)
	var o1, o2, o3, o4 int
	fmt.Sscanf(baseIP, "%d.%d.%d.%d", &o1, &o2, &o3, &o4)

	// Get all assigned IPs using optimized query
	rows, err := d.conn.Query("SELECT assigned_ip FROM peers ORDER BY assigned_ip")
	if err != nil {
		return "", err
	}
	defer rows.Close()

	// Use a set for O(1) lookup
	usedIPs := make(map[string]struct{}, 256)
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return "", err
		}
		usedIPs[ip] = struct{}{}
	}

	if err := rows.Err(); err != nil {
		return "", err
	}

	// Find next available IP (start from .2, .1 is usually server)
	for i := 2; i < 255; i++ {
		ip := fmt.Sprintf("%d.%d.%d.%d", o1, o2, o3, i)
		if _, exists := usedIPs[ip]; !exists {
			return ip, nil
		}
	}

	return "", fmt.Errorf("no available IP addresses in subnet")
}

// Refresh token operations
func (d *Database) SaveRefreshToken(userID int64, token string, expiresAt time.Time) error {
	_, err := d.conn.Exec(
		"INSERT INTO refresh_tokens (user_id, token, expires_at) VALUES (?, ?, ?)",
		userID, token, expiresAt,
	)
	return err
}

func (d *Database) GetRefreshToken(token string) (*models.RefreshToken, error) {
	var rt models.RefreshToken
	err := d.conn.QueryRow(
		"SELECT id, user_id, token, expires_at, created_at FROM refresh_tokens WHERE token = ?",
		token,
	).Scan(&rt.ID, &rt.UserID, &rt.Token, &rt.ExpiresAt, &rt.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &rt, nil
}

func (d *Database) DeleteRefreshToken(token string) error {
	_, err := d.conn.Exec("DELETE FROM refresh_tokens WHERE token = ?", token)
	return err
}

func (d *Database) DeleteUserRefreshTokens(userID int64) error {
	_, err := d.conn.Exec("DELETE FROM refresh_tokens WHERE user_id = ?", userID)
	return err
}

func (d *Database) CleanExpiredTokens() error {
	_, err := d.conn.Exec("DELETE FROM refresh_tokens WHERE expires_at < ?", time.Now())
	return err
}

// Optimize performs database maintenance to reclaim space and optimize performance
func (d *Database) Optimize() error {
	// Run incremental vacuum to reclaim free pages (non-blocking)
	if _, err := d.conn.Exec("PRAGMA incremental_vacuum(100)"); err != nil {
		return err
	}

	// Optimize query planner statistics
	if _, err := d.conn.Exec("PRAGMA optimize"); err != nil {
		return err
	}

	return nil
}

// FullVacuum performs a full database vacuum (use sparingly, blocks writes)
func (d *Database) FullVacuum() error {
	_, err := d.conn.Exec("VACUUM")
	return err
}

// Settings operations
func (d *Database) GetSetting(key string) (string, error) {
	var value string
	err := d.conn.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (d *Database) SetSetting(key, value string) error {
	_, err := d.conn.Exec(`
		INSERT INTO settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = CURRENT_TIMESTAMP
	`, key, value, value)
	return err
}

func (d *Database) GetAllSettings() (map[string]string, error) {
	rows, err := d.conn.Query("SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		settings[key] = value
	}
	return settings, rows.Err()
}

// Connection log operations
func (d *Database) AddConnectionLog(peerID int64, endpoint string) error {
	// Only log if this endpoint is different from the last logged endpoint for this peer
	var lastEndpoint string
	err := d.conn.QueryRow(`
		SELECT endpoint FROM connection_logs
		WHERE peer_id = ?
		ORDER BY connected_at DESC
		LIMIT 1
	`, peerID).Scan(&lastEndpoint)

	// If there's a previous log and it's the same endpoint, skip
	if err == nil && lastEndpoint == endpoint {
		return nil // Same endpoint as last log, skip
	}

	// Log new endpoint (either first log or different endpoint)
	_, err = d.conn.Exec(
		"INSERT INTO connection_logs (peer_id, endpoint) VALUES (?, ?)",
		peerID, endpoint,
	)
	return err
}

func (d *Database) GetConnectionLogs(peerID int64, limit int) ([]models.ConnectionLog, error) {
	rows, err := d.conn.Query(`
		SELECT id, peer_id, endpoint, connected_at
		FROM connection_logs
		WHERE peer_id = ?
		ORDER BY connected_at DESC
		LIMIT ?
	`, peerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.ConnectionLog
	for rows.Next() {
		var log models.ConnectionLog
		if err := rows.Scan(&log.ID, &log.PeerID, &log.Endpoint, &log.ConnectedAt); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (d *Database) DeletePeerLogs(peerID int64) error {
	_, err := d.conn.Exec("DELETE FROM connection_logs WHERE peer_id = ?", peerID)
	return err
}
