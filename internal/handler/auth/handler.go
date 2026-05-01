package auth

import (
	"crypto/rand"
	"encoding/json"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"nmegateway/internal/model"
	jwtutil "nmegateway/internal/pkg/jwt"
	"nmegateway/internal/pkg/response"
	"nmegateway/internal/service"
)

func isExpiredUser(u model.User) bool {
	return !u.ExpiresAt.IsZero() && u.ExpiresAt.Before(time.Now().UTC())
}

type Handler struct {
	DB             *gorm.DB
	JWTSecret      string
	JWTHours       int
	JWTRefreshDays int
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid payload")
		return
	}
	var u model.User
	if err := h.DB.Where("email = ?", req.Email).First(&u).Error; err != nil {
		response.Fail(c, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if u.Status != "active" {
		response.Fail(c, http.StatusForbidden, "user inactive")
		return
	}
	if isExpiredUser(u) {
		response.Fail(c, http.StatusForbidden, "user expired")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(req.Password)) != nil {
		response.Fail(c, http.StatusUnauthorized, "invalid credentials")
		return
	}
	perms := parsePermissions(u.Role, u.Permissions)
	token, err := jwtutil.Sign(h.JWTSecret, u.ID, u.Role, perms, h.JWTHours)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to sign token")
		return
	}
	refresh := mustToken()
	_, _ = service.SaveRefreshToken(h.DB, u.ID, refresh, h.JWTRefreshDays)
	response.OK(c, gin.H{
		"access_token":  token,
		"refresh_token": refresh,
		"token_type":    "Bearer",
		"expires_hours": h.JWTHours,
	})
}

func (h *Handler) Me(c *gin.Context) {
	perms, _ := c.Get("permissions")
	response.OK(c, gin.H{
		"user_id":     c.GetUint("user_id"),
		"role":        c.GetString("role"),
		"permissions": perms,
	})
}

func (h *Handler) Refresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.RefreshToken == "" {
		response.Fail(c, http.StatusBadRequest, "invalid payload")
		return
	}

	var rt model.RefreshToken
	if err := h.DB.Where("token_hash = ? AND revoked_at IS NULL", service.HashRefreshToken(req.RefreshToken)).First(&rt).Error; err != nil {
		response.Fail(c, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	if rt.ExpiresAt.Before(time.Now()) {
		response.Fail(c, http.StatusUnauthorized, "refresh token expired")
		return
	}
	var u model.User
	if err := h.DB.First(&u, rt.UserID).Error; err != nil {
		response.Fail(c, http.StatusUnauthorized, "user not found")
		return
	}
	if u.Status != "active" {
		response.Fail(c, http.StatusForbidden, "user inactive")
		return
	}
	if isExpiredUser(u) {
		response.Fail(c, http.StatusForbidden, "user expired")
		return
	}
	perms := parsePermissions(u.Role, u.Permissions)
	token, err := jwtutil.Sign(h.JWTSecret, rt.UserID, u.Role, perms, h.JWTHours)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to sign token")
		return
	}
	response.OK(c, gin.H{"access_token": token, "token_type": "Bearer", "expires_hours": h.JWTHours})
}

func (h *Handler) Logout(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.RefreshToken != "" {
		_ = service.RevokeRefreshToken(h.DB, req.RefreshToken)
	}
	response.OK(c, gin.H{"message": "logged out"})
}

func mustToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func parsePermissions(role, raw string) map[string]bool {
	if role == "super_admin" {
		return map[string]bool{"all": true}
	}
	parsed := map[string]bool{}
	if strings.TrimSpace(raw) == "" {
		return parsed
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return parsed
	}
	return parsed
}
