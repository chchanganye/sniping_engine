package engine

import "sniping_engine/internal/model"

func DefaultNotifySettings() model.NotifySettings {
	return model.NotifySettings{
		RushExpireDisableMinutes: 10,
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
	return out
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
