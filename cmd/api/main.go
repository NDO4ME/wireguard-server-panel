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
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"wgeasygo/internal/auth"
	"wgeasygo/internal/config"
	"wgeasygo/internal/db"
	"wgeasygo/internal/handlers"
	"wgeasygo/internal/middleware"
	"wgeasygo/pkg/wgmanager"
	"wgeasygo/pkg/wgserver"
)

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
	router.Use(middleware.CORS()) // For development - configure properly for production

	// Create handlers
	authHandler := handlers.NewAuthHandler(cfg)
	peerHandler := handlers.NewPeerHandler(cfg, wgManager)
	settingsHandler := handlers.NewSettingsHandler(cfg)
	tailscaleHandler := handlers.NewTailscaleHandler(cfg)

	// Rate limiter for auth endpoints
	rateLimiter := middleware.NewRateLimiter(
		cfg.Security.RateLimitRequests,
		cfg.Security.RateLimitWindowSeconds,
	)

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Auth routes (public)
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
			}

			// Tailscale
			tailscale := protected.Group("/tailscale")
			{
				tailscale.GET("/status", tailscaleHandler.GetStatus)
				tailscale.POST("/connect", tailscaleHandler.Connect)
				tailscale.POST("/disconnect", tailscaleHandler.Disconnect)
				tailscale.POST("/routing/enable", tailscaleHandler.EnableRouting)
				tailscale.POST("/routing/disable", tailscaleHandler.DisableRouting)
				tailscale.GET("/routes", tailscaleHandler.GetRoutes)
			}
		}
	}

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "version": "1.0.0"})
	})

	// Serve static frontend files
	router.Static("/assets", "./web/dist/assets")
	router.StaticFile("/logo.svg", "./web/dist/logo.svg")

	// SPA fallback - serve index.html for all non-API routes
	router.NoRoute(func(c *gin.Context) {
		// Don't serve index.html for API routes
		if len(c.Request.URL.Path) >= 4 && c.Request.URL.Path[:4] == "/api" {
			c.JSON(404, gin.H{"error": "Not found"})
			return
		}
		c.File("./web/dist/index.html")
	})

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	// Cancel background tasks
	cancel()

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

// ensureAdminUser creates the initial admin user if it doesn't exist
func ensureAdminUser(cfg *config.Config) error {
	exists, err := db.DB.UserExists(cfg.Admin.Username)
	if err != nil {
		return err
	}

	if !exists {
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
	if dns, ok := settings["dns"]; ok && dns != "" {
		cfg.WireGuard.DNS = dns
		log.Printf("Loaded DNS from settings: %s", dns)
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
