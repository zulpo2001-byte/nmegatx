package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"nmegateway/internal/model"
)

func TestGatewayAPIKeyAndABEndpoints(t *testing.T) {
	var aHits int32
	var bHits int32

	aSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&aHits, 1)
		if r.Header.Get("X-Api-Key") == "" || r.Header.Get("X-Signature") == "" || r.Header.Get("X-Timestamp") == "" {
			t.Fatalf("A endpoint missing signature headers")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer aSrv.Close()

	bSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/wp-json/b-station/v1/complete" {
			atomic.AddInt32(&bHits, 1)
			if r.Header.Get("X-Api-Key") == "" || r.Header.Get("X-Signature") == "" || r.Header.Get("X-Timestamp") == "" {
				t.Fatalf("B endpoint missing signature headers")
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/wp-json/b-station/v1/order" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"payment_url":"https://b.local/pay","b_order_id":"B-10001"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer bSrv.Close()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Order{}, &model.WebhookEndpoint{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	user := model.User{Email: "u@test.local", Password: "x", Role: "user", Status: "active"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	aEP := model.WebhookEndpoint{UserID: user.ID, Type: "a", Enabled: true, URL: aSrv.URL, AApiKey: "ak_test", SharedSecret: "sk_test"}
	bEP := model.WebhookEndpoint{UserID: user.ID, Type: "b", Enabled: true, URL: bSrv.URL, BApiKey: "b_api_key", BSharedSecret: "b_shared"}
	_ = db.Create(&aEP).Error
	_ = db.Create(&bEP).Error

	svc := &GatewayService{DB: db}

	if _, _, _, err := svc.ResolveUserByAPIKey("ak_test"); err != nil {
		t.Fatalf("ResolveUserByAPIKey failed: %v", err)
	}
	if _, err := svc.ResolveByBApiKey("b_api_key"); err != nil {
		t.Fatalf("ResolveByBApiKey failed: %v", err)
	}

	now := time.Now().UTC()
	order := model.Order{
		UserID:         user.ID,
		AOrderID:       "A-10001",
		BOrderID:       "B-10001",
		PayToken:       "pt_10001",
		Amount:         88.5,
		Status:         "completed",
		PaymentMethod:  "stripe",
		BTransactionID: "tx_abc",
		PaidAt:         &now,
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}

	svc.CallbackAStation(&order)
	svc.CallbackBStation(&order)
	time.Sleep(150 * time.Millisecond)

	if atomic.LoadInt32(&aHits) == 0 {
		t.Fatalf("A endpoint was not called")
	}
	if atomic.LoadInt32(&bHits) == 0 {
		t.Fatalf("B endpoint complete was not called")
	}
}
