package admin

// roles.go — 角色管理（CRUD + 分配给用户）
//
// GET    /api/admin/roles                    — 列出所有角色
// POST   /api/admin/roles                    — 创建角色
// PUT    /api/admin/roles/:id                — 更新角色
// DELETE /api/admin/roles/:id                — 删除角色
// POST   /api/admin/users/:id/assign-role    — 为用户分配角色

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"nmegateway/internal/model"
	"nmegateway/internal/pkg/response"
)

// CreateRole POST /api/admin/roles
func (h *Handler) CreateRole(c *gin.Context) {
	var req struct {
		Name        string          `json:"name"`
		DisplayName string          `json:"display_name"`
		Description string          `json:"description"`
		Permissions map[string]bool `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		response.Fail(c, http.StatusBadRequest, "name required")
		return
	}
	permsJSON := "{}"
	if len(req.Permissions) > 0 {
		if b, err := json.Marshal(req.Permissions); err == nil {
			permsJSON = string(b)
		}
	}
	role := model.Role{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Permissions: permsJSON,
	}
	if err := h.DB.Create(&role).Error; err != nil {
		response.Fail(c, http.StatusBadRequest, "create role failed: "+err.Error())
		return
	}
	response.OK(c, role)
}

// UpdateRole PUT /api/admin/roles/:id
func (h *Handler) UpdateRole(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req struct {
		DisplayName string          `json:"display_name"`
		Description string          `json:"description"`
		Permissions map[string]bool `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid payload")
		return
	}
	updates := map[string]any{}
	if req.DisplayName != "" {
		updates["display_name"] = req.DisplayName
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.Permissions != nil {
		if b, err := json.Marshal(req.Permissions); err == nil {
			updates["permissions"] = string(b)
		}
	}
	if len(updates) == 0 {
		response.Fail(c, http.StatusBadRequest, "nothing to update")
		return
	}
	if res := h.DB.Model(&model.Role{}).Where("id = ?", id).Updates(updates); res.RowsAffected == 0 {
		response.Fail(c, http.StatusNotFound, "role not found")
		return
	}
	response.OK(c, gin.H{"id": id, "updated": true})
}

// DeleteRole DELETE /api/admin/roles/:id
func (h *Handler) DeleteRole(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	// 禁止删除内置角色（super_admin / admin / user）
	var role model.Role
	if err := h.DB.First(&role, id).Error; err != nil {
		response.Fail(c, http.StatusNotFound, "role not found")
		return
	}
	built := map[string]bool{"super_admin": true, "admin": true, "user": true}
	if built[role.Name] {
		response.Fail(c, http.StatusForbidden, "cannot delete built-in role: "+role.Name)
		return
	}
	if err := h.DB.Delete(&model.Role{}, id).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "delete failed")
		return
	}
	response.OK(c, gin.H{"id": id, "deleted": true})
}

// AssignRole POST /api/admin/users/:id/assign-role
// Body: {"role": "admin"}  — 直接按 role 字符串分配（NMEGateway 用 User.Role 字段，不走 user_roles 关联表）
func (h *Handler) AssignRole(c *gin.Context) {
	userID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req struct {
		Role string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Role == "" {
		response.Fail(c, http.StatusBadRequest, "role required")
		return
	}
	// 校验角色存在
	var cnt int64
	h.DB.Model(&model.Role{}).Where("name = ?", req.Role).Count(&cnt)
	// 内置角色不在 roles 表时也允许
	built := map[string]bool{"super_admin": true, "admin": true, "user": true}
	if cnt == 0 && !built[req.Role] {
		response.Fail(c, http.StatusBadRequest, "role not found: "+req.Role)
		return
	}
	res := h.DB.Model(&model.User{}).Where("id = ?", userID).Update("role", req.Role)
	if res.RowsAffected == 0 {
		response.Fail(c, http.StatusNotFound, "user not found")
		return
	}
	response.OK(c, gin.H{"user_id": userID, "role": req.Role})
}
