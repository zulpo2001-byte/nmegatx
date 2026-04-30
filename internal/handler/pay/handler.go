package pay

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct{}

// Result serves the payment result HTML page.
// The frontend (pay.html) reads token/status from query params and polls
// /api/gateway/status/:token to get the real order state, then auto-redirects.
func (h *Handler) Result(c *gin.Context) {
	c.File("./frontend/pay.html")
}

func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
