package admin

import (
	"encoding/json"
	"fmt"
	"regexp"

	"gorm.io/gorm"
)

// marshalMapBool 将 map[string]bool 序列化为 JSON 字符串
func marshalMapBool(m map[string]bool) (string, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return "{}", err
	}
	return string(b), nil
}

var safeTableName = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// createTenantTables 为新用户创建独立业务表（订单、PayPal、Stripe）。
// 采用 CTAS 空表方式，避免跨数据库方言差异（SQLite/MySQL/Postgres 均可执行）。
func createTenantTables(db *gorm.DB, userID uint) error {
	tables := [][2]string{
		{"orders", fmt.Sprintf("orders_u_%d", userID)},
		{"paypal_accounts", fmt.Sprintf("paypal_accounts_u_%d", userID)},
		{"stripe_configs", fmt.Sprintf("stripe_configs_u_%d", userID)},
	}
	for _, pair := range tables {
		src, dst := pair[0], pair[1]
		if !safeTableName.MatchString(src) || !safeTableName.MatchString(dst) {
			return fmt.Errorf("invalid table name")
		}
		sql := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s AS SELECT * FROM %s WHERE 1=0", dst, src)
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("create tenant table %s failed: %w", dst, err)
		}
	}
	return nil
}
