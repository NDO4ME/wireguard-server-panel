package handlers

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/ndo4me/wireguard-server-panel/internal/db"
	"github.com/ndo4me/wireguard-server-panel/internal/models"
)

// Login page white-labelling. The login page ships without any brand; the admin
// can optionally set a logo, a heading, a browser tab title and an accent colour.
const (
	maxLoginTextRunes = 60
	maxLoginLogoBytes = 256 * 1024
)

var accentRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// PublicLoginPage returns the login page appearance. It is public (needed before
// sign-in) and exposes nothing but what the admin chose to show there.
func PublicLoginPage(c *gin.Context) {
	s, err := db.DB.GetAllSettings()
	if err != nil {
		s = map[string]string{}
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, models.LoginPage{
		Title:   s["login_title"],
		Heading: s["login_heading"],
		Logo:    s["login_logo"],
		Accent:  s["login_accent"],
	})
}

func validLoginText(v string) bool {
	if utf8.RuneCountInString(v) > maxLoginTextRunes {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// validLoginLogo accepts a base64 data URL of a small raster image or SVG.
func validLoginLogo(v string) error {
	if v == "" {
		return nil
	}
	const marker = ";base64,"
	i := strings.Index(v, marker)
	if !strings.HasPrefix(v, "data:image/") || i < 0 {
		return fmt.Errorf("logo must be a PNG, JPEG, WebP, GIF or SVG image")
	}
	mime := v[len("data:"):i]
	raw, err := base64.StdEncoding.DecodeString(v[i+len(marker):])
	if err != nil {
		return fmt.Errorf("logo is not valid base64")
	}
	if len(raw) == 0 || len(raw) > maxLoginLogoBytes {
		return fmt.Errorf("logo must be smaller than %d KB", maxLoginLogoBytes/1024)
	}
	ok := false
	switch mime {
	case "image/png":
		ok = bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n"))
	case "image/jpeg":
		ok = bytes.HasPrefix(raw, []byte{0xFF, 0xD8, 0xFF})
	case "image/gif":
		ok = bytes.HasPrefix(raw, []byte("GIF8"))
	case "image/webp":
		ok = len(raw) > 12 && string(raw[:4]) == "RIFF" && string(raw[8:12]) == "WEBP"
	case "image/svg+xml":
		l := strings.ToLower(string(raw))
		ok = strings.Contains(l, "<svg") && !strings.Contains(l, "<script") && !strings.Contains(l, "foreignobject")
	}
	if !ok {
		return fmt.Errorf("logo content does not match its image type")
	}
	return nil
}

// saveLoginPage validates every provided field first and stores them together.
func saveLoginPage(req *models.UpdateSettingsRequest) error {
	if req.LoginTitle != nil && !validLoginText(*req.LoginTitle) {
		return fmt.Errorf("tab title must be at most %d characters", maxLoginTextRunes)
	}
	if req.LoginHeading != nil && !validLoginText(*req.LoginHeading) {
		return fmt.Errorf("heading must be at most %d characters", maxLoginTextRunes)
	}
	if req.LoginAccent != nil && *req.LoginAccent != "" && !accentRe.MatchString(*req.LoginAccent) {
		return fmt.Errorf("accent must be a #rrggbb colour")
	}
	if req.LoginLogo != nil {
		if err := validLoginLogo(*req.LoginLogo); err != nil {
			return err
		}
	}
	for key, val := range map[string]*string{
		"login_title":   req.LoginTitle,
		"login_heading": req.LoginHeading,
		"login_logo":    req.LoginLogo,
		"login_accent":  req.LoginAccent,
	} {
		if val == nil {
			continue
		}
		if err := db.DB.SetSetting(key, strings.TrimSpace(*val)); err != nil {
			return fmt.Errorf("failed to save login page settings")
		}
	}
	return nil
}
