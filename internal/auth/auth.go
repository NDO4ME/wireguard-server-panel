package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ndo4me/wireguard-server-panel/internal/config"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidToken    = errors.New("invalid token")
	ErrExpiredToken    = errors.New("token has expired")
	ErrInvalidPassword = errors.New("invalid password")
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
			Issuer:    "ndo4me",
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

// GenerateAPIToken creates a random 43-character API token (256 bits of entropy).
// It is independent of the admin password, so it cannot be derived or brute-forced from it.
func GenerateAPIToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
