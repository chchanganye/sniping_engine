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
