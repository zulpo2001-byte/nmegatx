package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func Role(roles ...string) gin.HandlerFunc {
	allowed := map[string]struct{}{}
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		roleStr, ok := role.(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"ok": false, "message": "forbidden"})
			return
		}
		if _, ok := allowed[roleStr]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"ok": false, "message": "forbidden"})
			return
		}
		c.Next()
	}
}
