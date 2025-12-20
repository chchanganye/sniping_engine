package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"sniping_engine/internal/model"
)

func (s *Store) UpsertTarget(ctx context.Context, t model.Target) (model.Target, error) {
	if t.Mode != model.TargetModeRush && t.Mode != model.TargetModeScan {
		return model.Target{}, fmt.Errorf("invalid mode: %s", t.Mode)
	}
	if t.ItemID == 0 || t.SKUID == 0 {
		return model.Target{}, errors.New("itemId and skuId are required")
	}
	if t.TargetQty <= 0 {
		return model.Target{}, errors.New("targetQty must be > 0")
	}
	if t.PerOrderQty <= 0 {
		t.PerOrderQty = 1
	}
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	now := time.Now()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now

	enabled := 0
	if t.Enabled {
		enabled = 1
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO targets (id, name, image_url, item_id, sku_id, shop_id, mode, target_qty, per_order_qty, rush_at_ms, captcha_verify_param, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			image_url = excluded.image_url,
			item_id = excluded.item_id,
			sku_id = excluded.sku_id,
			shop_id = excluded.shop_id,
			mode = excluded.mode,
			target_qty = excluded.target_qty,
			per_order_qty = excluded.per_order_qty,
			rush_at_ms = excluded.rush_at_ms,
			captcha_verify_param = excluded.captcha_verify_param,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at
	`, t.ID, t.Name, t.ImageURL, t.ItemID, t.SKUID, t.ShopID, string(t.Mode), t.TargetQty, t.PerOrderQty, t.RushAtMs, t.CaptchaVerifyParam, enabled, t.CreatedAt.UnixMilli(), t.UpdatedAt.UnixMilli())
	if err != nil {
		return model.Target{}, err
	}
	return s.GetTarget(ctx, t.ID)
}

func (s *Store) GetTarget(ctx context.Context, id string) (model.Target, error) {
	var row struct {
		id                 string
		name               string
		imageURL           string
		itemID             int64
		skuID              int64
		shopID             int64
		mode               string
		targetQty          int
		perOrderQty        int
		rushAtMs           int64
		captchaVerifyParam string
		enabled            int
		createdAt          int64
		updatedAt          int64
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, image_url, item_id, sku_id, shop_id, mode, target_qty, per_order_qty, rush_at_ms, captcha_verify_param, enabled, created_at, updated_at
		FROM targets WHERE id = ?
	`, id).Scan(&row.id, &row.name, &row.imageURL, &row.itemID, &row.skuID, &row.shopID, &row.mode, &row.targetQty, &row.perOrderQty, &row.rushAtMs, &row.captchaVerifyParam, &row.enabled, &row.createdAt, &row.updatedAt)
	if err != nil {
		return model.Target{}, err
	}
	return model.Target{
		ID:                 row.id,
		Name:               row.name,
		ImageURL:           row.imageURL,
		ItemID:             row.itemID,
		SKUID:              row.skuID,
		ShopID:             row.shopID,
		Mode:               model.TargetMode(row.mode),
		TargetQty:          row.targetQty,
		PerOrderQty:        row.perOrderQty,
		RushAtMs:           row.rushAtMs,
		CaptchaVerifyParam: row.captchaVerifyParam,
		Enabled:            row.enabled == 1,
		CreatedAt:          time.UnixMilli(row.createdAt),
		UpdatedAt:          time.UnixMilli(row.updatedAt),
	}, nil
}

func (s *Store) ListTargets(ctx context.Context) ([]model.Target, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, image_url, item_id, sku_id, shop_id, mode, target_qty, per_order_qty, rush_at_ms, captcha_verify_param, enabled, created_at, updated_at
		FROM targets ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Target
	for rows.Next() {
		var row struct {
			id                 string
			name               string
			imageURL           string
			itemID             int64
			skuID              int64
			shopID             int64
			mode               string
			targetQty          int
			perOrderQty        int
			rushAtMs           int64
			captchaVerifyParam string
			enabled            int
			createdAt          int64
			updatedAt          int64
		}
		if err := rows.Scan(&row.id, &row.name, &row.imageURL, &row.itemID, &row.skuID, &row.shopID, &row.mode, &row.targetQty, &row.perOrderQty, &row.rushAtMs, &row.captchaVerifyParam, &row.enabled, &row.createdAt, &row.updatedAt); err != nil {
			return nil, err
		}
		out = append(out, model.Target{
			ID:                 row.id,
			Name:               row.name,
			ImageURL:           row.imageURL,
			ItemID:             row.itemID,
			SKUID:              row.skuID,
			ShopID:             row.shopID,
			Mode:               model.TargetMode(row.mode),
			TargetQty:          row.targetQty,
			PerOrderQty:        row.perOrderQty,
			RushAtMs:           row.rushAtMs,
			CaptchaVerifyParam: row.captchaVerifyParam,
			Enabled:            row.enabled == 1,
			CreatedAt:          time.UnixMilli(row.createdAt),
			UpdatedAt:          time.UnixMilli(row.updatedAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ListEnabledTargets(ctx context.Context) ([]model.Target, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, image_url, item_id, sku_id, shop_id, mode, target_qty, per_order_qty, rush_at_ms, captcha_verify_param, enabled, created_at, updated_at
		FROM targets WHERE enabled = 1 ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Target
	for rows.Next() {
		var row struct {
			id                 string
			name               string
			imageURL           string
			itemID             int64
			skuID              int64
			shopID             int64
			mode               string
			targetQty          int
			perOrderQty        int
			rushAtMs           int64
			captchaVerifyParam string
			enabled            int
			createdAt          int64
			updatedAt          int64
		}
		if err := rows.Scan(&row.id, &row.name, &row.imageURL, &row.itemID, &row.skuID, &row.shopID, &row.mode, &row.targetQty, &row.perOrderQty, &row.rushAtMs, &row.captchaVerifyParam, &row.enabled, &row.createdAt, &row.updatedAt); err != nil {
			return nil, err
		}
		out = append(out, model.Target{
			ID:                 row.id,
			Name:               row.name,
			ImageURL:           row.imageURL,
			ItemID:             row.itemID,
			SKUID:              row.skuID,
			ShopID:             row.shopID,
			Mode:               model.TargetMode(row.mode),
			TargetQty:          row.targetQty,
			PerOrderQty:        row.perOrderQty,
			RushAtMs:           row.rushAtMs,
			CaptchaVerifyParam: row.captchaVerifyParam,
			Enabled:            row.enabled == 1,
			CreatedAt:          time.UnixMilli(row.createdAt),
			UpdatedAt:          time.UnixMilli(row.updatedAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) DeleteTarget(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM targets WHERE id = ?`, id)
	return err
}
