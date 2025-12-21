package model

type EmailSettings struct {
	Enabled  bool   `json:"enabled"`
	Email    string `json:"email"`
	AuthCode string `json:"authCode,omitempty"`
}

type LimitsSettings struct {
	MaxPerTargetInFlight int `json:"maxPerTargetInFlight"`
	CaptchaMaxInFlight   int `json:"captchaMaxInFlight"`
}
