package response

import "github.com/gin-gonic/gin"

func OK(c *gin.Context, data any) {
	c.JSON(200, gin.H{"ok": true, "data": data})
}

func Fail(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"ok": false, "message": msg})
}
