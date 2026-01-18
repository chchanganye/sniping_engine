package engine

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"sniping_engine/internal/model"
	"sniping_engine/internal/utils"
)

var seedRandOnce sync.Once

func seedRand() {
	seedRandOnce.Do(func() {
		rand.Seed(time.Now().UnixNano())
	})
}

func (e *Engine) SetCaptchaPoolSettings(v model.CaptchaPoolSettings) model.CaptchaPoolSettings {
	if e == nil || e.captchaPool == nil {
		return normalizeCaptchaPoolSettings(v)
	}
	saved := e.captchaPool.SetSettings(v)
	e.recalcCaptchaPoolActivateAtMs()
	return saved
}

func (e *Engine) CaptchaPoolStatus() CaptchaPoolStatus {
	nowMs := time.Now().UnixMilli()
	st := DefaultCaptchaPoolSettings()
	items := []CaptchaPoolItemView(nil)
	if e != nil && e.captchaPool != nil {
		st = e.captchaPool.Settings()
		items = e.captchaPool.Snapshot(nowMs)
	}
	activated := false
	activateAt := int64(0)
	if e != nil {
		activated = e.captchaPoolActivated.Load()
		activateAt = e.captchaPoolActivateAtMs.Load()
	}
	return CaptchaPoolStatus{
		NowMs:        nowMs,
		Activated:    activated,
		ActivateAtMs: activateAt,
		DesiredSize:  st.PoolSize,
		Size:         len(items),
		Settings:     st,
		Items:        items,
	}
}

func (e *Engine) startCaptchaPoolMaintainer(ctx context.Context) {
	if e == nil {
		return
	}
	if !e.captchaPoolMaintainerRunning.CompareAndSwap(false, true) {
		return
	}
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer e.captchaPoolMaintainerRunning.Store(false)
		ticker := time.NewTicker(800 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e.tickCaptchaPool(ctx)
			}
		}
	}()
}

func (e *Engine) tickCaptchaPool(ctx context.Context) {
	nowMs := time.Now().UnixMilli()
	activateAtMs := e.captchaPoolActivateAtMs.Load()
	if !e.captchaPoolActivated.Load() && activateAtMs > 0 && nowMs >= activateAtMs {
		e.captchaPoolActivated.Store(true)
		if e.bus != nil {
			e.bus.Log("info", "验证码池开始维护", map[string]any{
				"activateAtMs": activateAtMs,
				"nowMs":        nowMs,
			})
		}
	}

	if !e.captchaPoolActivated.Load() {
		return
	}

	settings := e.captchaPool.Settings()
	desired := settings.PoolSize
	if desired <= 0 {
		return
	}

	size := e.captchaPool.Size(nowMs)
	missing := desired - size
	if missing <= 0 {
		return
	}

	fillCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	_, _, _ = e.FillCaptchaPool(fillCtx, missing)
}

func (e *Engine) recalcCaptchaPoolActivateAtMs() {
	if e == nil {
		return
	}

	st := DefaultCaptchaPoolSettings()
	if e.captchaPool != nil {
		st = e.captchaPool.Settings()
	}
	warmupMs := int64(st.WarmupSeconds) * 1000

	e.mu.Lock()
	targets := append([]model.Target(nil), e.targets...)
	e.mu.Unlock()

	var minActivateAt int64
	for _, t := range targets {
		if t.Mode != model.TargetModeRush || t.RushAtMs <= 0 {
			continue
		}
		at := t.RushAtMs - warmupMs
		if minActivateAt == 0 || at < minActivateAt {
			minActivateAt = at
		}
	}
	if minActivateAt <= 0 {
		return
	}

	e.captchaPoolActivateAtMs.Store(minActivateAt)
	if time.Now().UnixMilli() >= minActivateAt {
		e.captchaPoolActivated.Store(true)
	}
}

func (e *Engine) FillCaptchaPool(ctx context.Context, count int) (added int, failed int, err error) {
	return e.fillCaptchaPool(ctx, count, false)
}

func (e *Engine) FillCaptchaPoolManual(ctx context.Context, count int) (added int, failed int, err error) {
	if e != nil && e.bus != nil {
		e.bus.Log("info", "验证码池：手动补充开始", map[string]any{"count": count})
	}
	added, failed, err = e.fillCaptchaPool(ctx, count, true)
	if e != nil && e.bus != nil {
		fields := map[string]any{"added": added, "failed": failed}
		if err != nil {
			fields["error"] = err.Error()
			e.bus.Log("warn", "验证码池：手动补充失败", fields)
		} else {
			fields["size"] = e.captchaPool.Size(time.Now().UnixMilli())
			e.bus.Log("info", "验证码池：手动补充完成", fields)
		}
	}
	return added, failed, err
}

func (e *Engine) fillCaptchaPool(ctx context.Context, count int, manual bool) (added int, failed int, err error) {
	if e == nil || e.captchaPool == nil {
		return 0, 0, errors.New("engine unavailable")
	}
	if count <= 0 {
		return 0, 0, nil
	}
	if count > 50 {
		count = 50
	}

	if _, err := utils.EnsureCaptchaEngineReady(ctx, 0); err != nil {
		return 0, 0, err
	}

	desiredPages := utils.GetCaptchaMaxConcurrent()
	if desiredPages <= 0 {
		desiredPages = 1
	}
	if desiredPages > count {
		desiredPages = count
	}
	if err := utils.EnsureCaptchaPagePool(ctx, desiredPages); err != nil {
		return 0, 0, err
	}
	if manual {
		_, _ = utils.RefreshCaptchaPages(ctx, utils.CaptchaPagesRefreshOptions{EnsurePages: desiredPages})
	}

	dracoToken, _ := e.pickDracoToken(ctx)

	type result struct {
		param      string
		solvedAtMs int64
		metrics    utils.CaptchaSolveMetrics
		err        error
	}
	out := make(chan result, count)

	var wg sync.WaitGroup
	wg.Add(count)
	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			ts := time.Now().UnixMilli()
			param, metrics, solveErr := utils.SolveAliyunCaptchaWithMetrics(ctx, ts, dracoToken)
			out <- result{param: strings.TrimSpace(param), solvedAtMs: time.Now().UnixMilli(), metrics: metrics, err: solveErr}
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	for r := range out {
		if r.err != nil || strings.TrimSpace(r.param) == "" {
			failed++
			if e.bus != nil {
				msg := "captcha solve failed"
				if r.err != nil {
					msg = r.err.Error()
				}
				e.bus.Log("warn", "验证码池：生成失败", map[string]any{
					"error":    msg,
					"attempts": r.metrics.Attempts,
					"costMs":   r.metrics.Duration.Milliseconds(),
				})
			}
			continue
		}
		if _, ok := e.captchaPool.Add(r.param, r.solvedAtMs); ok {
			added++
		} else {
			failed++
		}
	}

	return added, failed, nil
}

func (e *Engine) AddCaptchaVerifyParamManual(verifyParam string) (bool, error) {
	if e == nil || e.captchaPool == nil {
		return false, errors.New("engine unavailable")
	}
	param := strings.TrimSpace(verifyParam)
	if param == "" {
		return false, errors.New("verifyParam is required")
	}
	if _, ok := e.captchaPool.Add(param, time.Now().UnixMilli()); !ok {
		return false, errors.New("failed to add verifyParam")
	}
	if e.bus != nil {
		e.bus.Log("info", "验证码池：人工补充完成", map[string]any{
			"added": 1,
			"size":  e.captchaPool.Size(time.Now().UnixMilli()),
		})
	}
	return true, nil
}

func (e *Engine) AcquireCaptchaVerifyParam(ctx context.Context) (string, bool) {
	if e == nil || e.captchaPool == nil {
		return "", false
	}
	it, ok := e.captchaPool.Acquire(ctx)
	if !ok || strings.TrimSpace(it.VerifyParam) == "" {
		return "", false
	}
	return strings.TrimSpace(it.VerifyParam), true
}

func (e *Engine) pickDracoToken(ctx context.Context) (string, string) {
	seedRand()
	accounts := []model.Account(nil)

	e.mu.Lock()
	if len(e.accounts) > 0 {
		accounts = append(accounts, e.accounts...)
	}
	e.mu.Unlock()

	if len(accounts) == 0 && e.store != nil {
		if stored, err := e.store.ListAccounts(ctx); err == nil {
			accounts = filterLoggedInAccounts(stored)
		}
	}
	if len(accounts) == 0 {
		return "", ""
	}

	acc := accounts[rand.Intn(len(accounts))]
	dracoToken := extractDracoToken(acc)
	return dracoToken, acc.ID
}

func extractDracoToken(acc model.Account) string {
	for _, cookieEntry := range acc.Cookies {
		for _, cookie := range cookieEntry.Cookies {
			if cookie.Name == "draco_local" {
				return cookie.Value
			}
		}
	}
	return ""
}

func (e *Engine) captchaVerifyParamForOrder(ctx context.Context, acc model.Account, target model.Target, needCaptcha bool) (string, bool, error) {
	if !needCaptcha {
		return "", false, nil
	}
	if v := strings.TrimSpace(target.CaptchaVerifyParam); v != "" {
		return v, false, nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if v, ok := e.AcquireCaptchaVerifyParam(waitCtx); ok {
		return v, true, nil
	}

	dracoToken := extractDracoToken(acc)
	if _, err := utils.EnsureCaptchaEngineReady(ctx, 0); err != nil {
		return "", false, err
	}
	ts := time.Now().UnixMilli()
	verifyParam, metrics, err := utils.SolveAliyunCaptchaWithMetrics(ctx, ts, dracoToken)
	if err != nil {
		if e.bus != nil {
			e.bus.Log("warn", "验证码处理失败", map[string]any{
				"accountId": acc.ID,
				"targetId":  target.ID,
				"attempts":  metrics.Attempts,
				"costMs":    metrics.Duration.Milliseconds(),
				"error":     err.Error(),
			})
		}
		return "", false, fmt.Errorf("failed to solve captcha: %w", err)
	}
	verifyParam = strings.TrimSpace(verifyParam)
	if verifyParam == "" {
		return "", false, errors.New("captcha solving returned empty result")
	}
	return verifyParam, false, nil
}
