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

type CaptchaPoolSettings struct {
	// WarmupSeconds 抢购前多少秒开始维护验证码池。
	WarmupSeconds int `json:"warmupSeconds"`
	// PoolSize 验证码池目标数量（维护到这个数量）。
	PoolSize int `json:"poolSize"`
	// ItemTTLSeconds 每条验证码（verifyParam）从获取时刻开始的有效期（倒计时）。
	ItemTTLSeconds int `json:"itemTtlSeconds"`
}

type NotifySettings struct {
	// RushExpireDisableMinutes 抢购时间(rushAtMs)过去多少分钟后自动关闭监控（enabled=false）。
	RushExpireDisableMinutes int `json:"rushExpireDisableMinutes"`
}
