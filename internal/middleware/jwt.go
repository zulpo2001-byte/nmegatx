package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	jwtutil "nme-v9/internal/pkg/jwt"
)

func JWT(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "missing bearer token"})
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		claims, err := jwtutil.Parse(secret, token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "invalid token"})
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)
		c.Set("permissions", claims.Permissions)
		c.Next()
	}
}
