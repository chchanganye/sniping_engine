package model

import "time"

type Account struct {
	ID        string           `json:"id"`
	Username  string           `json:"username,omitempty"`
	Mobile    string           `json:"mobile"`
	Token     string           `json:"token,omitempty"`
	UserAgent string           `json:"userAgent,omitempty"`
	DeviceID  string           `json:"deviceId,omitempty"`
	UUID      string           `json:"uuid,omitempty"`
	Proxy     string           `json:"proxy,omitempty"`
	Cookies   []CookieJarEntry `json:"cookies,omitempty"`
	CreatedAt time.Time        `json:"createdAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
}
