package config

import (
	"errors"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Storage  StorageConfig  `yaml:"storage"`
	Proxy    ProxyConfig    `yaml:"proxy"`
	Limits   LimitsConfig   `yaml:"limits"`
	Task     TaskConfig     `yaml:"task"`
	Provider ProviderConfig `yaml:"provider"`
}

type ServerConfig struct {
	Addr string     `yaml:"addr"`
	Cors CorsConfig `yaml:"cors"`
}

type CorsConfig struct {
	AllowOrigins     []string `yaml:"allowOrigins"`
	AllowCredentials bool     `yaml:"allowCredentials"`
}

type StorageConfig struct {
	SQLitePath string `yaml:"sqlitePath"`
}

type ProxyConfig struct {
	Global string `yaml:"global"`
}

type LimitsConfig struct {
	GlobalQPS       float64 `yaml:"globalQPS"`
	GlobalBurst     int     `yaml:"globalBurst"`
	PerAccountQPS   float64 `yaml:"perAccountQPS"`
	PerAccountBurst int     `yaml:"perAccountBurst"`
	MaxInFlight     int     `yaml:"maxInFlight"`
	// MaxPerTargetInFlight 控制同一个商品/任务在同一时间最多允许多少个账号并发尝试下单。
	// 默认 1，保持原来的“单目标串行”行为。
	MaxPerTargetInFlight int `yaml:"maxPerTargetInFlight"`
	// CaptchaMaxInFlight 控制验证码求解（无头浏览器）的并发数上限。
	// 默认 1，避免小机器 CPU/内存被打满。
	CaptchaMaxInFlight int `yaml:"captchaMaxInFlight"`
}

type TaskConfig struct {
	RushIntervalMs int `yaml:"rushIntervalMs"`
	ScanIntervalMs int `yaml:"scanIntervalMs"`
}

func (c TaskConfig) RushInterval() time.Duration {
	if c.RushIntervalMs <= 0 {
		return 200 * time.Millisecond
	}
	return time.Duration(c.RushIntervalMs) * time.Millisecond
}

func (c TaskConfig) ScanInterval() time.Duration {
	if c.ScanIntervalMs <= 0 {
		return 1 * time.Second
	}
	return time.Duration(c.ScanIntervalMs) * time.Millisecond
}

type ProviderConfig struct {
	BaseURL    string           `yaml:"baseURL"`
	TimeoutMs  int              `yaml:"timeoutMs"`
	Retry      ProviderRetryCfg `yaml:"retry"`
	UserAgent  string           `yaml:"userAgent"`
	DeviceID   string           `yaml:"deviceId"`
	DeviceType string           `yaml:"deviceType"`
}

type ProviderRetryCfg struct {
	Count     int `yaml:"count"`
	WaitMs    int `yaml:"waitMs"`
	MaxWaitMs int `yaml:"maxWaitMs"`
}

func (c ProviderConfig) Timeout() time.Duration {
	if c.TimeoutMs <= 0 {
		return 20 * time.Second
	}
	return time.Duration(c.TimeoutMs) * time.Millisecond
}

func (c ProviderRetryCfg) Wait() time.Duration {
	if c.WaitMs <= 0 {
		return 200 * time.Millisecond
	}
	return time.Duration(c.WaitMs) * time.Millisecond
}

func (c ProviderRetryCfg) MaxWait() time.Duration {
	if c.MaxWaitMs <= 0 {
		return 1200 * time.Millisecond
	}
	return time.Duration(c.MaxWaitMs) * time.Millisecond
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Addr == "" {
		c.Server.Addr = ":8090"
	}
	if c.Storage.SQLitePath == "" {
		c.Storage.SQLitePath = "./data/sniping_engine.db"
	}
	if c.Limits.GlobalBurst <= 0 {
		c.Limits.GlobalBurst = 10
	}
	if c.Limits.PerAccountBurst <= 0 {
		c.Limits.PerAccountBurst = 2
	}
	if c.Limits.MaxInFlight <= 0 {
		c.Limits.MaxInFlight = 20
	}
	if c.Limits.MaxPerTargetInFlight <= 0 {
		c.Limits.MaxPerTargetInFlight = 1
	}
	if c.Limits.CaptchaMaxInFlight <= 0 {
		c.Limits.CaptchaMaxInFlight = 1
	}
	if c.Provider.BaseURL == "" {
		c.Provider.BaseURL = "http://127.0.0.1:8080/mock"
	}
	if c.Provider.UserAgent == "" {
		// 默认用“手机端/微信小程序”UA，避免被上游识别为 PC
		c.Provider.UserAgent = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 MicroMessenger/8.0.66(0x18004235) NetType/WIFI Language/zh_CN"
	}
	if c.Provider.DeviceType == "" {
		c.Provider.DeviceType = "WXAPP"
	}
	if c.Provider.Retry.Count < 0 {
		c.Provider.Retry.Count = 0
	}
}

func (c Config) validate() error {
	if c.Server.Addr == "" {
		return errors.New("server.addr is required")
	}
	if c.Provider.BaseURL == "" {
		return errors.New("provider.baseURL is required")
	}
	return nil
}
