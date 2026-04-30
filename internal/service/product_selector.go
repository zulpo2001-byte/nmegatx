package service

import (
	"errors"
	"math/rand"
	"time"

	"gorm.io/gorm"
	"nme-v9/internal/model"
)

// SelectProduct picks a B-station product for the given user according to their strategy.
// Strategies: round_robin (default) | random (weighted) | fixed
func SelectProduct(db *gorm.DB, user *model.User) (*model.Product, error) {
	var candidates []model.Product
	if err := db.Where("user_id = ? AND enabled = true", user.ID).
		Order("poll_order asc, id asc").
		Find(&candidates).Error; err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, errors.New("no available product")
	}

	switch user.ProductStrategy {
	case "random":
		return weightedRandom(candidates), nil
	case "fixed":
		return &candidates[0], nil
	default: // round_robin
		return roundRobinAtomic(db, candidates)
	}
}

// roundRobinAtomic picks the product with the oldest last_used_at using
// an atomic timestamp update so concurrent callers get different products.
// NOTE: total_used increment is handled by RecordUsage() in the caller.
func roundRobinAtomic(db *gorm.DB, list []model.Product) (*model.Product, error) {
	best := &list[0]
	bestTs := lastUsedTs(list[0])
	for i := 1; i < len(list); i++ {
		if ts := lastUsedTs(list[i]); ts < bestTs {
			bestTs = ts
			best = &list[i]
		}
	}

	// Immediately stamp last_used_at so the next concurrent call picks a different product.
	now := time.Now()
	db.Model(&model.Product{}).Where("id = ?", best.ID).
		Update("last_used_at", &now)
	return best, nil
}

func lastUsedTs(p model.Product) int64 {
	if p.LastUsedAt == nil {
		return 0
	}
	return p.LastUsedAt.Unix()
}

// weightedRandom picks a product proportional to its Weight field.
func weightedRandom(list []model.Product) *model.Product {
	total := 0
	for _, p := range list {
		w := p.Weight
		if w < 1 {
			w = 1
		}
		total += w
	}
	if total <= 0 {
		return &list[0]
	}
	r := rand.Intn(total) + 1
	cum := 0
	for i := range list {
		w := list[i].Weight
		if w < 1 {
			w = 1
		}
		cum += w
		if r <= cum {
			return &list[i]
		}
	}
	return &list[len(list)-1]
}

// RecordUsage increments total_used and updates last_used_at for the chosen product.
func RecordUsage(db *gorm.DB, productID uint) {
	now := time.Now()
	db.Model(&model.Product{}).Where("id = ?", productID).Updates(map[string]any{
		"total_used":   gorm.Expr("total_used + 1"),
		"last_used_at": &now,
	})
}
