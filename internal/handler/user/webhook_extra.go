package user

// webhook_extra.go — Webhook 端点密钥轮换 + 测试推送
//
// POST /api/user/webhooks/:id/regenerate-a-secret      — 轮换 A站 shared_secret（HMAC 密钥）
// POST /api/user/webhooks/:id/regenerate-a-api-key     — 轮换 A站 a_api_key（标识符）
// POST /api/user/webhooks/:id/regenerate-b-secret      — 轮换 B站 b_shared_secret（HMAC 密钥）
// POST /api/user/webhooks/:id/regenerate-b-api-key     — 轮换 B站 b_api_key（标识符）
// POST /api/user/webhooks/:id/test                     — 向端点 URL 发送测试请求

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"nme-v9/internal/model"
	"nme-v9/internal/pkg/response"
)

// RegenerateASecret POST /api/user/webhooks/:id/regenerate-a-secret
// 轮换 A站 HMAC 签名密钥（shared_secret），旧密钥立即失效
func (h *Handler) RegenerateASecret(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var ep model.WebhookEndpoint
	if err := h.DB.Where("id = ? AND user_id = ? AND type = 'a'", id, userID).First(&ep).Error; err != nil {
		response.Fail(c, http.StatusNotFound, "webhook endpoint not found")
		return
	}

	newSecret := "whsec_" + randHex(20)
	h.DB.Model(&ep).Update("shared_secret", newSecret)

	// 返回新的 config_string，方便用户直接粘贴到插件
	configString := buildConfigString("a", h.BaseURL, map[string]string{
		"ak":    ep.AApiKey,
		"whsec": newSecret,
	})
	response.OK(c, gin.H{
		"id":             ep.ID,
		"shared_secret":  newSecret,
		"config_string":  configString,
		"warning":        "旧密钥已立即失效，请尽快更新 A站插件配置",
	})
}

// RegenerateAAApiKey POST /api/user/webhooks/:id/regenerate-a-api-key
// 轮换 A站 X-Api-Key 标识符
func (h *Handler) RegenerateAAApiKey(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var ep model.WebhookEndpoint
	if err := h.DB.Where("id = ? AND user_id = ? AND type = 'a'", id, userID).First(&ep).Error; err != nil {
		response.Fail(c, http.StatusNotFound, "webhook endpoint not found")
		return
	}

	newKey := "ak_cb_" + randHex(16)
	h.DB.Model(&ep).Update("a_api_key", newKey)

	configString := buildConfigString("a", h.BaseURL, map[string]string{
		"ak":    newKey,
		"whsec": ep.SharedSecret,
	})
	response.OK(c, gin.H{
		"id":            ep.ID,
		"a_api_key":     newKey,
		"config_string": configString,
		"warning":       "旧 API Key 已立即失效，请尽快更新 A站插件配置",
	})
}

// RegenerateBSecret POST /api/user/webhooks/:id/regenerate-b-secret
// 轮换 B站 HMAC 签名密钥（b_shared_secret）
func (h *Handler) RegenerateBSecret(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var ep model.WebhookEndpoint
	if err := h.DB.Where("id = ? AND user_id = ? AND type = 'b'", id, userID).First(&ep).Error; err != nil {
		response.Fail(c, http.StatusNotFound, "webhook endpoint not found")
		return
	}

	newSecret := "bsk_" + randHex(20)
	h.DB.Model(&ep).Update("b_shared_secret", newSecret)

	configString := buildConfigString("b", h.BaseURL, map[string]string{
		"bk":  ep.BApiKey,
		"bsk": newSecret,
	})
	response.OK(c, gin.H{
		"id":              ep.ID,
		"b_shared_secret": newSecret,
		"config_string":   configString,
		"warning":         "旧 B站密钥已立即失效，请尽快更新 B站插件配置",
	})
}

// RegenerateBApiKey POST /api/user/webhooks/:id/regenerate-b-api-key
// 轮换 B站 X-Api-Key 标识符（b_api_key）
func (h *Handler) RegenerateBApiKey(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var ep model.WebhookEndpoint
	if err := h.DB.Where("id = ? AND user_id = ? AND type = 'b'", id, userID).First(&ep).Error; err != nil {
		response.Fail(c, http.StatusNotFound, "webhook endpoint not found")
		return
	}

	newKey := "bk_live_" + randHex(16)
	h.DB.Model(&ep).Update("b_api_key", newKey)

	configString := buildConfigString("b", h.BaseURL, map[string]string{
		"bk":  newKey,
		"bsk": ep.BSharedSecret,
	})
	response.OK(c, gin.H{
		"id":            ep.ID,
		"b_api_key":     newKey,
		"config_string": configString,
		"warning":       "旧 B站 API Key 已立即失效，请尽快更新 B站插件配置",
	})
}

// TestWebhook POST /api/user/webhooks/:id/test
// 向端点配置的 URL 发送一条测试 JSON 请求，验证连通性
func (h *Handler) TestWebhook(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)

	var ep model.WebhookEndpoint
	if err := h.DB.Where("id = ? AND user_id = ?", id, userID).First(&ep).Error; err != nil {
		response.Fail(c, http.StatusNotFound, "webhook endpoint not found")
		return
	}
	if ep.URL == "" {
		response.Fail(c, http.StatusBadRequest, "endpoint URL is empty")
		return
	}

	payload := map[string]any{
		"event":      "test",
		"message":    "NMEGateway webhook test — if you see this, the endpoint is reachable",
		"endpoint_id": ep.ID,
		"sent_at":    time.Now().UTC().Format(time.RFC3339),
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, ep.URL, bytes.NewReader(body))
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid endpoint URL: "+err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-NME-Event", "test")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		response.OK(c, gin.H{
			"success":      false,
			"error":        err.Error(),
			"endpoint_url": ep.URL,
		})
		return
	}
	defer resp.Body.Close()

	response.OK(c, gin.H{
		"success":      resp.StatusCode >= 200 && resp.StatusCode < 300,
		"status_code":  resp.StatusCode,
		"endpoint_url": ep.URL,
	})
}
