package router_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"nme-v9/internal/config"
	"nme-v9/internal/model"
	hmacutil "nme-v9/internal/pkg/hmac"
	"nme-v9/internal/router"
)

func TestSmokeCoreFlows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.APIKey{}, &model.Product{}, &model.Order{},
		&model.WebhookEndpoint{}, &model.RefreshToken{},
		&model.Role{}, &model.RiskRule{}, &model.AlertRecord{}, &model.AlertChannel{}, &model.GlobalSetting{}, &model.AuditLog{},
		&model.PaypalAccount{}, &model.StripeConfig{}, &model.ChannelMetric{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	admin := model.User{Email: "admin@test.local", Password: "123456", Role: "admin", Status: "active", Permissions: `{"admin.stats.view":true,"admin.users.view":true}`}
	user := model.User{Email: "user@test.local", Password: "123456", Role: "user", Status: "active", Permissions: `{"dashboard_view":true,"products_manage":true,"order_view":true,"api_keys":true,"webhooks":true}`}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	key := model.APIKey{UserID: user.ID, APIKey: "ak_user_test", Secret: "sk_user_test", Enabled: true}
	if err := db.Create(&key).Error; err != nil {
		t.Fatalf("seed key: %v", err)
	}
	prod := model.Product{UserID: user.ID, BProductID: "B-PROD-1", Weight: 1, PollOrder: 1, Enabled: true}
	if err := db.Create(&prod).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}

	cfg := &config.Config{
		JWTSecret:         "test-jwt-secret",
		JWTExpiresHours:   24,
		JWTRefreshDays:    30,
		HMACWindowSeconds: 300,
		CORSAllowOrigins:  []string{"http://localhost"},
	}
	r := router.New(cfg, db, nil, nil, nil)

	adminToken := loginAndGetToken(t, r, "admin@test.local", "123456")
	userToken := loginAndGetToken(t, r, "user@test.local", "123456")

	// Admin route should work with admin token.
	resp := callJSON(t, r, "GET", "/api/admin/stats", nil, map[string]string{"Authorization": "Bearer " + adminToken})
	if !resp["ok"].(bool) {
		t.Fatalf("admin stats should pass: %#v", resp)
	}

	// Admin route should fail with user token.
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for user on admin route, got %d", w.Code)
	}

	// User products route should pass with user token.
	resp = callJSON(t, r, "GET", "/api/user/products", nil, map[string]string{"Authorization": "Bearer " + userToken})
	if !resp["ok"].(bool) {
		t.Fatalf("user products should pass: %#v", resp)
	}

	// Gateway order.
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	orderReq := map[string]any{
		"a_order_id":   "A-10001",
		"amount":       188.5,
		"return_url":   "https://a.local/thanks",
		"checkout_url": "https://a.local/checkout",
	}
	raw := "A-10001|" + ts + "|" + key.APIKey
	sig := hmacutil.Sign(key.Secret, raw)
	resp = callJSON(t, r, "POST", "/api/gateway/order", orderReq, map[string]string{
		"X-Api-Key":    key.APIKey,
		"X-Timestamp":  ts,
		"X-Signature":  sig,
		"Content-Type": "application/json",
	})
	if !resp["ok"].(bool) {
		t.Fatalf("gateway order failed: %#v", resp)
	}
	data := resp["data"].(map[string]any)
	payToken := data["pay_token"].(string)

	// Gateway callback complete.
	ts2 := strconv.FormatInt(time.Now().Unix(), 10)
	callbackReq := map[string]any{"pay_token": payToken, "status": "completed"}
	raw2 := payToken + "|completed|" + ts2 + "|" + key.APIKey
	sig2 := hmacutil.Sign(key.Secret, raw2)
	resp = callJSON(t, r, "POST", "/api/gateway/callback", callbackReq, map[string]string{
		"X-Api-Key":    key.APIKey,
		"X-Timestamp":  ts2,
		"X-Signature":  sig2,
		"Content-Type": "application/json",
	})
	if !resp["ok"].(bool) {
		t.Fatalf("gateway callback failed: %#v", resp)
	}

	// Status query.
	resp = callJSON(t, r, "GET", "/api/gateway/status/"+payToken, nil, nil)
	if !resp["ok"].(bool) {
		t.Fatalf("status query failed: %#v", resp)
	}
	sData := resp["data"].(map[string]any)
	if sData["status"] != "completed" {
		t.Fatalf("expected completed, got %#v", sData["status"])
	}
}

func loginAndGetToken(t *testing.T, r http.Handler, email, password string) string {
	t.Helper()
	resp := callJSON(t, r, "POST", "/api/auth/login", map[string]any{
		"email":    email,
		"password": password,
	}, map[string]string{"Content-Type": "application/json"})
	if !resp["ok"].(bool) {
		t.Fatalf("login failed: %#v", resp)
	}
	data := resp["data"].(map[string]any)
	return data["access_token"].(string)
}

func callJSON(t *testing.T, r http.Handler, method, path string, payload any, headers map[string]string) map[string]any {
	t.Helper()
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	r.ServeHTTP(w, req)
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response(%d): %s", w.Code, w.Body.String())
	}
	return resp
}
