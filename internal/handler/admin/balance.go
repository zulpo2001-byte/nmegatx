package admin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"nme-v9/internal/model"
	"nme-v9/internal/pkg/response"
)

// RechargeUserBalance POST /api/admin/users/:id/recharge
func (h *Handler) RechargeUserBalance(c *gin.Context) {
	uid, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req struct {
		AmountUSD float64 `json:"amount_usd"`
		Note      string  `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.AmountUSD <= 0 {
		response.Fail(c, http.StatusBadRequest, "amount_usd must be > 0")
		return
	}
	var user model.User
	if err := h.DB.First(&user, uid).Error; err != nil {
		response.Fail(c, http.StatusNotFound, "user not found")
		return
	}
	newBalance := user.BalanceUSD + req.AmountUSD
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.User{}).Where("id = ?", uid).Update("balance_usd", newBalance).Error; err != nil {
			return err
		}
		entry := model.BalanceLedger{UserID: uint(uid), Type: "recharge", AmountUSD: req.AmountUSD, BalanceUSD: newBalance, Note: req.Note}
		return tx.Create(&entry).Error
	}); err != nil {
		response.Fail(c, http.StatusInternalServerError, "recharge failed")
		return
	}
	response.OK(c, gin.H{"user_id": uid, "balance_usd": newBalance})
}

// UserBalanceRecords GET /api/admin/users/:id/balance-records
func (h *Handler) UserBalanceRecords(c *gin.Context) {
	uid, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var rows []model.BalanceLedger
	if err := h.DB.Where("user_id = ?", uid).Order("id desc").Limit(500).Find(&rows).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "query records failed")
		return
	}
	response.OK(c, gin.H{"items": rows, "count": len(rows)})
}
