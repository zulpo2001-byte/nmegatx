package router_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"nmegateway/internal/config"
	"nmegateway/internal/model"
	hmacutil "nmegateway/internal/pkg/hmac"
	"nmegateway/internal/router"
)

func TestSmokeCoreFlows(t *testing.T) {
	bStation := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/wp-json/b-station/v1/order" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"data":{"payment_url":"https://b.local/pay/abc","b_order_id":"B-10001"}}`))
	}))
	defer bStation.Close()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.APIKey{}, &model.Order{},
		&model.BalanceLedger{},
		&model.WebhookEndpoint{}, &model.RefreshToken{},
		&model.Role{}, &model.RiskRule{}, &model.AlertRecord{}, &model.AlertChannel{}, &model.GlobalSetting{}, &model.AuditLog{},
		&model.PaypalAccount{}, &model.StripeConfig{}, &model.ChannelMetric{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	adminPwd, _ := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	userPwd, _ := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	admin := model.User{Email: "admin@test.local", Password: string(adminPwd), Role: "admin", Status: "active", Permissions: `{"admin.stats.view":true,"admin.users.view":true}`}
	user := model.User{Email: "user@test.local", Password: string(userPwd), Role: "user", Status: "active", Permissions: `{"dashboard_view":true,"paypal_manage":true,"stripe_manage":true,"order_view":true,"webhooks":true}`}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	aWebhook := model.WebhookEndpoint{
		UserID:        user.ID,
		Type:          "a",
		Label:         "a-endpoint",
		URL:           "https://a.local/callback",
		PaymentMethod: "all",
		Enabled:       true,
		AApiKey:       "ak_user_test",
		SharedSecret:  "sk_user_test",
	}
	if err := db.Create(&aWebhook).Error; err != nil {
		t.Fatalf("seed a webhook: %v", err)
	}
	bWebhook := model.WebhookEndpoint{
		UserID:        user.ID,
		Type:          "b",
		Label:         "b-endpoint",
		URL:           bStation.URL,
		PaymentMethod: "all",
		Enabled:       true,
		BApiKey:       "bk_user_test",
		BSharedSecret: "bsk_user_test",
	}
	if err := db.Create(&bWebhook).Error; err != nil {
		t.Fatalf("seed b webhook: %v", err)
	}
	stripe := model.StripeConfig{
		UserID:       user.ID,
		Label:        "stripe-main",
		SecretKey:    "sk_test_x",
		AccountState: "active",
		Enabled:      true,
	}
	if err := db.Create(&stripe).Error; err != nil {
		t.Fatalf("seed stripe: %v", err)
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

	// Gateway order.
	orderReq := map[string]any{
		"a_order_id":   "A-10001",
		"amount":       188.5,
		"return_url":   "https://a.local/thanks",
		"checkout_url": "https://a.local/checkout",
	}
	orderBody, _ := json.Marshal(orderReq)
	orderHeaders := hmacutil.BuildHeaders(aWebhook.AApiKey, aWebhook.SharedSecret, orderBody)
	orderHeaders["Content-Type"] = "application/json"
	resp = callJSON(t, r, "POST", "/api/gateway/order", orderReq, map[string]string{
		"X-Api-Key":    orderHeaders["X-Api-Key"],
		"X-Timestamp":  orderHeaders["X-Timestamp"],
		"X-Signature":  orderHeaders["X-Signature"],
		"Content-Type": orderHeaders["Content-Type"],
	})
	if !resp["ok"].(bool) {
		t.Fatalf("gateway order failed: %#v", resp)
	}
	data := resp["data"].(map[string]any)
	payToken := data["pay_token"].(string)

	// Gateway callback complete.
	callbackReq := map[string]any{"pay_token": payToken, "status": "completed"}
	callbackBody, _ := json.Marshal(callbackReq)
	callbackHeaders := hmacutil.BuildHeaders(bWebhook.BApiKey, bWebhook.BSharedSecret, callbackBody)
	callbackHeaders["Content-Type"] = "application/json"
	resp = callJSON(t, r, "POST", "/api/gateway/callback", callbackReq, map[string]string{
		"X-Api-Key":    callbackHeaders["X-Api-Key"],
		"X-Timestamp":  callbackHeaders["X-Timestamp"],
		"X-Signature":  callbackHeaders["X-Signature"],
		"Content-Type": callbackHeaders["Content-Type"],
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
