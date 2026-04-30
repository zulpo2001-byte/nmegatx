package config

import (
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	AppPort           string
	AppBaseURL        string
	JWTSecret         string
	JWTExpiresHours   int
	JWTRefreshDays    int
	DBDSN             string
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	AsynqConcurrency  int
	HMACWindowSeconds int64
	CORSAllowOrigins  []string
}

func Load() (*Config, error) {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	_ = viper.ReadInConfig()

	cfg := &Config{
		AppPort:           viper.GetString("APP_PORT"),
		AppBaseURL:        viper.GetString("APP_BASE_URL"),
		JWTSecret:         viper.GetString("JWT_SECRET"),
		JWTExpiresHours:   viper.GetInt("JWT_EXPIRES_HOURS"),
		JWTRefreshDays:    viper.GetInt("JWT_REFRESH_DAYS"),
		RedisAddr:         viper.GetString("REDIS_ADDR"),
		RedisPassword:     viper.GetString("REDIS_PASSWORD"),
		RedisDB:           viper.GetInt("REDIS_DB"),
		AsynqConcurrency:  viper.GetInt("ASYNQ_CONCURRENCY"),
		HMACWindowSeconds: viper.GetInt64("HMAC_WINDOW_SECONDS"),
		CORSAllowOrigins:  strings.Split(viper.GetString("CORS_ALLOW_ORIGINS"), ","),
	}

	if cfg.AppPort == "" {
		cfg.AppPort = "8080"
	}
	if cfg.JWTRefreshDays <= 0 {
		cfg.JWTRefreshDays = 30
	}

	dbHost := viper.GetString("DB_HOST")
	dbPort := viper.GetString("DB_PORT")
	dbUser := viper.GetString("DB_USER")
	dbPass := viper.GetString("DB_PASSWORD")
	dbName := viper.GetString("DB_NAME")
	dbSSL := viper.GetString("DB_SSLMODE")
	if dbSSL == "" {
		dbSSL = "disable"
	}
	cfg.DBDSN = "host=" + dbHost + " port=" + dbPort + " user=" + dbUser + " password=" + dbPass + " dbname=" + dbName + " sslmode=" + dbSSL
	return cfg, nil
}
