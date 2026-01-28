package engine

import (
	"strings"
	"time"

	"sniping_engine/internal/model"
)

func DefaultNotifySettings() model.NotifySettings {
	return model.NotifySettings{
		RushExpireDisableMinutes: 10,
		RushMode:                 "concurrent",
		RoundRobinIntervalMs:     120,
	}
}

func normalizeNotifySettings(in model.NotifySettings) model.NotifySettings {
	out := in
	if out.RushExpireDisableMinutes <= 0 {
		out.RushExpireDisableMinutes = 10
	}
	if out.RushExpireDisableMinutes > 1440 {
		out.RushExpireDisableMinutes = 1440
	}
	switch strings.ToLower(strings.TrimSpace(out.RushMode)) {
	case "round_robin":
		out.RushMode = "round_robin"
	default:
		out.RushMode = "concurrent"
	}
	if out.RoundRobinIntervalMs <= 0 {
		out.RoundRobinIntervalMs = 120
	}
	if out.RoundRobinIntervalMs < 50 {
		out.RoundRobinIntervalMs = 50
	}
	if out.RoundRobinIntervalMs > 2000 {
		out.RoundRobinIntervalMs = 2000
	}
	return out
}

func NormalizeNotifySettings(in model.NotifySettings) model.NotifySettings {
	return normalizeNotifySettings(in)
}

func (e *Engine) NotifySettings() model.NotifySettings {
	if e == nil {
		return DefaultNotifySettings()
	}
	v := e.notifySettings.Load()
	if v == nil {
		return DefaultNotifySettings()
	}
	if s, ok := v.(model.NotifySettings); ok {
		return normalizeNotifySettings(s)
	}
	return DefaultNotifySettings()
}

func (e *Engine) SetNotifySettings(next model.NotifySettings) model.NotifySettings {
	next = normalizeNotifySettings(next)
	if e == nil {
		return next
	}
	e.notifySettings.Store(next)
	return next
}

func (e *Engine) RushMode() string {
	st := e.NotifySettings()
	return strings.TrimSpace(st.RushMode)
}

func (e *Engine) RoundRobinInterval() time.Duration {
	st := e.NotifySettings()
	if st.RoundRobinIntervalMs <= 0 {
		return 120 * time.Millisecond
	}
	return time.Duration(st.RoundRobinIntervalMs) * time.Millisecond
}
