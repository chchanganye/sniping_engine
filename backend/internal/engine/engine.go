package engine

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"sniping_engine/internal/config"
	"sniping_engine/internal/logbus"
	"sniping_engine/internal/model"
	"sniping_engine/internal/notify"
	"sniping_engine/internal/provider"
	"sniping_engine/internal/store/sqlite"
)

type Options struct {
	Store    *sqlite.Store
	Provider provider.Provider
	Bus      *logbus.Bus
	Limits   config.LimitsConfig
	Task     config.TaskConfig
	Notifier notify.Notifier
}

type Engine struct {
	store    *sqlite.Store
	provider provider.Provider
	bus      *logbus.Bus
	notifier notify.Notifier

	limits config.LimitsConfig
	task   config.TaskConfig

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	states  map[string]*model.TaskState

	accounts []model.Account
	targets  []model.Target

	globalLimiter *rate.Limiter
	perLimiter    map[string]*rate.Limiter
	inFlight      chan struct{}
	accountLocks  map[string]chan struct{}

	rr atomic.Uint64
}

type TestBuyResult struct {
	CanBuy      bool   `json:"canBuy"`
	NeedCaptcha bool   `json:"needCaptcha,omitempty"`
	Success     bool   `json:"success"`
	OrderID     string `json:"orderId,omitempty"`
	TraceID     string `json:"traceId,omitempty"`
	Message     string `json:"message,omitempty"`
}

type PreflightCheckResult struct {
	CanBuy      bool   `json:"canBuy"`
	NeedCaptcha bool   `json:"needCaptcha"`
	TotalFee    int64  `json:"totalFee"`
	TraceID     string `json:"traceId,omitempty"`
	Message     string `json:"message,omitempty"`
}

func New(opts Options) *Engine {
	maxInFlight := opts.Limits.MaxInFlight
	if maxInFlight <= 0 {
		maxInFlight = 20
	}

	globalBurst := opts.Limits.GlobalBurst
	if globalBurst <= 0 {
		globalBurst = 10
	}
	globalQPS := opts.Limits.GlobalQPS
	if globalQPS <= 0 {
		globalQPS = 5
	}

	return &Engine{
		store:         opts.Store,
		provider:      opts.Provider,
		bus:           opts.Bus,
		notifier:      opts.Notifier,
		limits:        opts.Limits,
		task:          opts.Task,
		states:        make(map[string]*model.TaskState),
		perLimiter:    make(map[string]*rate.Limiter),
		inFlight:      make(chan struct{}, maxInFlight),
		accountLocks:  make(map[string]chan struct{}),
		globalLimiter: rate.NewLimiter(rate.Limit(globalQPS), globalBurst),
	}
}

func (e *Engine) StartAll(ctx context.Context) error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return nil
	}
	e.running = true
	runCtx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.mu.Unlock()

	if e.bus != nil {
		e.bus.Log("info", "引擎已启动", map[string]any{"provider": e.provider.Name()})
	}

	accounts, err := e.store.ListAccounts(ctx)
	if err != nil {
		_ = e.StopAll(ctx)
		return err
	}
	accounts = filterLoggedInAccounts(accounts)
	if len(accounts) == 0 {
		_ = e.StopAll(ctx)
		return errors.New("no logged-in accounts in storage")
	}
	targets, err := e.store.ListEnabledTargets(ctx)
	if err != nil {
		_ = e.StopAll(ctx)
		return err
	}
	if len(targets) == 0 {
		_ = e.StopAll(ctx)
		return errors.New("no enabled targets in storage")
	}

	perQPS := e.limits.PerAccountQPS
	if perQPS <= 0 {
		perQPS = 1
	}
	perBurst := e.limits.PerAccountBurst
	if perBurst <= 0 {
		perBurst = 2
	}

	e.mu.Lock()
	e.accounts = accounts
	e.targets = targets
	e.perLimiter = make(map[string]*rate.Limiter)
	e.accountLocks = make(map[string]chan struct{})
	for _, acc := range accounts {
		e.perLimiter[acc.ID] = rate.NewLimiter(rate.Limit(perQPS), perBurst)
		e.accountLocks[acc.ID] = make(chan struct{}, 1)
	}
	for _, t := range targets {
		state := &model.TaskState{
			TargetID:     t.ID,
			Running:      true,
			PurchasedQty: 0,
			TargetQty:    t.TargetQty,
		}
		e.states[t.ID] = state
		e.publishStateLocked(*state)
		e.wg.Add(1)
		go e.runTarget(runCtx, t)
	}
	e.mu.Unlock()
	return nil
}

func (e *Engine) StopAll(ctx context.Context) error {
	e.mu.Lock()
	cancel := e.cancel
	e.cancel = nil
	wasRunning := e.running
	e.running = false
	e.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if !wasRunning {
		return nil
	}

	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if e.bus != nil {
			e.bus.Log("info", "引擎已停止", nil)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Engine) State() model.EngineState {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := model.EngineState{Running: e.running}
	for _, st := range e.states {
		out.Tasks = append(out.Tasks, *st)
	}
	return out
}

func (e *Engine) runTarget(ctx context.Context, target model.Target) {
	defer e.wg.Done()
	defer func() {
		e.mu.Lock()
		st := e.states[target.ID]
		if st != nil {
			st.Running = false
			e.publishStateLocked(*st)
		}
		e.mu.Unlock()
	}()

	if target.Mode == model.TargetModeRush && target.RushAtMs > 0 {
		startAt := time.UnixMilli(target.RushAtMs)
		if e.bus != nil {
			e.bus.Log("info", "等待开抢时间", map[string]any{
				"targetId": target.ID,
				"startAt":  startAt.Format(time.RFC3339Nano),
			})
		}
		if !sleepUntil(ctx, startAt) {
			return
		}
	}

	interval := e.task.ScanInterval()
	if target.Mode == model.TargetModeRush {
		interval = e.task.RushInterval()
	}

	e.attemptOnce(ctx, target)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.attemptOnce(ctx, target)
		}
	}
}

func (e *Engine) attemptOnce(ctx context.Context, target model.Target) {
	acc := e.pickAccount()
	if acc.ID == "" {
		return
	}
	// Refresh latest account snapshot to keep cookies/token/proxy/UA consistent with browsing sessions.
	if e.store != nil {
		if latest, err := e.store.GetAccount(ctx, acc.ID); err == nil {
			acc = latest
		}
	}

	e.mu.Lock()
	st := e.states[target.ID]
	if st == nil {
		st = &model.TaskState{TargetID: target.ID, Running: true, TargetQty: target.TargetQty}
		e.states[target.ID] = st
	}
	if st.PurchasedQty >= st.TargetQty {
		st.Running = false
		e.publishStateLocked(*st)
		e.mu.Unlock()
		return
	}
	st.LastAttemptMs = time.Now().UnixMilli()
	e.publishStateLocked(*st)
	e.mu.Unlock()

	if !e.acquireAccount(ctx, acc.ID) {
		return
	}
	defer e.releaseAccount(acc.ID)

	if !e.acquireInFlight(ctx) {
		return
	}
	defer e.releaseInFlight()

	if strings.TrimSpace(acc.Token) == "" {
		return
	}

	if !e.waitLimits(ctx, acc.ID) {
		return
	}

	pre, updatedAcc, err := e.provider.Preflight(ctx, acc, target)
	if err != nil {
		e.setError(target.ID, err)
		return
	}
	_ = e.persistAccount(ctx, updatedAcc)

	e.mu.Lock()
	if st := e.states[target.ID]; st != nil {
		v := pre.NeedCaptcha
		st.NeedCaptcha = &v
		e.publishStateLocked(*st)
	}
	e.mu.Unlock()

	if !pre.CanBuy {
		if e.bus != nil {
			e.bus.Log("debug", "预下单结果：不可购买", map[string]any{
				"targetId":  target.ID,
				"accountId": acc.ID,
				"traceId":   pre.TraceID,
			})
		}
		return
	}

	if !e.waitLimits(ctx, acc.ID) {
		return
	}

	res, updatedAcc2, err := e.provider.CreateOrder(ctx, acc, target, pre)
	if err != nil {
		e.setError(target.ID, err)
		return
	}
	_ = e.persistAccount(ctx, updatedAcc2)

	if res.Success {
		e.mu.Lock()
		st := e.states[target.ID]
		if st != nil {
			st.PurchasedQty += target.PerOrderQty
			st.LastSuccessMs = time.Now().UnixMilli()
			st.LastError = ""
			e.publishStateLocked(*st)
		}
		e.mu.Unlock()
		if e.bus != nil {
			e.bus.Log("info", "下单成功", map[string]any{
				"targetId":  target.ID,
				"accountId": acc.ID,
				"orderId":   res.OrderID,
				"traceId":   res.TraceID,
			})
		}
		if e.notifier != nil {
			e.notifier.NotifyOrderCreated(ctx, notify.OrderCreatedEvent{
				At:         time.Now().UnixMilli(),
				AccountID:  acc.ID,
				Mobile:     acc.Mobile,
				TargetID:   target.ID,
				TargetName: target.Name,
				Mode:       string(target.Mode),
				ItemID:     target.ItemID,
				SKUID:      target.SKUID,
				ShopID:     target.ShopID,
				Quantity:   target.PerOrderQty,
				OrderID:    res.OrderID,
				TraceID:    res.TraceID,
			})
		}
	}
}

func (e *Engine) TestBuyOnce(ctx context.Context, targetID string, captchaVerifyParam string, opID string) (TestBuyResult, error) {
	opID = strings.TrimSpace(opID)
	if len(opID) > 120 {
		opID = opID[:120]
	}
	accountID := ""
	progress := func(step, phase, message string, fields map[string]any) {
		if opID == "" || e.bus == nil {
			return
		}
		if fields == nil {
			fields = map[string]any{}
		}
		e.bus.Publish("progress", logbus.ProgressData{
			OpID:      opID,
			Kind:      "test_buy",
			Step:      strings.TrimSpace(step),
			Phase:     strings.TrimSpace(phase),
			Message:   strings.TrimSpace(message),
			TargetID:  strings.TrimSpace(targetID),
			AccountID: strings.TrimSpace(accountID),
			Fields:    fields,
		})
	}

	progress("start", "start", "开始测试抢购", nil)
	if e.store == nil {
		progress("init", "error", "store unavailable", nil)
		return TestBuyResult{}, errors.New("store unavailable")
	}
	if e.provider == nil {
		progress("init", "error", "provider unavailable", nil)
		return TestBuyResult{}, errors.New("provider unavailable")
	}
	target, err := e.store.GetTarget(ctx, targetID)
	if err != nil {
		progress("load_target", "error", err.Error(), nil)
		return TestBuyResult{}, err
	}
	if strings.TrimSpace(captchaVerifyParam) != "" {
		target.CaptchaVerifyParam = strings.TrimSpace(captchaVerifyParam)
	}
	progress("load_target", "success", "已加载目标配置", map[string]any{
		"name":   target.Name,
		"itemId": target.ItemID,
		"skuId":  target.SKUID,
		"shopId": target.ShopID,
	})

	accounts, err := e.store.ListAccounts(ctx)
	if err != nil {
		progress("load_accounts", "error", err.Error(), nil)
		return TestBuyResult{}, err
	}
	accounts = filterLoggedInAccounts(accounts)
	if len(accounts) == 0 {
		progress("load_accounts", "error", "没有已登录账号（缺少 token/cookie）", nil)
		return TestBuyResult{}, errors.New("no logged-in accounts")
	}

	n := e.rr.Add(1)
	acc := accounts[int(n-1)%len(accounts)]
	if latest, err := e.store.GetAccount(ctx, acc.ID); err == nil {
		acc = latest
	}
	accountID = acc.ID
	progress("select_account", "success", "已选择账号", map[string]any{
		"mobile": acc.Mobile,
	})

	e.ensureAccountLimiter(acc.ID)

	e.mu.Lock()
	st := e.states[target.ID]
	if st == nil {
		st = &model.TaskState{TargetID: target.ID, Running: false, TargetQty: target.TargetQty}
		e.states[target.ID] = st
	}
	st.LastAttemptMs = time.Now().UnixMilli()
	e.publishStateLocked(*st)
	e.mu.Unlock()

	if !e.acquireAccount(ctx, acc.ID) {
		return TestBuyResult{}, ctx.Err()
	}
	defer e.releaseAccount(acc.ID)

	if !e.acquireInFlight(ctx) {
		return TestBuyResult{}, ctx.Err()
	}
	defer e.releaseInFlight()

	if !e.waitLimits(ctx, acc.ID) {
		progress("limits", "error", "等待限速失败", nil)
		return TestBuyResult{}, ctx.Err()
	}

	progress("render_order", "start", "请求 render-order", map[string]any{"api": "/api/trade/buy/render-order"})
	pre, updatedAcc, err := e.provider.Preflight(ctx, acc, target)
	if err != nil {
		e.setError(target.ID, err)
		progress("render_order", "error", err.Error(), nil)
		return TestBuyResult{}, err
	}
	_ = e.persistAccount(ctx, updatedAcc)
	acc = updatedAcc
	progress("render_order", "success", "render-order 返回", map[string]any{
		"canBuy":      pre.CanBuy,
		"needCaptcha": pre.NeedCaptcha,
		"totalFee":    pre.TotalFee,
		"traceId":     pre.TraceID,
	})

	e.mu.Lock()
	if st := e.states[target.ID]; st != nil {
		v := pre.NeedCaptcha
		st.NeedCaptcha = &v
		e.publishStateLocked(*st)
	}
	e.mu.Unlock()

	if !pre.CanBuy {
		progress("done", "warning", "当前不可购买，结束", map[string]any{
			"canBuy":      pre.CanBuy,
			"needCaptcha": pre.NeedCaptcha,
		})
		return TestBuyResult{CanBuy: false, NeedCaptcha: pre.NeedCaptcha, Success: false, TraceID: pre.TraceID, Message: "当前不可购买"}, nil
	}

	if pre.NeedCaptcha {
		progress("captcha", "info", "需要验证码，将自动处理", nil)
	}

	if !e.waitLimits(ctx, acc.ID) {
		progress("limits", "error", "等待限速失败", nil)
		return TestBuyResult{}, ctx.Err()
	}

	progress("create_order", "start", "请求 create-order", map[string]any{"api": "/api/trade/buy/create-order"})
	res, updatedAcc2, err := e.provider.CreateOrder(ctx, acc, target, pre)
	if err != nil {
		e.setError(target.ID, err)
		progress("create_order", "error", err.Error(), nil)
		return TestBuyResult{}, err
	}
	_ = e.persistAccount(ctx, updatedAcc2)
	progress("create_order", "success", "create-order 成功", map[string]any{
		"orderId": res.OrderID,
		"traceId": res.TraceID,
	})

	if res.Success {
		e.mu.Lock()
		st := e.states[target.ID]
		if st != nil {
			st.PurchasedQty += target.PerOrderQty
			st.LastSuccessMs = time.Now().UnixMilli()
			st.LastError = ""
			e.publishStateLocked(*st)
		}
		e.mu.Unlock()
		if e.bus != nil {
			e.bus.Log("info", "测试下单成功", map[string]any{
				"targetId":  target.ID,
				"accountId": acc.ID,
				"orderId":   res.OrderID,
				"traceId":   res.TraceID,
			})
		}
		if e.notifier != nil {
			e.notifier.NotifyOrderCreated(ctx, notify.OrderCreatedEvent{
				At:         time.Now().UnixMilli(),
				AccountID:  acc.ID,
				Mobile:     acc.Mobile,
				TargetID:   target.ID,
				TargetName: target.Name,
				Mode:       string(target.Mode),
				ItemID:     target.ItemID,
				SKUID:      target.SKUID,
				ShopID:     target.ShopID,
				Quantity:   target.PerOrderQty,
				OrderID:    res.OrderID,
				TraceID:    res.TraceID,
			})
		}
	}

	progress("done", "success", "测试抢购完成", map[string]any{
		"success": res.Success,
		"orderId": res.OrderID,
		"traceId": res.TraceID,
	})
	return TestBuyResult{
		CanBuy:      true,
		NeedCaptcha: pre.NeedCaptcha,
		Success:     res.Success,
		OrderID:     res.OrderID,
		TraceID:     res.TraceID,
		Message: func() string {
			if res.Success {
				return "下单成功"
			}
			return "下单未成功"
		}(),
	}, nil
}

func (e *Engine) PreflightOnce(ctx context.Context, targetID string) (PreflightCheckResult, error) {
	if e.store == nil {
		return PreflightCheckResult{}, errors.New("store unavailable")
	}
	if e.provider == nil {
		return PreflightCheckResult{}, errors.New("provider unavailable")
	}
	target, err := e.store.GetTarget(ctx, targetID)
	if err != nil {
		return PreflightCheckResult{}, err
	}

	accounts, err := e.store.ListAccounts(ctx)
	if err != nil {
		return PreflightCheckResult{}, err
	}
	accounts = filterLoggedInAccounts(accounts)
	if len(accounts) == 0 {
		return PreflightCheckResult{}, errors.New("no logged-in accounts")
	}

	n := e.rr.Add(1)
	acc := accounts[int(n-1)%len(accounts)]
	if latest, err := e.store.GetAccount(ctx, acc.ID); err == nil {
		acc = latest
	}
	e.ensureAccountLimiter(acc.ID)

	e.mu.Lock()
	st := e.states[target.ID]
	if st == nil {
		st = &model.TaskState{TargetID: target.ID, Running: false, TargetQty: target.TargetQty}
		e.states[target.ID] = st
	}
	st.LastAttemptMs = time.Now().UnixMilli()
	e.publishStateLocked(*st)
	e.mu.Unlock()

	if !e.acquireAccount(ctx, acc.ID) {
		return PreflightCheckResult{}, ctx.Err()
	}
	defer e.releaseAccount(acc.ID)

	if !e.acquireInFlight(ctx) {
		return PreflightCheckResult{}, ctx.Err()
	}
	defer e.releaseInFlight()

	if !e.waitLimits(ctx, acc.ID) {
		return PreflightCheckResult{}, ctx.Err()
	}

	pre, updatedAcc, err := e.provider.Preflight(ctx, acc, target)
	if err != nil {
		e.setError(target.ID, err)
		return PreflightCheckResult{}, err
	}
	_ = e.persistAccount(ctx, updatedAcc)

	e.mu.Lock()
	if st := e.states[target.ID]; st != nil {
		v := pre.NeedCaptcha
		st.NeedCaptcha = &v
		e.publishStateLocked(*st)
	}
	e.mu.Unlock()

	msg := "预检完成"
	if !pre.CanBuy {
		msg = "当前不可购买"
	} else if pre.NeedCaptcha {
		msg = "需要验证码"
	} else {
		msg = "无需验证码"
	}
	return PreflightCheckResult{
		CanBuy:      pre.CanBuy,
		NeedCaptcha: pre.NeedCaptcha,
		TotalFee:    pre.TotalFee,
		TraceID:     pre.TraceID,
		Message:     msg,
	}, nil
}

func (e *Engine) persistAccount(ctx context.Context, acc model.Account) error {
	if acc.Mobile == "" {
		return nil
	}
	_, err := e.store.UpsertAccount(ctx, acc)
	return err
}

func (e *Engine) setError(targetID string, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.states[targetID]
	if st == nil {
		return
	}
	st.LastError = err.Error()
	e.publishStateLocked(*st)
	if e.bus != nil {
		e.bus.Log("warn", "任务执行失败", map[string]any{"targetId": targetID, "error": err.Error()})
	}
}

func (e *Engine) pickAccount() model.Account {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.accounts) == 0 {
		return model.Account{}
	}
	n := e.rr.Add(1)
	return e.accounts[int(n-1)%len(e.accounts)]
}

func filterLoggedInAccounts(accounts []model.Account) []model.Account {
	out := make([]model.Account, 0, len(accounts))
	for _, a := range accounts {
		if strings.TrimSpace(a.Token) == "" {
			continue
		}
		out = append(out, a)
	}
	return out
}

func (e *Engine) acquireInFlight(ctx context.Context) bool {
	select {
	case e.inFlight <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (e *Engine) releaseInFlight() {
	select {
	case <-e.inFlight:
	default:
	}
}

func (e *Engine) acquireAccount(ctx context.Context, accountID string) bool {
	e.mu.Lock()
	lock := e.accountLocks[accountID]
	e.mu.Unlock()
	if lock == nil {
		return true
	}
	select {
	case lock <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (e *Engine) releaseAccount(accountID string) {
	e.mu.Lock()
	lock := e.accountLocks[accountID]
	e.mu.Unlock()
	if lock == nil {
		return
	}
	select {
	case <-lock:
	default:
	}
}

func (e *Engine) publishStateLocked(st model.TaskState) {
	if e.bus != nil {
		e.bus.Publish("task_state", st)
	}
}

func (e *Engine) ensureAccountLimiter(accountID string) {
	perQPS := e.limits.PerAccountQPS
	if perQPS <= 0 {
		perQPS = 1
	}
	perBurst := e.limits.PerAccountBurst
	if perBurst <= 0 {
		perBurst = 2
	}
	e.mu.Lock()
	if e.perLimiter == nil {
		e.perLimiter = make(map[string]*rate.Limiter)
	}
	if e.accountLocks == nil {
		e.accountLocks = make(map[string]chan struct{})
	}
	if e.perLimiter[accountID] == nil {
		e.perLimiter[accountID] = rate.NewLimiter(rate.Limit(perQPS), perBurst)
	}
	if e.accountLocks[accountID] == nil {
		e.accountLocks[accountID] = make(chan struct{}, 1)
	}
	e.mu.Unlock()
}

func (e *Engine) waitLimits(ctx context.Context, accountID string) bool {
	if err := e.globalLimiter.Wait(ctx); err != nil {
		return false
	}
	e.mu.Lock()
	limiter := e.perLimiter[accountID]
	e.mu.Unlock()
	if limiter == nil {
		return true
	}
	if err := limiter.Wait(ctx); err != nil {
		return false
	}
	return true
}

func sleepUntil(ctx context.Context, t time.Time) bool {
	d := time.Until(t)
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
