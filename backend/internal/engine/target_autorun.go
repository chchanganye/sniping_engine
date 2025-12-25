package engine

import (
	"context"
	"errors"
	"time"

	"sniping_engine/internal/model"
)

func (e *Engine) IsRunning() bool {
	if e == nil {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// AutoRunByStore 根据数据库中“已启用任务”自动启动/停止引擎，并在引擎运行中动态同步任务列表。
// 目标：
// - 单个商品开关打开：无需点“开启全部”，也会生效并开始抢购/预热
// - 运行中启用/停用任务：无需重启引擎
func (e *Engine) AutoRunByStore(ctx context.Context) error {
	if e == nil || e.store == nil {
		return errors.New("store unavailable")
	}
	enabledTargets, err := e.store.ListEnabledTargets(ctx)
	if err != nil {
		return err
	}
	if len(enabledTargets) == 0 {
		if e.IsRunning() {
			_ = e.StopAll(ctx)
		}
		return nil
	}

	if !e.IsRunning() {
		return e.StartAll(ctx)
	}

	e.SyncEnabledTargets(enabledTargets)
	e.recalcCaptchaPoolActivateAtMs()
	return nil
}

func (e *Engine) SyncEnabledTargets(enabledTargets []model.Target) {
	if e == nil {
		return
	}

	type startItem struct {
		ctx    context.Context
		target model.Target
	}

	var cancels []context.CancelFunc
	var starts []startItem

	nowMs := time.Now().UnixMilli()

	e.mu.Lock()
	if !e.running || e.runCtx == nil {
		e.mu.Unlock()
		return
	}

	enabledMap := make(map[string]model.Target, len(enabledTargets))
	for _, t := range enabledTargets {
		if t.ID == "" {
			continue
		}
		enabledMap[t.ID] = t
	}

	// 更新快照给 captcha pool 的激活时间计算使用。
	e.targets = enabledTargets

	// 1) 停用/删除的目标：取消并移除
	for id, cancel := range e.targetCancels {
		next, ok := enabledMap[id]
		if !ok {
			cancels = append(cancels, cancel)
			delete(e.targetCancels, id)
			delete(e.targetSnapshots, id)
			if st := e.states[id]; st != nil {
				st.Running = false
				st.LastError = ""
				st.LastAttemptMs = nowMs
				e.publishStateLocked(*st)
			}
			continue
		}

		// 2) 配置变更：重启该目标 goroutine（避免“抢购时间/模式变了但不生效”）
		if prev, ok := e.targetSnapshots[id]; ok {
			if !prev.UpdatedAt.Equal(next.UpdatedAt) {
				cancels = append(cancels, cancel)
				delete(e.targetCancels, id)
				delete(e.targetSnapshots, id)
			}
		}
	}

	// 3) 新增/需要重启的目标：启动
	for id, t := range enabledMap {
		if _, ok := e.targetCancels[id]; ok {
			continue
		}
		targetCtx, targetCancel := context.WithCancel(e.runCtx)
		e.targetCancels[id] = targetCancel
		e.targetSnapshots[id] = t

		st := e.states[id]
		if st == nil {
			st = &model.TaskState{TargetID: id, Running: true, TargetQty: t.TargetQty}
			e.states[id] = st
		} else {
			st.Running = true
			st.TargetQty = t.TargetQty
		}
		st.LastAttemptMs = nowMs
		e.publishStateLocked(*st)

		starts = append(starts, startItem{ctx: targetCtx, target: t})
	}
	e.mu.Unlock()

	for _, c := range cancels {
		if c != nil {
			c()
		}
	}

	for _, s := range starts {
		e.wg.Add(1)
		go func(si startItem) {
			defer e.wg.Done()
			e.runTarget(si.ctx, si.target)
		}(s)
	}
}

