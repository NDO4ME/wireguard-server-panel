package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"wgeasygo/internal/auth"
	"wgeasygo/internal/config"
	"wgeasygo/internal/db"
	"wgeasygo/internal/models"
)

type AuthHandler struct {
	config *config.Config
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{config: cfg}
}

// Login handles user authentication and token generation
func (h *AuthHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "Invalid request body",
		})
		return
	}

	// Get user from database
	user, err := db.DB.GetUserByUsername(req.Username)
	if err != nil {
		// Use same error for security (don't reveal if user exists)
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: "Invalid credentials",
		})
		return
	}

	// Verify password
	if err := auth.VerifyPassword(req.Password, user.PasswordHash); err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: "Invalid credentials",
		})
		return
	}

	// Generate deterministic API token from password
	apiToken := auth.GenerateAPIToken(req.Password, h.config.JWT.AccessSecret)

	// Update API token in database if it changed (or first time)
	if user.APIToken != apiToken {
		if err := db.DB.UpdateUserAPIToken(user.ID, apiToken); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error: "Failed to update API token",
			})
			return
		}
	}

	// Generate access token
	accessToken, err := auth.GenerateAccessToken(user.ID, user.Username, &h.config.JWT)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Failed to generate access token",
		})
		return
	}

	// Generate refresh token
	refreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Failed to generate refresh token",
		})
		return
	}

	// Save refresh token to database
	expiresAt := auth.GetRefreshTokenExpiry(&h.config.JWT)
	if err := db.DB.SaveRefreshToken(user.ID, refreshToken, expiresAt); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Failed to save refresh token",
		})
		return
	}

	// Set refresh token as HTTP-only secure cookie with SameSite=Strict
	maxAge := int(time.Until(expiresAt).Seconds())
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(
		"refresh_token",
		refreshToken,
		maxAge,
		"/api/v1/auth",
		"",    // domain - empty for current domain
		true,  // secure - HTTPS only
		true,  // httpOnly - not accessible via JavaScript
	)

	c.JSON(http.StatusOK, models.LoginResponse{
		AccessToken: accessToken,
		ExpiresIn:   h.config.JWT.AccessExpiryMinutes * 60,
		APIToken:    apiToken,
	})
}

// Refresh handles access token refresh using the refresh token cookie
func (h *AuthHandler) Refresh(c *gin.Context) {
	// Get refresh token from cookie
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: "Refresh token not found",
		})
		return
	}

	// Validate refresh token from database
	tokenData, err := db.DB.GetRefreshToken(refreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: "Invalid refresh token",
		})
		return
	}

	// Check if token is expired
	if time.Now().After(tokenData.ExpiresAt) {
		// Delete expired token
		db.DB.DeleteRefreshToken(refreshToken)
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: "Refresh token expired",
		})
		return
	}

	// Get user
	user, err := db.DB.GetUserByID(tokenData.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: "User not found",
		})
		return
	}

	// Generate new access token
	accessToken, err := auth.GenerateAccessToken(user.ID, user.Username, &h.config.JWT)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Failed to generate access token",
		})
		return
	}

	// Optionally rotate refresh token for enhanced security
	// Delete old token
	db.DB.DeleteRefreshToken(refreshToken)

	// Generate new refresh token
	newRefreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Failed to generate refresh token",
		})
		return
	}

	// Save new refresh token
	expiresAt := auth.GetRefreshTokenExpiry(&h.config.JWT)
	if err := db.DB.SaveRefreshToken(user.ID, newRefreshToken, expiresAt); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Failed to save refresh token",
		})
		return
	}

	// Set new refresh token cookie with SameSite=Strict
	maxAge := int(time.Until(expiresAt).Seconds())
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(
		"refresh_token",
		newRefreshToken,
		maxAge,
		"/api/v1/auth",
		"",
		true,
		true,
	)

	c.JSON(http.StatusOK, models.LoginResponse{
		AccessToken: accessToken,
		ExpiresIn:   h.config.JWT.AccessExpiryMinutes * 60,
		APIToken:    user.APIToken,
	})
}

// Logout invalidates the refresh token
func (h *AuthHandler) Logout(c *gin.Context) {
	refreshToken, err := c.Cookie("refresh_token")
	if err == nil {
		// Delete token from database
		db.DB.DeleteRefreshToken(refreshToken)
	}

	// Clear the cookie with SameSite=Strict
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(
		"refresh_token",
		"",
		-1,
		"/api/v1/auth",
		"",
		true,
		true,
	)

	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}
