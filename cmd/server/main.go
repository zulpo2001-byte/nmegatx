package main

import (
	"log"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"nme-v9/internal/config"
	"nme-v9/internal/pkg/db"
	"nme-v9/internal/router"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	z, _ := zap.NewProduction()
	defer z.Sync()

	gdb, err := db.New(cfg.DBDSN)
	if err != nil {
		z.Fatal("db connect failed", zap.Error(err))
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	queue := asynq.NewClient(asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer queue.Close()

	r := router.New(cfg, gdb, rdb, queue, z)
	if err := r.Run(":" + cfg.AppPort); err != nil {
		z.Fatal("server start failed", zap.Error(err))
	}
}
