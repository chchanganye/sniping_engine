package utils

import "strings"

const defaultWXAppUserAgent = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 MicroMessenger/8.0.66(0x18004235) NetType/WIFI Language/zh_CN"

// DefaultWXAppUserAgent 返回默认的“微信小程序/手机端”UA。
func DefaultWXAppUserAgent() string {
	return defaultWXAppUserAgent
}

// NormalizeWXAppUserAgent 把 UA 规范为“手机端”风格；当入参为空或不像手机 UA 时，返回默认 UA。
func NormalizeWXAppUserAgent(ua string) string {
	v := strings.TrimSpace(ua)
	if v == "" {
		return defaultWXAppUserAgent
	}
	if looksLikeMobileUA(v) {
		return v
	}
	return defaultWXAppUserAgent
}

func looksLikeMobileUA(ua string) bool {
	s := strings.ToLower(ua)
	if strings.Contains(s, "micromessenger") {
		return true
	}
	if strings.Contains(s, "mobile") {
		return true
	}
	if strings.Contains(s, "iphone") || strings.Contains(s, "android") || strings.Contains(s, "ipad") {
		return true
	}
	return false
}

