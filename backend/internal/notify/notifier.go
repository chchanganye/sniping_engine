package notify

import "context"

type OrderCreatedEvent struct {
	At         int64  `json:"atMs"`
	AccountID  string `json:"accountId"`
	Mobile     string `json:"mobile,omitempty"`
	TargetID   string `json:"targetId"`
	TargetName string `json:"targetName,omitempty"`
	Mode       string `json:"mode,omitempty"`
	ItemID     int64  `json:"itemId,omitempty"`
	SKUID      int64  `json:"skuId,omitempty"`
	ShopID     int64  `json:"shopId,omitempty"`
	Quantity   int    `json:"quantity,omitempty"`
	OrderID    string `json:"orderId,omitempty"`
	TraceID    string `json:"traceId,omitempty"`
}

type Notifier interface {
	NotifyOrderCreated(ctx context.Context, evt OrderCreatedEvent)
}

