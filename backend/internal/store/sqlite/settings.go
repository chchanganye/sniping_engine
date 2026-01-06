package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"sniping_engine/internal/model"
)

const emailSettingsKey = "email_settings"
const limitsSettingsKey = "limits_settings"
const captchaPoolSettingsKey = "captcha_pool_settings"
const notifySettingsKey = "notify_settings"

func (s *Store) GetEmailSettings(ctx context.Context) (model.EmailSettings, bool, error) {
	var row struct {
		valueJSON string
		updatedAt int64
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT value_json, updated_at FROM settings WHERE key = ?
	`, emailSettingsKey).Scan(&row.valueJSON, &row.updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.EmailSettings{}, false, nil
		}
		return model.EmailSettings{}, false, err
	}
	var out model.EmailSettings
	if err := json.Unmarshal([]byte(row.valueJSON), &out); err != nil {
		return model.EmailSettings{}, false, err
	}
	if strings.TrimSpace(out.Email) == "" {
		var legacy struct {
			Enabled  bool   `json:"enabled"`
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.Unmarshal([]byte(row.valueJSON), &legacy); err == nil {
			if strings.TrimSpace(legacy.Username) != "" && strings.TrimSpace(out.Email) == "" {
				out.Enabled = out.Enabled || legacy.Enabled
				out.Email = strings.TrimSpace(legacy.Username)
				out.AuthCode = strings.TrimSpace(legacy.Password)
			}
		}
	}
	return out, true, nil
}

func (s *Store) UpsertEmailSettings(ctx context.Context, v model.EmailSettings) (model.EmailSettings, error) {
	now := time.Now().UnixMilli()
	b, err := json.Marshal(v)
	if err != nil {
		return model.EmailSettings{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO settings (key, value_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value_json = excluded.value_json,
			updated_at = excluded.updated_at
	`, emailSettingsKey, string(b), now)
	if err != nil {
		return model.EmailSettings{}, err
	}
	return v, nil
}

func (s *Store) GetLimitsSettings(ctx context.Context) (model.LimitsSettings, bool, error) {
	var row struct {
		valueJSON string
		updatedAt int64
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT value_json, updated_at FROM settings WHERE key = ?
	`, limitsSettingsKey).Scan(&row.valueJSON, &row.updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.LimitsSettings{}, false, nil
		}
		return model.LimitsSettings{}, false, err
	}
	var out model.LimitsSettings
	if err := json.Unmarshal([]byte(row.valueJSON), &out); err != nil {
		return model.LimitsSettings{}, false, err
	}
	return out, true, nil
}

func (s *Store) UpsertLimitsSettings(ctx context.Context, v model.LimitsSettings) (model.LimitsSettings, error) {
	now := time.Now().UnixMilli()
	b, err := json.Marshal(v)
	if err != nil {
		return model.LimitsSettings{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO settings (key, value_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value_json = excluded.value_json,
			updated_at = excluded.updated_at
	`, limitsSettingsKey, string(b), now)
	if err != nil {
		return model.LimitsSettings{}, err
	}
	return v, nil
}

func (s *Store) GetCaptchaPoolSettings(ctx context.Context) (model.CaptchaPoolSettings, bool, error) {
	var row struct {
		valueJSON string
		updatedAt int64
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT value_json, updated_at FROM settings WHERE key = ?
	`, captchaPoolSettingsKey).Scan(&row.valueJSON, &row.updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.CaptchaPoolSettings{}, false, nil
		}
		return model.CaptchaPoolSettings{}, false, err
	}
	var out model.CaptchaPoolSettings
	if err := json.Unmarshal([]byte(row.valueJSON), &out); err != nil {
		return model.CaptchaPoolSettings{}, false, err
	}
	return out, true, nil
}

func (s *Store) UpsertCaptchaPoolSettings(ctx context.Context, v model.CaptchaPoolSettings) (model.CaptchaPoolSettings, error) {
	now := time.Now().UnixMilli()
	b, err := json.Marshal(v)
	if err != nil {
		return model.CaptchaPoolSettings{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO settings (key, value_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value_json = excluded.value_json,
			updated_at = excluded.updated_at
	`, captchaPoolSettingsKey, string(b), now)
	if err != nil {
		return model.CaptchaPoolSettings{}, err
	}
	return v, nil
}

func (s *Store) GetNotifySettings(ctx context.Context) (model.NotifySettings, bool, error) {
	var row struct {
		valueJSON string
		updatedAt int64
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT value_json, updated_at FROM settings WHERE key = ?
	`, notifySettingsKey).Scan(&row.valueJSON, &row.updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.NotifySettings{}, false, nil
		}
		return model.NotifySettings{}, false, err
	}
	var out model.NotifySettings
	if err := json.Unmarshal([]byte(row.valueJSON), &out); err != nil {
		return model.NotifySettings{}, false, err
	}
	return out, true, nil
}

func (s *Store) UpsertNotifySettings(ctx context.Context, v model.NotifySettings) (model.NotifySettings, error) {
	now := time.Now().UnixMilli()
	b, err := json.Marshal(v)
	if err != nil {
		return model.NotifySettings{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO settings (key, value_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value_json = excluded.value_json,
			updated_at = excluded.updated_at
	`, notifySettingsKey, string(b), now)
	if err != nil {
		return model.NotifySettings{}, err
	}
	return v, nil
}
