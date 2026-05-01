package admin

// user.go — 用户管理 CRUD（保留原有逻辑，结构体定义已移至 all.go）

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"nmegateway/internal/model"
	"nmegateway/internal/pkg/response"
)

// Users GET /api/admin/users
func (h *Handler) Users(c *gin.Context) {
	status := c.Query("status")
	query := h.DB.Order("id desc").Limit(200)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var users []model.User
	if err := query.Find(&users).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "failed to query users")
		return
	}
	response.OK(c, gin.H{"items": users})
}

// CreateUser POST /api/admin/users
func (h *Handler) CreateUser(c *gin.Context) {
	var req struct {
		Email       string          `json:"email"`
		Password    string          `json:"password"`
		Role        string          `json:"role"`
		Status      string          `json:"status"`
		Permissions map[string]bool `json:"permissions"`
		ExpiresAt   string          `json:"expires_at"`
		BalanceUSD    *float64      `json:"balance_usd"`
		PaypalFeeRate *float64      `json:"paypal_fee_rate"`
		StripeFeeRate *float64      `json:"stripe_fee_rate"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Email == "" || req.Password == "" {
		response.Fail(c, http.StatusBadRequest, "email and password required")
		return
	}
	if req.Role == "" {
		req.Role = "user"
	}
	if req.Status == "" {
		req.Status = "active"
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "password hash failed")
		return
	}

	// 序列化 permissions
	permsJSON := "{}"
	if len(req.Permissions) > 0 {
		if s, err2 := marshalMapBool(req.Permissions); err2 == nil {
			permsJSON = s
		}
	}

	u := model.User{
		Email:       req.Email,
		Role:        req.Role,
		Status:      req.Status,
		Password:    string(hash),
		Permissions: permsJSON,
	}
	if req.BalanceUSD != nil {
		u.BalanceUSD = *req.BalanceUSD
	}
	if req.PaypalFeeRate != nil {
		u.PaypalFeeRate = *req.PaypalFeeRate
	}
	if req.StripeFeeRate != nil {
		u.StripeFeeRate = *req.StripeFeeRate
	}
	if req.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, req.ExpiresAt); err == nil {
			u.ExpiresAt = t
		}
	}
	if err := h.DB.Create(&u).Error; err != nil {
		response.Fail(c, http.StatusBadRequest, "create user failed: "+err.Error())
		return
	}
	u.Password = ""
	response.OK(c, u)
}

// UpdateUser PUT /api/admin/users/:id
func (h *Handler) UpdateUser(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req struct {
		Role      string `json:"role"`
		Status    string `json:"status"`
		Password  string `json:"password"`
		ExpiresAt string `json:"expires_at"`
		PaypalFeeRate *float64 `json:"paypal_fee_rate"`
		StripeFeeRate *float64 `json:"stripe_fee_rate"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid payload")
		return
	}
	updates := map[string]any{}
	if req.Role != "" {
		updates["role"] = req.Role
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			response.Fail(c, http.StatusInternalServerError, "password hash failed")
			return
		}
		updates["password"] = string(hash)
	}
	if req.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, req.ExpiresAt); err == nil {
			updates["expires_at"] = t
		}
	}
	if req.PaypalFeeRate != nil {
		updates["paypal_fee_rate"] = *req.PaypalFeeRate
	}
	if req.StripeFeeRate != nil {
		updates["stripe_fee_rate"] = *req.StripeFeeRate
	}
	if len(updates) == 0 {
		response.Fail(c, http.StatusBadRequest, "nothing to update")
		return
	}
	if err := h.DB.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "update failed")
		return
	}
	response.OK(c, gin.H{"id": id, "updated": true})
}

// DeleteUser DELETE /api/admin/users/:id
func (h *Handler) DeleteUser(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.DB.Delete(&model.User{}, id).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "delete failed")
		return
	}
	response.OK(c, gin.H{"id": id, "deleted": true})
}

// UpdateSelfPassword PUT /api/profile/password（通用，admin 和 user 均可）
// super_admin 可同时修改邮箱；user 只能改密码
func (h *Handler) UpdateSelfPassword(c *gin.Context) {
	selfID := c.GetUint("user_id")
	role := c.GetString("role")
	var req struct {
		Email       string `json:"email"`
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid payload")
		return
	}
	if role != "super_admin" && req.Email != "" {
		response.Fail(c, http.StatusForbidden, "only super_admin can change email")
		return
	}
	if req.NewPassword == "" && req.Email == "" {
		response.Fail(c, http.StatusBadRequest, "nothing to update")
		return
	}

	var user model.User
	if err := h.DB.First(&user, selfID).Error; err != nil {
		response.Fail(c, http.StatusNotFound, "user not found")
		return
	}
	if req.NewPassword != "" {
		if req.OldPassword == "" {
			response.Fail(c, http.StatusBadRequest, "old_password required to change password")
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)) != nil {
			response.Fail(c, http.StatusUnauthorized, "old password incorrect")
			return
		}
	}

	updates := map[string]any{}
	if req.Email != "" && req.Email != user.Email {
		var cnt int64
		h.DB.Model(&model.User{}).Where("email = ? AND id != ?", req.Email, selfID).Count(&cnt)
		if cnt > 0 {
			response.Fail(c, http.StatusBadRequest, "email already in use")
			return
		}
		updates["email"] = req.Email
	}
	if req.NewPassword != "" {
		hash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		updates["password"] = string(hash)
	}
	if len(updates) == 0 {
		response.OK(c, gin.H{"updated": false, "message": "nothing changed"})
		return
	}
	h.DB.Model(&model.User{}).Where("id = ?", selfID).Updates(updates)
	response.OK(c, gin.H{"updated": true})
}

// SubAccountsAmount GET /api/admin/users/sub-accounts-amount
// 汇总每个用户的今日流水 + 历史总流水
func (h *Handler) SubAccountsAmount(c *gin.Context) {
	type row struct {
		ID          uint    `json:"id"`
		Email       string  `json:"email"`
		Status      string  `json:"status"`
		DailyAmount float64 `json:"daily_amount"`
		TotalAmount float64 `json:"total_amount"`
	}

	var users []model.User
	h.DB.Select("id, email, status").Find(&users)

	var rows []row
	var grandDaily, grandTotal float64

	for _, u := range users {
		var daily, total float64
		h.DB.Raw(`
			SELECT COALESCE(SUM(daily_amount),0) FROM paypal_accounts WHERE user_id=?
		`, u.ID).Scan(&daily)
		var stripeDly float64
		h.DB.Raw(`
			SELECT COALESCE(SUM(daily_amount),0) FROM stripe_configs WHERE user_id=?
		`, u.ID).Scan(&stripeDly)
		daily += stripeDly

		h.DB.Model(&model.Order{}).
			Where("user_id = ? AND status = ?", u.ID, "completed").
			Select("COALESCE(SUM(amount),0)").Scan(&total)

		rows = append(rows, row{
			ID:          u.ID,
			Email:       u.Email,
			Status:      u.Status,
			DailyAmount: daily,
			TotalAmount: total,
		})
		grandDaily += daily
		grandTotal += total
	}

	response.OK(c, gin.H{
		"users":              rows,
		"grand_daily_amount": grandDaily,
		"grand_total_amount": grandTotal,
	})
}
