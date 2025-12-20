package model

type TaskState struct {
	TargetID      string `json:"targetId"`
	Running       bool   `json:"running"`
	PurchasedQty  int    `json:"purchasedQty"`
	TargetQty     int    `json:"targetQty"`
	NeedCaptcha   *bool  `json:"needCaptcha,omitempty"`
	LastError     string `json:"lastError,omitempty"`
	LastAttemptMs int64  `json:"lastAttemptMs,omitempty"`
	LastSuccessMs int64  `json:"lastSuccessMs,omitempty"`
}

type EngineState struct {
	Running bool        `json:"running"`
	Tasks   []TaskState `json:"tasks"`
}
