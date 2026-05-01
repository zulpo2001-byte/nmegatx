package main

import (
	"errors"
	"log"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"nmegateway/internal/config"
	"nmegateway/internal/model"
	"nmegateway/internal/pkg/db"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	gdb, err := db.New(cfg.DBDSN)
	if err != nil {
		log.Fatal(err)
	}
	// Seed 场景下忽略 ErrRecordNotFound 的错误日志，避免“首次写入”时刷屏误导。
	gdb = gdb.Session(&gorm.Session{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})

	adminEmail := getenv("SEED_ADMIN_EMAIL", "admin@nme.local")
	adminPassword := getenv("SEED_ADMIN_PASSWORD", "Admin@123456")
	userEmail := getenv("SEED_USER_EMAIL", "user@nme.local")
	userPassword := getenv("SEED_USER_PASSWORD", "User@123456")
	apiKey := getenv("SEED_API_KEY", "ak_demo_seed_001")
	apiSecret := getenv("SEED_API_SECRET", "sk_demo_seed_001")
	userAPIKey := getenv("SEED_USER_API_KEY", "ak_demo_user_001")
	userAPISecret := getenv("SEED_USER_API_SECRET", "sk_demo_user_001")
	createUsers := getenv("SEED_CREATE_USERS", "false") == "true"

	if createUsers {
		admin := ensureAdmin(gdb, adminEmail, adminPassword)
		merchant := ensureMerchantUser(gdb, userEmail, userPassword)
		ensureAPIKey(gdb, admin.ID, apiKey, apiSecret)
		ensureAPIKey(gdb, merchant.ID, userAPIKey, userAPISecret)
	} else {
		log.Println("seed users skipped (set SEED_CREATE_USERS=true to enable)")
	}
	ensureGlobalSettings(gdb)
	ensureRiskRules(gdb)
	log.Println("seed completed")
}

func ensureAdmin(gdb *gorm.DB, email, password string) model.User {
	var user model.User
	if err := gdb.Where("email = ?", email).First(&user).Error; err == nil {
		return user
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Fatal(err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	user = model.User{
		Email:       email,
		Password:    string(hash),
		Role:        "super_admin",
		Status:      "active",
		Permissions: `{"all":true}`,
		ExpiresAt:   time.Now().AddDate(10, 0, 0),
	}
	if err := gdb.Create(&user).Error; err != nil {
		log.Fatal(err)
	}
	return user
}

func ensureAPIKey(gdb *gorm.DB, userID uint, apiKey, secret string) {
	var key model.APIKey
	if err := gdb.Where("api_key = ?", apiKey).First(&key).Error; err == nil {
		return
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Fatal(err)
	}
	key = model.APIKey{
		UserID:  userID,
		APIKey:  apiKey,
		Secret:  secret,
		Enabled: true,
	}
	if err := gdb.Create(&key).Error; err != nil {
		log.Fatal(err)
	}
}

func ensureGlobalSettings(gdb *gorm.DB) {
	defaults := []struct{ key, val, desc string }{
		{"reset_mode", "daily", "daily=按天重置日限额, sprint=所有账号耗尽后立刻归零重来"},
		{"chargeback_threshold", "3", "PayPal/Stripe 账号 fail_count 达到此值后自动熔断"},
		{"wa_message", "网络繁忙，请联系客服", "无可用支付通道时展示给客户的提示信息"},
		{"wa_number", "", "WhatsApp 联系号码（可选）"},
	}
	for _, s := range defaults {
		var exists model.GlobalSetting
		if err := gdb.Where("key = ?", s.key).First(&exists).Error; err == nil {
			continue // 已存在，不覆盖
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Fatal(err)
		}
		gdb.Create(&model.GlobalSetting{Key: s.key, Value: s.val})
	}
}

func ensureRiskRules(gdb *gorm.DB) {
	type ruleSpec struct {
		Name, Type, Action, Conditions, Config, Description string
		Enabled                                              bool
		RiskScore                                            int
	}
	defaults := []ruleSpec{
		{"large_amount_block", "amount_range", "block", `{"min":1000,"max":999999}`, "{}", "单笔金额超过 $1000 触发高风险拦截", true, 50},
		{"ip_frequency_limit", "ip_frequency", "warn", `{"max_per_minute":5}`, "{}", "同一 IP 每分钟超过阈值叠加风险分", true, 30},
		{"ip_region_blacklist", "ip_region", "block", `{"blocked_prefixes":[]}`, "{}", "IP 前缀黑名单", true, 50},
	}
	for _, r := range defaults {
		var exists model.RiskRule
		if err := gdb.Where("name = ?", r.Name).First(&exists).Error; err == nil {
			continue
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Fatal(err)
		}
		item := model.RiskRule{
			Name: r.Name, Type: r.Type, Action: r.Action,
			Conditions: r.Conditions, Config: r.Config,
			Description: r.Description, Enabled: r.Enabled, RiskScore: r.RiskScore,
		}
		if err := gdb.Create(&item).Error; err != nil {
			log.Println("seed risk rule:", err)
		}
	}
}

func ensureMerchantUser(gdb *gorm.DB, email, password string) model.User {
	var user model.User
	if err := gdb.Where("email = ?", email).First(&user).Error; err == nil {
		return user
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Fatal(err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	user = model.User{
		Email:    email,
		Password: string(hash),
		Role:     "user",
		Status:   "active",
		Permissions: `{
			"dashboard_view":true,
			"paypal_manage":true,
			"stripe_manage":true,
			"strategy_manage":true,
			"order_view":true,
			"webhooks":true
		}`,
		ExpiresAt: time.Now().AddDate(5, 0, 0),
	}
	if err := gdb.Create(&user).Error; err != nil {
		log.Fatal(err)
	}
	return user
}

func getenv(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}
