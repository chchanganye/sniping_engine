package provider

import (
	"context"

	"sniping_engine/internal/model"
)

type PreflightResult struct {
	CanBuy   bool   `json:"canBuy"`
	TotalFee int64  `json:"totalFee"`
	TraceID  string `json:"traceId,omitempty"`
}

type CreateResult struct {
	Success bool   `json:"success"`
	OrderID string `json:"orderId,omitempty"`
	TraceID string `json:"traceId,omitempty"`
}

type Provider interface {
	Name() string

	LoginBySMS(ctx context.Context, account model.Account, mobile, smsCode string) (model.Account, error)
	Preflight(ctx context.Context, account model.Account, target model.Target) (PreflightResult, model.Account, error)
	CreateOrder(ctx context.Context, account model.Account, target model.Target, preflight PreflightResult) (CreateResult, model.Account, error)
}

