package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"nme-v9/internal/config"
	"nme-v9/internal/model"
	"nme-v9/internal/pkg/db"
	"nme-v9/internal/service"
	"nme-v9/internal/task"
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
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	}
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	queue := asynq.NewClient(redisOpt)

	alertSvc := &service.AlertService{DB: gdb, Log: logger}
	smartSvc := &service.SmartRoutingService{DB: gdb, RDB: rdb, Logger: logger}
	gwSvc := &service.GatewayService{
		DB:       gdb,
		Queue:    queue,
		Log:      logger,
		RDB:      rdb,
		AlertSvc: alertSvc,
		SmartSvc: smartSvc,
	}

	srv := asynq.NewServer(redisOpt, asynq.Config{Concurrency: cfg.AsynqConcurrency})
	mux := asynq.NewServeMux()

	// ── callback:a — 异步回调 A站 ─────────────────────────────────
	mux.HandleFunc(task.TypeCallbackA, func(ctx context.Context, t *asynq.Task) error {
		var p task.CallbackPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return err
		}
		var order model.Order
		if err := gdb.First(&order, p.OrderID).Error; err != nil {
			return err
		}
		gwSvc.CallbackAStation(&order)
		return nil
	})

		// ── check:abandoned — 180s 超时标记放弃 ──────────────────────
	mux.HandleFunc(task.TypeCheckAbandoned, func(ctx context.Context, t *asynq.Task) error {
		var p map[string]uint
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return err
		}
		var order model.Order
		if err := gdb.First(&order, p["order_id"]).Error; err != nil {
			return err
		}
		if order.Status != "pending" {
			return nil
		}
			if time.Since(order.CreatedAt) < 180*time.Second {
			return nil
		}
		now := time.Now()
		return gdb.Model(&model.Order{}).
			Where("id = ? AND status = 'pending'", order.ID).
			Updates(map[string]any{"status": "abandoned", "abandoned_at": &now}).Error
	})

	// ── smart_routing:recalculate — 每小时重算动态权重 ───────────
	mux.HandleFunc("smart_routing:recalculate", func(ctx context.Context, t *asynq.Task) error {
		smartSvc.RecalculateAll()
		return nil
	})

	// ── smart_routing:flush_metrics — 每5分钟强制 flush buffer ──
	mux.HandleFunc("smart_routing:flush_metrics", func(ctx context.Context, t *asynq.Task) error {
		smartSvc.FlushAllMetrics()
		return nil
	})

	// ── orders:expire — 每小时扫描过期订单 ───────────────────────
	mux.HandleFunc("orders:expire", func(ctx context.Context, t *asynq.Task) error {
		now := time.Now().UTC()
		var expiredOrders []model.Order
		gdb.Where("expires_at IS NOT NULL AND expires_at <= ? AND status = 'pending'", now).Find(&expiredOrders)
		if len(expiredOrders) == 0 {
			return nil
		}
		ids := make([]uint, 0, len(expiredOrders))
		for _, o := range expiredOrders {
			ids = append(ids, o.ID)
		}
		gdb.Model(&model.Order{}).Where("id IN ?", ids).Updates(map[string]any{
			"status":       "expired",
			"abandoned_at": &now,
		})
		logger.Info("Orders expired", zap.Int("count", len(expiredOrders)))
		// 通知 A站订单已过期
		for _, o := range expiredOrders {
			order := o
			order.Status = "expired"
			go gwSvc.CallbackAStation(&order)
		}
		return nil
	})

	// ── 定时任务注册 ──────────────────────────────────────────────
	scheduler := asynq.NewScheduler(redisOpt, nil)

	if _, err := scheduler.Register("0 * * * *",
		asynq.NewTask("smart_routing:recalculate", nil)); err != nil {
		logger.Error("register recalculate failed", zap.Error(err))
	}
	if _, err := scheduler.Register("*/5 * * * *",
		asynq.NewTask("smart_routing:flush_metrics", nil)); err != nil {
		logger.Error("register flush_metrics failed", zap.Error(err))
	}
	if _, err := scheduler.Register("0 * * * *",
		asynq.NewTask("orders:expire", nil)); err != nil {
		logger.Error("register orders:expire failed", zap.Error(err))
	}

	go func() {
		if err := scheduler.Run(); err != nil {
			logger.Fatal("scheduler failed", zap.Error(err))
		}
	}()

	logger.Info("Worker started",
		zap.String("redis", cfg.RedisAddr),
		zap.Int("concurrency", cfg.AsynqConcurrency),
	)
	if err := srv.Run(mux); err != nil {
		log.Fatal(err)
	}
}
