// CVE-2026-98432 (Linux kernel IPv4 TCP use-after-free): the known exploit path
// needs a Multipath TCP listener, and Go 1.24+ opens MPTCP listeners by default.
// The panel never needs MPTCP, so it is switched off for every listener.
//go:debug multipathtcp=0

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ndo4me/wireguard-server-panel/internal/auth"
	"github.com/ndo4me/wireguard-server-panel/internal/config"
	"github.com/ndo4me/wireguard-server-panel/internal/db"
	"github.com/ndo4me/wireguard-server-panel/internal/handlers"
	"github.com/ndo4me/wireguard-server-panel/internal/middleware"
	"github.com/ndo4me/wireguard-server-panel/pkg/tailscale"
	"github.com/ndo4me/wireguard-server-panel/pkg/wgmanager"
	"github.com/ndo4me/wireguard-server-panel/pkg/wgserver"
	"github.com/ndo4me/wireguard-server-panel/pkg/wgupdate"
)

// appVersion is overridden at build time: -ldflags "-X main.appVersion=1.2.3"
var appVersion = "dev"

func main() {
	// Load configuration
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "configs/config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize database
	database, err := db.Initialize(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Create admin user if it doesn't exist
	if err := ensureAdminUser(cfg); err != nil {
		log.Fatalf("Failed to create admin user: %v", err)
	}
	if err := ensureAPIToken(cfg); err != nil {
		log.Fatalf("Failed to prepare API token: %v", err)
	}

	// Auto-detect WireGuard server configuration
	if err := autoConfigureWireGuard(cfg); err != nil {
		log.Printf("Warning: Failed to auto-configure WireGuard: %v", err)
	}

	// Load saved settings from database
	if err := loadSavedSettings(cfg); err != nil {
		log.Printf("Warning: Failed to load saved settings: %v", err)
	}

	// Initialize WireGuard manager
	wgManager := wgmanager.New(&cfg.WireGuard)

	// Sync existing peers to WireGuard interface
	peers, err := db.DB.GetAllPeers()
	if err != nil {
		log.Printf("Warning: Failed to get peers for sync: %v", err)
	} else {
		if err := wgManager.SyncPeersToInterface(peers); err != nil {
			log.Printf("Warning: Failed to sync peers: %v", err)
		}
	}

	// Set Gin mode to release for production (no debug logs)
	gin.SetMode(gin.ReleaseMode)

	// Create router
	router := gin.New()

	// Add middlewares
	// Only Recovery middleware - no request logging to reduce log volume
	router.Use(gin.Recovery())
	router.Use(middleware.SecurityHeaders())

	// Only trust X-Forwarded-For from a local reverse proxy (Caddy). The panel is
	// published on 127.0.0.1 only, so direct clients can never spoof their IP.
	if err := router.SetTrustedProxies(trustedProxies()); err != nil {
		log.Fatalf("Invalid TRUSTED_PROXIES: %v", err)
	}

	// Tailscale: one shared manager supervises tailscaled and handles updates
	tsManager := tailscale.NewManager()
	tsManager.ConfigureUpdater(db.DB, func() { restoreTailscaleRouting(cfg, tsManager, 30*time.Second) })

	// Create handlers
	authHandler := handlers.NewAuthHandler(cfg)
	peerHandler := handlers.NewPeerHandler(cfg, wgManager)
	settingsHandler := handlers.NewSettingsHandler(cfg)
	tailscaleHandler := handlers.NewTailscaleHandler(cfg, tsManager)

	// WireGuard userspace tools: checked and updated like Tailscale, verified and rolled back on failure
	wgUpdater := wgupdate.New(db.DB, cfg.WireGuard.Interface, wgManager.Exclusive)
	systemHandler := handlers.NewSystemHandler(appVersion, wgManager, tsManager, wgUpdater)

	// Rate limiter for auth endpoints
	rateLimiter := middleware.NewRateLimiter(
		cfg.Security.RateLimitRequests,
		cfg.Security.RateLimitWindowSeconds,
	)

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Auth routes (public)
		v1.Use(middleware.Audit())
		v1.GET("/public/login-page", handlers.PublicLoginPage)

		authGroup := v1.Group("/auth")
		{
			authGroup.POST("/login", rateLimiter.Middleware(), authHandler.Login)
			authGroup.POST("/refresh", authHandler.Refresh)
			authGroup.POST("/logout", authHandler.Logout)
		}

		// Protected routes
		protected := v1.Group("/")
		protected.Use(middleware.AuthMiddleware(&cfg.JWT))
		{
			// Peer management
			peers := protected.Group("/peers")
			{
				peers.POST("", peerHandler.CreatePeer)
				peers.GET("", peerHandler.ListPeers)
				peers.PATCH("/:ip", peerHandler.UpdatePeer)
				peers.DELETE("/:ip", peerHandler.DeletePeer)
				peers.GET("/:ip/config", peerHandler.GetPeerConfig)
				peers.GET("/:ip/qrcode", peerHandler.GetPeerQRCode)
				peers.GET("/:ip/logs", settingsHandler.GetPeerLogs)
			}

			// Settings
			settings := protected.Group("/settings")
			{
				settings.GET("", settingsHandler.GetSettings)
				settings.PUT("", settingsHandler.UpdateSettings)
				settings.POST("/api-token/rotate", settingsHandler.RotateAPIToken)
				settings.GET("/backup", settingsHandler.DownloadBackup)
			}

			// System overview and WireGuard tools updates
			sysGroup := protected.Group("/system")
			sysGroup.GET("/audit", systemHandler.GetAudit)
			{
				sysGroup.GET("/status", systemHandler.GetStatus)
				sysGroup.GET("/wireguard/update/status", systemHandler.GetWireGuardUpdate)
				sysGroup.POST("/wireguard/update/check", systemHandler.CheckWireGuardUpdate)
				sysGroup.POST("/wireguard/update/apply", systemHandler.ApplyWireGuardUpdate)
				sysGroup.PUT("/wireguard/update/auto", systemHandler.SetWireGuardAutoUpdate)
			}

			// Tailscale
			tsGroup := protected.Group("/tailscale")
			{
				tsGroup.GET("/status", tailscaleHandler.GetStatus)
				tsGroup.POST("/connect", tailscaleHandler.Connect)
				tsGroup.POST("/disconnect", tailscaleHandler.Disconnect)
				tsGroup.POST("/routing/enable", tailscaleHandler.EnableRouting)
				tsGroup.POST("/routing/disable", tailscaleHandler.DisableRouting)
				tsGroup.GET("/prefs", tailscaleHandler.GetPrefs)
				tsGroup.PUT("/prefs", tailscaleHandler.SetPrefs)
				tsGroup.GET("/routes", tailscaleHandler.GetRoutes)
				tsGroup.GET("/update/status", tailscaleHandler.GetUpdateStatus)
				tsGroup.POST("/update/check", tailscaleHandler.CheckUpdate)
				tsGroup.POST("/update/apply", tailscaleHandler.ApplyUpdate)
				tsGroup.PUT("/update/auto", tailscaleHandler.SetAutoUpdate)
			}
		}
	}

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "version": appVersion})
	})

	// Serve static frontend files
	router.Static("/assets", "./web/dist/assets")
	router.StaticFile("/logo.svg", "./web/dist/logo.svg")

	// SPA fallback - serve index.html for all non-API routes
	router.NoRoute(func(c *gin.Context) {
		// Don't serve index.html for API routes
		if path := c.Request.URL.Path; path == "/api" || strings.HasPrefix(path, "/api/") {
			c.JSON(404, gin.H{"error": "Not found"})
			return
		}
		c.File("./web/dist/index.html")
	})

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Supervise tailscaled (Docker) and run the Tailscale auto-updater
	var bg sync.WaitGroup
	bg.Add(2)
	go func() {
		defer bg.Done()
		tsManager.RunDaemon(ctx)
	}()
	go func() {
		defer bg.Done()
		tsManager.RunAutoUpdate(ctx, 6*time.Hour)
	}()
	// iptables rules do not survive a container restart: re-apply when enabled
	go restoreTailscaleRouting(cfg, tsManager, 3*time.Minute)

	// Disable clients whose expiry date has passed
	go handlers.RunExpiryEnforcer(ctx, wgManager)

	// Keep WireGuard tools current (verified, rolled back on failure)
	go wgUpdater.Run(ctx, 6*time.Hour)

	// Start periodic maintenance (every hour)
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Clean expired tokens
				if err := db.DB.CleanExpiredTokens(); err != nil {
					log.Printf("Warning: Failed to clean expired tokens: %v", err)
				} else {
					log.Println("Cleaned expired refresh tokens")
				}

				// Optimize database (incremental vacuum + optimize)
				if err := db.DB.Optimize(); err != nil {
					log.Printf("Warning: Failed to optimize database: %v", err)
				} else {
					log.Println("Database optimized")
				}

				// Force garbage collection and return memory to OS
				runtime.GC()
				debug.FreeOSMemory()

				// Log memory stats
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				log.Printf("Memory: Alloc=%vMB, Sys=%vMB, NumGC=%v",
					m.Alloc/1024/1024, m.Sys/1024/1024, m.NumGC)
			}
		}
	}()

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Starting WireGuard Panel on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Cancel background tasks and wait for tailscaled to stop cleanly
	cancel()
	bgDone := make(chan struct{})
	go func() { bg.Wait(); close(bgDone) }()
	select {
	case <-bgDone:
	case <-time.After(25 * time.Second):
		log.Println("Timed out waiting for background tasks")
	}

	// Stop rate limiter cleanup goroutine
	rateLimiter.Stop()

	// Gracefully shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited gracefully")
}

// trustedProxies returns the proxy CIDRs allowed to set X-Forwarded-For.
// Default: loopback and private ranges (reverse proxy on the same host or Docker bridge).
func trustedProxies() []string {
	if v := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES")); v != "" {
		var out []string
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	return []string{"127.0.0.0/8", "::1/128", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
}

// ensureAPIToken makes sure the admin has a random API token. Tokens derived from
// the password by older releases are rotated once.
func ensureAPIToken(cfg *config.Config) error {
	user, err := db.DB.GetUserByUsername(cfg.Admin.Username)
	if err != nil {
		return err
	}
	if v, _ := db.DB.GetSetting("api_token_v2"); v == "1" && user.APIToken != "" {
		return nil
	}
	tok, err := auth.GenerateAPIToken()
	if err != nil {
		return err
	}
	if err := db.DB.UpdateUserAPIToken(user.ID, tok); err != nil {
		return err
	}
	log.Println("API token regenerated (random, no longer derived from the password); update API clients")
	return db.DB.SetSetting("api_token_v2", "1")
}

// ensureAdminUser creates the initial admin user if it doesn't exist
func ensureAdminUser(cfg *config.Config) error {
	exists, err := db.DB.UserExists(cfg.Admin.Username)
	if err != nil {
		return err
	}

	// Rebrand migration: older installs used the fixed username "layerweb".
	// Rename it in place so the password, API token and sessions are kept.
	if !exists && cfg.Admin.Username == "ndo4me" {
		if legacy, lerr := db.DB.UserExists("layerweb"); lerr == nil && legacy {
			if n, rerr := db.DB.RenameUser("layerweb", cfg.Admin.Username); rerr != nil {
				return rerr
			} else if n > 0 {
				log.Printf("Migrated admin user layerweb -> %s", cfg.Admin.Username)
				return nil
			}
		}
	}

	if !exists {
		switch cfg.Admin.Password {
		case "", "admin", "changeme123":
			return fmt.Errorf("refusing to create the admin user with a default password; set ADMIN_PASSWORD")
		}
		passwordHash, err := auth.HashPassword(cfg.Admin.Password, cfg.Security.BcryptCost)
		if err != nil {
			return err
		}

		_, err = db.DB.CreateUser(cfg.Admin.Username, passwordHash)
		if err != nil {
			return err
		}

		log.Printf("Created admin user: %s", cfg.Admin.Username)
	}

	return nil
}

// loadSavedSettings loads saved settings from database and applies them to config
func loadSavedSettings(cfg *config.Config) error {
	settings, err := db.DB.GetAllSettings()
	if err != nil {
		return err
	}

	// Load DNS if saved
	// An empty saved value is valid: clients then get no DNS line at all.
	if dns, ok := settings["dns"]; ok {
		cfg.WireGuard.DNS = dns
		log.Printf("Loaded DNS from settings: %q", dns)
	}

	// Load the endpoint hostname override (client configs only; the port stays fixed)
	if host, ok := settings["endpoint_host"]; ok && host != "" {
		if h, herr := wgmanager.NormalizeEndpointHost(host); herr == nil {
			cfg.WireGuard.EndpointHost = h
			log.Printf("Client endpoint host: %s", h)
		}
	}

	// Load AllowedIPs if saved
	if allowedIPs, ok := settings["allowed_ips"]; ok && allowedIPs != "" {
		cfg.WireGuard.AllowedIPs = allowedIPs
		log.Printf("Loaded AllowedIPs from settings: %s", allowedIPs)
	}

	return nil
}

// autoConfigureWireGuard reads server public key and endpoint from wg0.conf
func autoConfigureWireGuard(cfg *config.Config) error {
	setup := wgserver.NewSetup()

	// Load existing config if available
	if setup.IsConfigured() {
		if err := setup.LoadExistingConfig(); err != nil {
			return err
		}

		serverConfig := setup.GetConfig()
		if serverConfig != nil {
			// Set server public key if not already set
			if cfg.WireGuard.ServerPublicKey == "" && serverConfig.PublicKey != "" {
				cfg.WireGuard.ServerPublicKey = serverConfig.PublicKey
				log.Printf("Auto-detected server public key: %s...", serverConfig.PublicKey[:20])
			}

			// Set server endpoint if not already set
			if cfg.WireGuard.ServerEndpoint == "" && serverConfig.PublicIP != "" {
				// Get WireGuard port from environment or default
				wgPort := os.Getenv("WG_PORT")
				if wgPort == "" {
					wgPort = "51820"
				}
				cfg.WireGuard.ServerEndpoint = serverConfig.PublicIP + ":" + wgPort
				log.Printf("Auto-detected server endpoint: %s", cfg.WireGuard.ServerEndpoint)
			}
		}
	}

	return nil
}

// restoreTailscaleRouting re-applies the WireGuard -> Tailscale iptables rules
// when routing is enabled in settings. It waits up to maxWait for Tailscale to
// be connected (e.g. right after a container start or a daemon update).
func restoreTailscaleRouting(cfg *config.Config, m *tailscale.Manager, maxWait time.Duration) {
	if val, _ := db.DB.GetSetting("tailscale_routing_enabled"); val != "true" {
		return
	}
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if st, err := m.GetStatus(); err == nil && st != nil && st.Connected {
			if err := m.SetupRouting(cfg.WireGuard.Subnet); err != nil {
				log.Printf("Warning: failed to restore Tailscale routing: %v", err)
			} else {
				log.Println("Restored Tailscale routing rules")
			}
			return
		}
		time.Sleep(3 * time.Second)
	}
	log.Println("Tailscale not connected; routing rules not restored")
}
