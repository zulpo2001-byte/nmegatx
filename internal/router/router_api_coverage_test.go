package router_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"nme-v9/internal/config"
	"nme-v9/internal/model"
	"nme-v9/internal/router"
)

func TestAllBackendAPIRoutesRespond(t *testing.T) {
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
	admin := model.User{Email: "admin@cov.local", Password: string(adminPwd), Role: "admin", Status: "active", Permissions: `{"all":true}`}
	user := model.User{Email: "user@cov.local", Password: string(userPwd), Role: "user", Status: "active", Permissions: `{"all":true}`}
	_ = db.Create(&admin).Error
	_ = db.Create(&user).Error

	cfg := &config.Config{
		JWTSecret:         "test-jwt-secret",
		JWTExpiresHours:   24,
		JWTRefreshDays:    30,
		HMACWindowSeconds: 300,
		CORSAllowOrigins:  []string{"http://localhost"},
	}
	r := router.New(cfg, db, nil, nil, nil)

	adminToken := loginAndGetToken(t, r, "admin@cov.local", "123456")
	userToken := loginAndGetToken(t, r, "user@cov.local", "123456")

	for _, rt := range r.Routes() {
		if !strings.HasPrefix(rt.Path, "/api/") {
			continue
		}
		path := strings.ReplaceAll(strings.ReplaceAll(rt.Path, ":id", "1"), ":token", "tok_1")
		path = strings.ReplaceAll(path, ":endpoint_id", "1")
		body := map[string]any{}
		bodyBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(rt.Method, path, bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")

		if strings.HasPrefix(rt.Path, "/api/admin/") {
			req.Header.Set("Authorization", "Bearer "+adminToken)
		} else if strings.HasPrefix(rt.Path, "/api/user/") || strings.HasPrefix(rt.Path, "/api/auth/me") || strings.HasPrefix(rt.Path, "/api/auth/logout") || strings.HasPrefix(rt.Path, "/api/profile/password") {
			req.Header.Set("Authorization", "Bearer "+userToken)
		}

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		allowNotFound := strings.Contains(rt.Path, ":id") || strings.Contains(rt.Path, ":token") || strings.Contains(rt.Path, ":endpoint_id")
		if (w.Code == http.StatusNotFound && !allowNotFound) || w.Code == http.StatusMethodNotAllowed || w.Code >= 500 {
			// metrics summary/reset依赖 redis，允许 503
			if strings.Contains(rt.Path, "/api/admin/metrics/") && w.Code == http.StatusServiceUnavailable {
				continue
			}
			t.Fatalf("%s %s unexpected status=%d body=%s", rt.Method, rt.Path, w.Code, w.Body.String())
		}
	}
}
