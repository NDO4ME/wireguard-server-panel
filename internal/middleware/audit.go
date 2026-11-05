package middleware

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ndo4me/wireguard-server-panel/internal/db"
)

// Audit records state-changing requests (and sign-in attempts) with client IP,
// route and result. Request bodies, tokens and query strings are never stored.
func Audit() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		m := c.Request.Method
		if m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions {
			return
		}
		path := c.FullPath() // route pattern, so client IPs/ids are not mixed in
		if path == "" {
			return
		}
		if strings.HasSuffix(path, "/auth/refresh") || strings.Contains(path, "/update/check") {
			return // routine background traffic
		}
		if err := db.DB.AddAudit(c.ClientIP(), m, path, c.Writer.Status()); err != nil {
			log.Printf("audit: %v", err)
		}
	}
}
