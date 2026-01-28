package engine

import (
	"context"
	"strings"
	"time"

	"sniping_engine/internal/model"
)

func (e *Engine) shouldDisableRushTargetNow(target model.Target, nowMs int64) (expired bool, expireAtMs int64, expireMinutes int) {
	if e == nil {
		return false, 0, 0
	}
	if target.Mode != model.TargetModeRush || target.RushAtMs <= 0 {
		return false, 0, 0
	}

	st := e.NotifySettings()
	expireMinutes = st.RushExpireDisableMinutes
	if expireMinutes <= 0 {
		return false, 0, 0
	}
	expireAtMs = target.RushAtMs + int64(expireMinutes)*60*1000
	return nowMs >= expireAtMs, expireAtMs, expireMinutes
}

func (e *Engine) disableTargetAsync(targetID string, reason string, fields map[string]any) {
	go e.disableTarget(targetID, reason, fields)
}

func (e *Engine) disableTarget(targetID string, reason string, fields map[string]any) {
	targetID = strings.TrimSpace(targetID)
	if e == nil || targetID == "" {
		return
	}

	if e.bus != nil {
		out := map[string]any{"targetId": targetID}
		if strings.TrimSpace(reason) != "" {
			out["reason"] = strings.TrimSpace(reason)
		}
		for k, v := range fields {
			if strings.TrimSpace(k) == "" || v == nil {
				continue
			}
			out[k] = v
		}
		e.bus.Log("info", "任务已自动关闭", out)
		e.bus.Publish("target_disabled", out)
	}

	if e.store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = e.store.SetTargetEnabled(ctx, targetID, false)
		cancel()
	}

	var cancel context.CancelFunc
	shouldStop := false
	nowMs := time.Now().UnixMilli()

	e.mu.Lock()
	if e.targetCancels != nil {
		cancel = e.targetCancels[targetID]
		delete(e.targetCancels, targetID)
	}
	if e.targetSnapshots != nil {
		delete(e.targetSnapshots, targetID)
	}
	if len(e.targets) > 0 {
		n := 0
		for _, t := range e.targets {
			if strings.TrimSpace(t.ID) == targetID {
				continue
			}
			e.targets[n] = t
			n++
		}
		e.targets = e.targets[:n]
	}
	if st := e.states[targetID]; st != nil {
		st.Running = false
		st.LastAttemptMs = nowMs
		e.publishStateLocked(*st)
	}
	shouldStop = e.running && (e.targetCancels == nil || len(e.targetCancels) == 0)
	e.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	e.recalcCaptchaPoolActivateAtMs()

	if shouldStop {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = e.StopAll(ctx)
			cancel()
		}()
	}
}
