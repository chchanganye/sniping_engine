package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"sniping_engine/internal/model"
)

func (s *Store) UpsertAccount(ctx context.Context, acc model.Account) (model.Account, error) {
	if acc.Mobile == "" {
		return model.Account{}, errors.New("mobile is required")
	}
	if acc.ID == "" {
		acc.ID = uuid.NewString()
	}
	now := time.Now()
	if acc.CreatedAt.IsZero() {
		acc.CreatedAt = now
	}
	acc.UpdatedAt = now

	cookiesJSON, err := json.Marshal(acc.Cookies)
	if err != nil {
		return model.Account{}, err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO accounts (id, username, mobile, token, user_agent, device_id, uuid, proxy, address_id, division_ids, cookies_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(mobile) DO UPDATE SET
			username = excluded.username,
			token = excluded.token,
			user_agent = excluded.user_agent,
			device_id = excluded.device_id,
			uuid = excluded.uuid,
			proxy = excluded.proxy,
			address_id = excluded.address_id,
			division_ids = excluded.division_ids,
			cookies_json = excluded.cookies_json,
			updated_at = excluded.updated_at
	`, acc.ID, acc.Username, acc.Mobile, acc.Token, acc.UserAgent, acc.DeviceID, acc.UUID, acc.Proxy, acc.AddressID, acc.DivisionIDs, string(cookiesJSON), acc.CreatedAt.UnixMilli(), acc.UpdatedAt.UnixMilli())
	if err != nil {
		return model.Account{}, err
	}

	return s.GetAccountByMobile(ctx, acc.Mobile)
}

func (s *Store) GetAccountByMobile(ctx context.Context, mobile string) (model.Account, error) {
	var row struct {
		id        string
		username  string
		mobile    string
		token     string
		userAgent string
		deviceID  string
		uuid      string
		proxy     string
		addressID int64
		divisionIDs string
		cookies   string
		createdAt int64
		updatedAt int64
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, mobile, token, user_agent, device_id, uuid, proxy, address_id, division_ids, cookies_json, created_at, updated_at
		FROM accounts WHERE mobile = ?
	`, mobile).Scan(&row.id, &row.username, &row.mobile, &row.token, &row.userAgent, &row.deviceID, &row.uuid, &row.proxy, &row.addressID, &row.divisionIDs, &row.cookies, &row.createdAt, &row.updatedAt)
	if err != nil {
		return model.Account{}, err
	}
	var cookies []model.CookieJarEntry
	_ = json.Unmarshal([]byte(row.cookies), &cookies)
	return model.Account{
		ID:        row.id,
		Username:  row.username,
		Mobile:    row.mobile,
		Token:     row.token,
		UserAgent: row.userAgent,
		DeviceID:  row.deviceID,
		UUID:      row.uuid,
		Proxy:     row.proxy,
		AddressID: row.addressID,
		DivisionIDs: row.divisionIDs,
		Cookies:   cookies,
		CreatedAt: time.UnixMilli(row.createdAt),
		UpdatedAt: time.UnixMilli(row.updatedAt),
	}, nil
}

func (s *Store) GetAccount(ctx context.Context, id string) (model.Account, error) {
	var row struct {
		id        string
		username  string
		mobile    string
		token     string
		userAgent string
		deviceID  string
		uuid      string
		proxy     string
		addressID int64
		divisionIDs string
		cookies   string
		createdAt int64
		updatedAt int64
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, mobile, token, user_agent, device_id, uuid, proxy, address_id, division_ids, cookies_json, created_at, updated_at
		FROM accounts WHERE id = ?
	`, id).Scan(&row.id, &row.username, &row.mobile, &row.token, &row.userAgent, &row.deviceID, &row.uuid, &row.proxy, &row.addressID, &row.divisionIDs, &row.cookies, &row.createdAt, &row.updatedAt)
	if err != nil {
		return model.Account{}, err
	}
	var cookies []model.CookieJarEntry
	_ = json.Unmarshal([]byte(row.cookies), &cookies)
	return model.Account{
		ID:        row.id,
		Username:  row.username,
		Mobile:    row.mobile,
		Token:     row.token,
		UserAgent: row.userAgent,
		DeviceID:  row.deviceID,
		UUID:      row.uuid,
		Proxy:     row.proxy,
		AddressID: row.addressID,
		DivisionIDs: row.divisionIDs,
		Cookies:   cookies,
		CreatedAt: time.UnixMilli(row.createdAt),
		UpdatedAt: time.UnixMilli(row.updatedAt),
	}, nil
}

func (s *Store) GetAccountByToken(ctx context.Context, token string) (model.Account, error) {
	if token == "" {
		return model.Account{}, errors.New("token is required")
	}
	var row struct {
		id        string
		username  string
		mobile    string
		token     string
		userAgent string
		deviceID  string
		uuid      string
		proxy     string
		addressID int64
		divisionIDs string
		cookies   string
		createdAt int64
		updatedAt int64
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, mobile, token, user_agent, device_id, uuid, proxy, address_id, division_ids, cookies_json, created_at, updated_at
		FROM accounts WHERE token = ? ORDER BY updated_at DESC LIMIT 1
	`, token).Scan(&row.id, &row.username, &row.mobile, &row.token, &row.userAgent, &row.deviceID, &row.uuid, &row.proxy, &row.addressID, &row.divisionIDs, &row.cookies, &row.createdAt, &row.updatedAt)
	if err != nil {
		return model.Account{}, fmt.Errorf("get account by token: %w", err)
	}
	var cookies []model.CookieJarEntry
	_ = json.Unmarshal([]byte(row.cookies), &cookies)
	return model.Account{
		ID:        row.id,
		Username:  row.username,
		Mobile:    row.mobile,
		Token:     row.token,
		UserAgent: row.userAgent,
		DeviceID:  row.deviceID,
		UUID:      row.uuid,
		Proxy:     row.proxy,
		AddressID: row.addressID,
		DivisionIDs: row.divisionIDs,
		Cookies:   cookies,
		CreatedAt: time.UnixMilli(row.createdAt),
		UpdatedAt: time.UnixMilli(row.updatedAt),
	}, nil
}

func (s *Store) ListAccounts(ctx context.Context) ([]model.Account, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, mobile, token, user_agent, device_id, uuid, proxy, address_id, division_ids, cookies_json, created_at, updated_at
		FROM accounts ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Account
	for rows.Next() {
		var row struct {
			id        string
			username  string
			mobile    string
			token     string
			userAgent string
			deviceID  string
			uuid      string
			proxy     string
			addressID int64
			divisionIDs string
			cookies   string
			createdAt int64
			updatedAt int64
		}
		if err := rows.Scan(&row.id, &row.username, &row.mobile, &row.token, &row.userAgent, &row.deviceID, &row.uuid, &row.proxy, &row.addressID, &row.divisionIDs, &row.cookies, &row.createdAt, &row.updatedAt); err != nil {
			return nil, err
		}
		var cookies []model.CookieJarEntry
		_ = json.Unmarshal([]byte(row.cookies), &cookies)
		out = append(out, model.Account{
			ID:        row.id,
			Username:  row.username,
			Mobile:    row.mobile,
			Token:     row.token,
			UserAgent: row.userAgent,
			DeviceID:  row.deviceID,
			UUID:      row.uuid,
			Proxy:     row.proxy,
			AddressID: row.addressID,
			DivisionIDs: row.divisionIDs,
			Cookies:   cookies,
			CreatedAt: time.UnixMilli(row.createdAt),
			UpdatedAt: time.UnixMilli(row.updatedAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) DeleteAccount(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM accounts WHERE id = ?`, id)
	return err
}
