package engine

import (
	"context"
	"errors"
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
		e.bus.Log("info", "engine start", map[string]any{"provider": e.provider.Name()})
	}

	accounts, err := e.store.ListAccounts(ctx)
	if err != nil {
		_ = e.StopAll(ctx)
		return err
	}
	if len(accounts) == 0 {
		_ = e.StopAll(ctx)
		return errors.New("no accounts in storage")
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
			e.bus.Log("info", "engine stopped", nil)
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
			e.bus.Log("info", "target waiting for rush time", map[string]any{
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

	if err := e.globalLimiter.Wait(ctx); err != nil {
		return
	}
	if limiter := e.perLimiter[acc.ID]; limiter != nil {
		if err := limiter.Wait(ctx); err != nil {
			return
		}
	}

	pre, updatedAcc, err := e.provider.Preflight(ctx, acc, target)
	if err != nil {
		e.setError(target.ID, err)
		return
	}
	_ = e.persistAccount(ctx, updatedAcc)

	if !pre.CanBuy {
		if e.bus != nil {
			e.bus.Log("debug", "preflight: canBuy=false", map[string]any{
				"targetId":  target.ID,
				"accountId": acc.ID,
				"traceId":   pre.TraceID,
			})
		}
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
			e.bus.Log("info", "order created", map[string]any{
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
		e.bus.Log("warn", "task error", map[string]any{"targetId": targetID, "error": err.Error()})
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
