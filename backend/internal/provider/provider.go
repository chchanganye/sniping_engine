package provider

import (
	"context"
	"encoding/json"

	"sniping_engine/internal/model"
)

type PreflightResult struct {
	CanBuy      bool            `json:"canBuy"`
	NeedCaptcha bool            `json:"needCaptcha,omitempty"`
	TotalFee    int64           `json:"totalFee"`
	TraceID     string          `json:"traceId,omitempty"`
	Render      json.RawMessage `json:"render,omitempty"`
}

type CreateResult struct {
	Success bool   `json:"success"`
	OrderID string `json:"orderId,omitempty"`
	TraceID string `json:"traceId,omitempty"`
}

type ShippingAddressParams struct {
	App        string `json:"app"`
	IsAllCover int    `json:"isAllCover"`
}

type CategoryTreeParams struct {
	FrontCategoryID int64   `json:"frontCategoryId"`
	Longitude       float64 `json:"longitude"`
	Latitude        float64 `json:"latitude"`
	IsFinish        bool    `json:"isFinish"`
}

type StoreSkuByCategoryParams struct {
	PageNo          int     `json:"pageNo"`
	PageSize        int     `json:"pageSize"`
	FrontCategoryID int64   `json:"frontCategoryId"`
	Longitude       float64 `json:"longitude"`
	Latitude        float64 `json:"latitude"`
	IsFinish        bool    `json:"isFinish"`
}

type Provider interface {
	Name() string

	LoginBySMS(ctx context.Context, account model.Account, mobile, smsCode string) (model.Account, error)
	Preflight(ctx context.Context, account model.Account, target model.Target) (PreflightResult, model.Account, error)
	CreateOrder(ctx context.Context, account model.Account, target model.Target, preflight PreflightResult) (CreateResult, model.Account, error)

	GetShippingAddresses(ctx context.Context, account model.Account, params ShippingAddressParams) (json.RawMessage, model.Account, error)
	GetCategoryTree(ctx context.Context, account model.Account, params CategoryTreeParams) (json.RawMessage, model.Account, error)
	GetStoreSkuByCategory(ctx context.Context, account model.Account, params StoreSkuByCategoryParams) (json.RawMessage, model.Account, error)
}
