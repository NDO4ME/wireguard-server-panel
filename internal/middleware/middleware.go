package middleware

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
	"wgeasygo/internal/auth"
	"wgeasygo/internal/config"
	"wgeasygo/internal/db"
)

// AuthMiddleware validates JWT access tokens or API tokens from Authorization header
func AuthMiddleware(cfg *config.JWTConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		// Check Bearer scheme
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization format. Use: Bearer <token>"})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// Reject empty tokens
		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token required"})
			c.Abort()
			return
		}

		// First try to validate as JWT access token
		claims, err := auth.ValidateAccessToken(tokenString, cfg)
		if err == nil {
			// Valid JWT token
			c.Set("user_id", claims.UserID)
			c.Set("username", claims.Username)
			c.Next()
			return
		}

		// If JWT validation failed, try API token (exactly 43 characters)
		if len(tokenString) == 43 {
			user, apiErr := db.DB.GetUserByAPIToken(tokenString)
			if apiErr == nil {
				// Valid API token
				c.Set("user_id", user.ID)
				c.Set("username", user.Username)
				c.Next()
				return
			}
		}

		// Both validations failed - use generic error for security
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
		c.Abort()
	}
}

// RateLimiter provides IP-based rate limiting
type RateLimiter struct {
	visitors map[string]*visitorInfo
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
	ctx      context.Context
	cancel   context.CancelFunc
}

type visitorInfo struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func NewRateLimiter(requests int, windowSeconds int) *RateLimiter {
	ctx, cancel := context.WithCancel(context.Background())
	rl := &RateLimiter{
		visitors: make(map[string]*visitorInfo),
		rate:     rate.Limit(float64(requests) / float64(windowSeconds)),
		burst:    requests,
		ctx:      ctx,
		cancel:   cancel,
	}

	// Cleanup old entries periodically
	go rl.cleanupVisitors()

	return rl
}

// Stop gracefully stops the rate limiter cleanup goroutine
func (rl *RateLimiter) Stop() {
	rl.cancel()
}

func (rl *RateLimiter) getVisitor(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	if !exists {
		limiter := rate.NewLimiter(rl.rate, rl.burst)
		rl.visitors[ip] = &visitorInfo{limiter: limiter, lastSeen: time.Now()}
		return limiter
	}

	v.lastSeen = time.Now()
	return v.limiter
}

func (rl *RateLimiter) cleanupVisitors() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rl.ctx.Done():
			return
		case <-ticker.C:
			rl.mu.Lock()
			for ip, v := range rl.visitors {
				if time.Since(v.lastSeen) > 3*time.Minute {
					delete(rl.visitors, ip)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// Middleware returns a Gin middleware for rate limiting
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		limiter := rl.getVisitor(ip)

		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":   "Too many requests",
				"message": "Please try again later",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// SecurityHeaders adds security headers to responses
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'; img-src 'self' data:")
		c.Next()
	}
}

// CORS middleware - nginx proxy compatible
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		// If origin is present, reflect it back (allows any origin that sends the header)
		// This is safe because we use SameSite=Strict cookies for CSRF protection
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
		} else {
			// For same-origin requests or requests without Origin header
			c.Header("Access-Control-Allow-Origin", "*")
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With")
		c.Header("Access-Control-Expose-Headers", "Content-Length, Content-Type")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
