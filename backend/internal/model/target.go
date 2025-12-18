package model

import "time"

type TargetMode string

const (
	TargetModeRush TargetMode = "rush"
	TargetModeScan TargetMode = "scan"
)

type Target struct {
	ID          string     `json:"id"`
	Name        string     `json:"name,omitempty"`
	ImageURL    string     `json:"imageUrl,omitempty"`
	ItemID      int64      `json:"itemId"`
	SKUID       int64      `json:"skuId"`
	ShopID      int64      `json:"shopId,omitempty"`
	Mode        TargetMode `json:"mode"`
	TargetQty   int        `json:"targetQty"`
	PerOrderQty int        `json:"perOrderQty"`
	RushAtMs    int64      `json:"rushAtMs,omitempty"`
	Enabled     bool       `json:"enabled"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}
