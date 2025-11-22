package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"wgeasygo/internal/config"
)

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrExpiredToken     = errors.New("token has expired")
	ErrInvalidPassword  = errors.New("invalid password")
)

type Claims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// HashPassword creates a bcrypt hash of the password with the configured cost
func HashPassword(password string, cost int) (string, error) {
	if cost < 12 {
		cost = 12 // Minimum recommended cost
	}
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// VerifyPassword compares a password with its hash
func VerifyPassword(password, hash string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return ErrInvalidPassword
	}
	return nil
}

// GenerateAccessToken creates a short-lived JWT access token
func GenerateAccessToken(userID int64, username string, cfg *config.JWTConfig) (string, error) {
	expirationTime := time.Now().Add(time.Duration(cfg.AccessExpiryMinutes) * time.Minute)

	claims := &Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "wgeasygo",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.AccessSecret))
}

// GenerateRefreshToken creates a cryptographically secure random token
func GenerateRefreshToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// ValidateAccessToken verifies and parses the access token
func ValidateAccessToken(tokenString string, cfg *config.JWTConfig) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return []byte(cfg.AccessSecret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// GetRefreshTokenExpiry returns the expiration time for refresh tokens
func GetRefreshTokenExpiry(cfg *config.JWTConfig) time.Time {
	return time.Now().Add(time.Duration(cfg.RefreshExpiryDays) * 24 * time.Hour)
}

// GenerateAPIToken creates a deterministic 43-character API token from password
// Token stays constant unless password changes - uses fixed salt so same password = same token across instances
func GenerateAPIToken(password string, _ string) string {
	// Use fixed salt so token is deterministic based only on password
	// This allows same password to generate same token on different servers/restarts
	fixedSalt := "wireguard-panel-api-token-salt-v1"

	// Use HMAC-SHA256 with password and fixed salt
	h := hmac.New(sha256.New, []byte(fixedSalt))
	h.Write([]byte(password))
	hash := h.Sum(nil)

	// Encode to base64url (URL-safe base64 without padding)
	token := base64.RawURLEncoding.EncodeToString(hash)

	// Ensure exactly 43 characters
	if len(token) > 43 {
		token = token[:43]
	}

	return token
}
