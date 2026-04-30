package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func UserPerm(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if role == "super_admin" {
			c.Next()
			return
		}
		raw, ok := c.Get("permissions")
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"ok": false, "message": "permission denied"})
			return
		}
		perms, ok := raw.(map[string]bool)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"ok": false, "message": "permission denied"})
			return
		}
		if perms["all"] || perms[perm] {
			c.Next()
			return
		}
		// 兼容老权限键：products_manage -> paypal_manage/stripe_manage
		if (perm == "paypal_manage" || perm == "stripe_manage") && perms["products_manage"] {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"ok": false, "message": "permission denied"})
	}
}
