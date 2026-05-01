package router

import (
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"nme-v9/internal/config"
	"nme-v9/internal/handler/admin"
	"nme-v9/internal/handler/auth"
	"nme-v9/internal/handler/gateway"
	"nme-v9/internal/handler/pay"
	"nme-v9/internal/handler/user"
	"nme-v9/internal/middleware"
	"nme-v9/internal/service"
)

func New(cfg *config.Config, db *gorm.DB, rdb *redis.Client, queue *asynq.Client, logger *zap.Logger) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.CORS(cfg.CORSAllowOrigins))

	alertSvc := &service.AlertService{DB: db, Log: logger}
	smartSvc := &service.SmartRoutingService{DB: db, RDB: rdb, Logger: logger}
	gwSvc := &service.GatewayService{
		DB:       db,
		Queue:    queue,
		Log:      logger,
		RDB:      rdb,
		AlertSvc: alertSvc,
		SmartSvc: smartSvc,
	}

	authH  := &auth.Handler{DB: db, JWTSecret: cfg.JWTSecret, JWTHours: cfg.JWTExpiresHours, JWTRefreshDays: cfg.JWTRefreshDays}
	gwH    := &gateway.Handler{Gateway: gwSvc, HMACWindowS: cfg.HMACWindowSeconds}
	payH   := &pay.Handler{}
	adminH := &admin.Handler{DB: db, Log: logger, RDB: rdb}
	userH  := &user.Handler{DB: db, BaseURL: cfg.AppBaseURL}

	// ── 静态前端 ────────────────────────────────────────────────
	r.StaticFile("/", "./frontend/index.html")
	r.StaticFile("/index.html", "./frontend/index.html")
	r.Static("/assets", "./frontend/assets")

	// ── 公开 API ────────────────────────────────────────────────
	api := r.Group("/api")
	{
		api.GET("/health", payH.Health)
		api.POST("/auth/login", authH.Login)
		api.POST("/auth/refresh", authH.Refresh)
		api.POST("/gateway/order", gwH.Order)
		api.POST("/gateway/callback", gwH.Callback)
		api.POST("/gateway/pay-link", gwH.GeneratePayLink)
		api.GET("/gateway/status/:token", gwH.Status)
	}

	// ── 需登录（所有角色） ───────────────────────────────────────
	authGroup := api.Group("", middleware.JWT(cfg.JWTSecret))
	{
		authGroup.GET("/auth/me", authH.Me)
		authGroup.POST("/auth/logout", authH.Logout)
		// 所有角色改自己密码/邮箱（handler 内部按 role 区分权限）
		authGroup.PUT("/profile/password", adminH.UpdateSelfPassword)

		// ════════════════════════════════════════════════════════
		// Admin 路由（role: admin | super_admin）
		// ════════════════════════════════════════════════════════
		ag := authGroup.Group("/admin", middleware.Role("admin", "super_admin"))

		// 系统概览
		ag.GET("/stats", middleware.AdminPerm("admin.stats.view"), adminH.Stats)

		// 订单管理
		ag.GET("/orders", middleware.AdminPerm("admin.orders.view"), adminH.Orders)
		ag.GET("/orders/stats", middleware.AdminPerm("admin.orders.view"), adminH.OrdersStats)
		ag.GET("/orders/export", middleware.AdminPerm("admin.reports.view"), adminH.OrdersExport)
		ag.GET("/orders/reset-mode", middleware.AdminPerm("admin.orders.view"), adminH.GetResetMode)
		// ⚠️ 具体 id 路由必须在 /orders/reset-mode 等静态路由之后注册
		ag.GET("/orders/:id", middleware.AdminPerm("admin.orders.view"), adminH.OrderDetail)
		ag.POST("/orders/force-reset", middleware.AdminPerm("admin.orders.manage"), adminH.ForceReset)

		// 报表
		ag.GET("/reports/daily", middleware.AdminPerm("admin.reports.view"), adminH.ReportsDaily)
		ag.GET("/reports/export", middleware.AdminPerm("admin.reports.view"), adminH.ReportsExport)

		// 用户管理
		ag.GET("/users", middleware.AdminPerm("admin.users.view"), adminH.Users)
		ag.GET("/users/sub-accounts-amount", middleware.AdminPerm("admin.users.view"), adminH.SubAccountsAmount)
		ag.POST("/users", middleware.AdminPerm("admin.users.manage"), adminH.CreateUser)
		ag.PUT("/users/:id", middleware.AdminPerm("admin.users.manage"), adminH.UpdateUser)
		ag.DELETE("/users/:id", middleware.AdminPerm("admin.users.manage"), adminH.DeleteUser)
		ag.POST("/users/:id/reset-password", middleware.AdminPerm("admin.users.manage"), adminH.AdminResetPassword)
		ag.POST("/users/:id/toggle-status", middleware.AdminPerm("admin.users.manage"), adminH.AdminToggleStatus)
		ag.PUT("/users/:id/permissions", middleware.AdminPerm("admin.users.manage"), adminH.AdminUpdateUserPermissions)
		ag.POST("/users/:id/assign-role", middleware.AdminPerm("admin.users.manage"), adminH.AssignRole)
		ag.POST("/users/:id/recharge", middleware.AdminPerm("admin.users.manage"), adminH.RechargeUserBalance)
		ag.GET("/users/:id/balance-records", middleware.AdminPerm("admin.users.view"), adminH.UserBalanceRecords)

		// 角色管理（完整 CRUD）
		ag.GET("/roles", middleware.AdminPerm("admin.roles.view"), adminH.Roles)
		ag.POST("/roles", middleware.AdminPerm("admin.roles.manage"), adminH.CreateRole)
		ag.PUT("/roles/:id", middleware.AdminPerm("admin.roles.manage"), adminH.UpdateRole)
		ag.DELETE("/roles/:id", middleware.AdminPerm("admin.roles.manage"), adminH.DeleteRole)

		// 风控规则
		ag.GET("/risk-rules", middleware.AdminPerm("admin.risk_rules.view"), adminH.RiskRules)
		ag.POST("/risk-rules", middleware.AdminPerm("admin.risk_rules.manage"), adminH.CreateRiskRule)
		ag.PUT("/risk-rules/:id", middleware.AdminPerm("admin.risk_rules.manage"), adminH.UpdateRiskRule)
		ag.DELETE("/risk-rules/:id", middleware.AdminPerm("admin.risk_rules.manage"), adminH.DeleteRiskRule)
		ag.POST("/risk-rules/:id/toggle", middleware.AdminPerm("admin.risk_rules.manage"), adminH.ToggleRiskRule)

		// 告警记录
		ag.GET("/alerts", middleware.AdminPerm("admin.alerts.view"), adminH.Alerts)
		ag.POST("/alerts/:id/acknowledge", middleware.AdminPerm("admin.alerts.manage"), adminH.AcknowledgeAlert)
		ag.POST("/alerts/:id/resolve", middleware.AdminPerm("admin.alerts.manage"), adminH.ResolveAlert)

		// 告警渠道（含 toggle）
		ag.GET("/alert-channels", middleware.AdminPerm("admin.alerts.manage"), adminH.AlertChannels)
		ag.POST("/alert-channels", middleware.AdminPerm("admin.alerts.manage"), adminH.CreateAlertChannel)
		ag.POST("/alert-channels/:id/toggle", middleware.AdminPerm("admin.alerts.manage"), adminH.ToggleAlertChannel)
		ag.DELETE("/alert-channels/:id", middleware.AdminPerm("admin.alerts.manage"), adminH.DeleteAlertChannel)

		// 智能路由
		ag.GET("/smart-routing/stats", middleware.AdminPerm("admin.stats.view"), adminH.SmartRoutingStats)
		ag.POST("/smart-routing/recalculate", middleware.AdminPerm("admin.stats.view"), adminH.SmartRoutingRecalculate)

		// 渠道指标
		ag.GET("/metrics/channels", middleware.AdminPerm("admin.stats.view"), adminH.ChannelMetrics)
		ag.GET("/metrics/summary", middleware.AdminPerm("admin.stats.view"), adminH.MetricsSummary)
		ag.DELETE("/metrics/endpoints/:id/reset", middleware.AdminPerm("admin.stats.view"), adminH.MetricsResetEndpoint)
		ag.DELETE("/metrics/:endpoint_id/reset", middleware.AdminPerm("admin.stats.view"), adminH.MetricsResetEndpoint) // backward-compatible alias

		// 系统设置
		ag.GET("/settings", middleware.AdminPerm("admin.settings.view"), adminH.GetSettings)
		ag.POST("/settings", middleware.AdminPerm("admin.settings.manage"), adminH.UpdateSettings)
		ag.POST("/settings/test-alert", middleware.AdminPerm("admin.alerts.manage"), adminH.TestAlertPush)

		// 审计日志（支持过滤+分页）
		ag.GET("/audit-logs", middleware.AdminPerm("admin.audit.view"), adminH.AuditLogs)

		// 网关运维
		ag.POST("/gateway/circuit-break", middleware.AdminPerm("admin.stats.view"), gwH.CircuitBreak)
		ag.POST("/gateway/fail", middleware.AdminPerm("admin.stats.view"), gwH.ReportFail)

		// ════════════════════════════════════════════════════════
		// User 路由（role: user | super_admin）
		// ════════════════════════════════════════════════════════
		ug := authGroup.Group("/user", middleware.Role("user", "super_admin"))

		// 概览 & 订单
		ug.GET("/dashboard", middleware.UserPerm("dashboard_view"), userH.Dashboard)
		ug.GET("/orders", middleware.UserPerm("order_view"), userH.Orders)
		ug.GET("/orders/stats", middleware.UserPerm("order_view"), userH.UserOrdersStats)

		// 用户可读的全局设置（只读）
		ug.GET("/settings", middleware.UserPerm("dashboard_view"), userH.UserReadSettings)
		// 用户权重快照（只读）
		ug.GET("/smart-routing/my-weights", middleware.UserPerm("dashboard_view"), userH.UserSmartRoutingWeights)

		// PayPal 账号
		ug.GET("/paypal", middleware.UserPerm("paypal_manage"), userH.PaypalAccounts)
		ug.POST("/paypal", middleware.UserPerm("paypal_manage"), userH.CreatePaypalAccount)
		ug.PUT("/paypal/:id", middleware.UserPerm("paypal_manage"), userH.UpdatePaypalAccount)
		ug.DELETE("/paypal/:id", middleware.UserPerm("paypal_manage"), userH.DeletePaypalAccount)
		ug.POST("/paypal/:id/toggle", middleware.UserPerm("paypal_manage"), userH.TogglePaypalAccount)
		ug.POST("/paypal/:id/reset-daily", middleware.UserPerm("paypal_manage"), userH.ResetPaypalDaily)
		ug.POST("/paypal/:id/state", middleware.UserPerm("paypal_manage"), userH.SetPaypalAccountState)

		// Stripe 配置
		ug.GET("/stripe", middleware.UserPerm("stripe_manage"), userH.StripeConfigs)
		ug.POST("/stripe", middleware.UserPerm("stripe_manage"), userH.CreateStripeConfig)
		ug.PUT("/stripe/:id", middleware.UserPerm("stripe_manage"), userH.UpdateStripeConfig)
		ug.DELETE("/stripe/:id", middleware.UserPerm("stripe_manage"), userH.DeleteStripeConfig)
		ug.POST("/stripe/:id/toggle", middleware.UserPerm("stripe_manage"), userH.ToggleStripeConfig)
		ug.POST("/stripe/:id/reset-daily", middleware.UserPerm("stripe_manage"), userH.ResetStripeDaily)
		ug.POST("/stripe/:id/state", middleware.UserPerm("stripe_manage"), userH.SetStripeAccountState)

		// Webhook 端点（保留原有 + 新增密钥轮换 + 测试）
		ug.GET("/webhooks", middleware.UserPerm("webhooks"), userH.Webhooks)
		ug.POST("/webhooks/a", middleware.UserPerm("webhooks"), userH.CreateWebhookA)
		ug.POST("/webhooks/b", middleware.UserPerm("webhooks"), userH.CreateWebhookB)
		ug.PUT("/webhooks/:id", middleware.UserPerm("webhooks"), userH.UpdateWebhook)
		ug.DELETE("/webhooks/:id", middleware.UserPerm("webhooks"), userH.DeleteWebhook)
		ug.GET("/webhooks/:id/config-string", middleware.UserPerm("webhooks"), userH.WebhookConfigString)
		// 密钥轮换
		ug.POST("/webhooks/:id/regenerate-a-secret", middleware.UserPerm("webhooks"), userH.RegenerateASecret)
		ug.POST("/webhooks/:id/regenerate-a-api-key", middleware.UserPerm("webhooks"), userH.RegenerateAAApiKey)
		ug.POST("/webhooks/:id/regenerate-b-secret", middleware.UserPerm("webhooks"), userH.RegenerateBSecret)
		ug.POST("/webhooks/:id/regenerate-b-api-key", middleware.UserPerm("webhooks"), userH.RegenerateBApiKey)
		// 测试推送
		ug.POST("/webhooks/:id/test", middleware.UserPerm("webhooks"), userH.TestWebhook)
	}

	r.NoRoute(func(c *gin.Context) {
		c.File("./frontend/index.html")
	})

	return r
}
