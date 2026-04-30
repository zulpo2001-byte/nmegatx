package service

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"gorm.io/gorm"
	"nme-v9/internal/model"
)

func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func SaveRefreshToken(db *gorm.DB, userID uint, rawToken string, days int) (*model.RefreshToken, error) {
	rt := &model.RefreshToken{
		UserID:    userID,
		TokenHash: HashRefreshToken(rawToken),
		ExpiresAt: time.Now().Add(time.Duration(days) * 24 * time.Hour),
	}
	return rt, db.Create(rt).Error
}

func RevokeRefreshToken(db *gorm.DB, rawToken string) error {
	now := time.Now()
	return db.Model(&model.RefreshToken{}).
		Where("token_hash = ? AND revoked_at IS NULL", HashRefreshToken(rawToken)).
		Update("revoked_at", &now).Error
}
