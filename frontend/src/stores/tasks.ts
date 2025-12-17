import { defineStore } from 'pinia'
import type { GoodsItem, Task, TaskMode, TaskStatus } from '@/types/core'
import {
  beDeleteTarget,
  beEngineStart,
  beEngineState,
  beEngineStop,
  beListTargets,
  beUpsertTarget,
  type BackendTarget,
  type EngineState,
  type EngineTaskState,
} from '@/services/backend'
import { useGoodsStore } from '@/stores/goods'

function normalizeMode(value: unknown): TaskMode {
  return value === 'scan' ? 'scan' : 'rush'
}

function normalizeTaskStatus(target: BackendTarget, state: EngineTaskState | undefined, engineRunning: boolean): TaskStatus {
  if (!target.enabled) return 'stopped'
  const now = Date.now()

  if (target.mode === 'rush' && typeof target.rushAtMs === 'number' && target.rushAtMs > now && engineRunning) {
    return 'scheduled'
  }

  if (state) {
    if (state.purchasedQty >= state.targetQty && state.targetQty > 0) return 'success'
    if (engineRunning && state.running) return 'running'
    if (state.lastError && state.purchasedQty < state.targetQty) return 'failed'
  }

  return engineRunning ? 'running' : 'idle'
}

function mapTargetToTask(target: BackendTarget, engine: EngineState | null): Task {
  const state = engine?.tasks?.find((t) => t.targetId === target.id)
  const purchasedQty = typeof state?.purchasedQty === 'number' ? state.purchasedQty : 0

  return {
    id: target.id,
    goodsTitle: target.name || `${target.itemId}`,
    itemId: target.itemId,
    skuId: target.skuId,
    shopId: target.shopId,
    mode: normalizeMode(target.mode),
    targetQty: target.targetQty,
    perOrderQty: target.perOrderQty,
    rushAtMs: typeof target.rushAtMs === 'number' && target.rushAtMs > 0 ? target.rushAtMs : undefined,
    enabled: Boolean(target.enabled),
    status: normalizeTaskStatus(target, state, Boolean(engine?.running)),
    purchasedQty,
    lastError: typeof state?.lastError === 'string' ? state.lastError : undefined,
    lastAttemptMs: state?.lastAttemptMs,
    lastSuccessMs: state?.lastSuccessMs,
    createdAt: target.createdAt,
    updatedAt: target.updatedAt,
  }
}

function extractTargetFromGoods(goods: GoodsItem): Pick<BackendTarget, 'itemId' | 'skuId' | 'shopId' | 'name'> | null {
  const sku = goods.raw as any
  const itemId = Number(sku?.itemId ?? goods.id)
  const skuId = Number(sku?.skuId ?? sku?.itemId ?? goods.id)
  const shopId = Number(sku?.storeId ?? sku?.shopId ?? sku?.sellerId ?? 0)
  if (!Number.isFinite(itemId) || !Number.isFinite(skuId)) return null
  return {
    name: goods.title,
    itemId,
    skuId,
    shopId: Number.isFinite(shopId) ? shopId : undefined,
  }
}

export const useTasksStore = defineStore('tasks', {
  state: () => ({
    loading: false,
    engineLoading: false,
    engine: null as EngineState | null,
    tasks: [] as Task[],
    loaded: false,
  }),
  getters: {
    summary: (state) => {
      const total = state.tasks.length
      const running = state.tasks.filter((t) => t.status === 'running' || t.status === 'scheduled').length
      const success = state.tasks.filter((t) => t.status === 'success').length
      const failed = state.tasks.filter((t) => t.status === 'failed').length
      return { total, running, success, failed }
    },
    engineRunning: (state) => Boolean(state.engine?.running),
  },
  actions: {
    async refresh() {
      this.loading = true
      try {
        const [targets, engine] = await Promise.all([beListTargets(), beEngineState().catch(() => null)])
        this.engine = engine
        this.tasks = targets.map((t) => mapTargetToTask(t, engine))
        this.loaded = true
      } finally {
        this.loading = false
      }
    },
    async ensureLoaded() {
      if (this.loaded) return
      await this.refresh()
    },
    async importFromGoodsSelection() {
      const goodsStore = useGoodsStore()
      await this.ensureLoaded()

      for (const goods of goodsStore.targetGoods) {
        const extracted = extractTargetFromGoods(goods)
        if (!extracted) continue

        const existing = this.tasks.find((t) => t.itemId === extracted.itemId && t.skuId === extracted.skuId)
        await beUpsertTarget({
          id: existing?.id,
          name: extracted.name,
          itemId: extracted.itemId,
          skuId: extracted.skuId,
          shopId: extracted.shopId,
          mode: existing?.mode ?? 'rush',
          targetQty: existing?.targetQty ?? 1,
          perOrderQty: existing?.perOrderQty ?? 1,
          rushAtMs: existing?.rushAtMs ?? 0,
          enabled: existing?.enabled ?? true,
        })
      }

      await this.refresh()
    },
    async updateTask(id: string, patch: Partial<Pick<Task, 'mode' | 'targetQty' | 'perOrderQty' | 'rushAtMs' | 'enabled' | 'goodsTitle'>>) {
      const current = this.tasks.find((t) => t.id === id)
      if (!current) return

      const next: Task = { ...current, ...patch }
      if (next.targetQty <= 0) next.targetQty = 1
      if (next.perOrderQty <= 0) next.perOrderQty = 1

      const saved = await beUpsertTarget({
        id: next.id,
        name: next.goodsTitle,
        itemId: next.itemId,
        skuId: next.skuId,
        shopId: next.shopId,
        mode: next.mode,
        targetQty: next.targetQty,
        perOrderQty: next.perOrderQty,
        rushAtMs: next.mode === 'rush' ? next.rushAtMs ?? 0 : 0,
        enabled: next.enabled,
      })

      const engine = this.engine
      const mapped = mapTargetToTask(saved, engine)
      const idx = this.tasks.findIndex((t) => t.id === id)
      if (idx >= 0) this.tasks.splice(idx, 1, mapped)
    },
    async removeTask(id: string) {
      await beDeleteTarget(id)
      this.tasks = this.tasks.filter((t) => t.id !== id)
    },
    async startEngine() {
      this.engineLoading = true
      try {
        await beEngineStart()
        this.engine = await beEngineState()
        this.tasks = this.tasks.map((t) => mapTargetToTask({
          id: t.id,
          name: t.goodsTitle,
          itemId: t.itemId,
          skuId: t.skuId,
          shopId: t.shopId,
          mode: t.mode,
          targetQty: t.targetQty,
          perOrderQty: t.perOrderQty,
          rushAtMs: t.rushAtMs,
          enabled: t.enabled,
        } as BackendTarget, this.engine))
      } finally {
        this.engineLoading = false
      }
    },
    async stopEngine() {
      this.engineLoading = true
      try {
        await beEngineStop()
        this.engine = await beEngineState().catch(() => ({ running: false, tasks: [] }))
        await this.refresh()
      } finally {
        this.engineLoading = false
      }
    },
    applyTaskState(state: EngineTaskState) {
      const idx = this.tasks.findIndex((t) => t.id === state.targetId)
      if (idx < 0) return
      const current = this.tasks[idx]!
      const next: Task = {
        ...current,
        purchasedQty: typeof state.purchasedQty === 'number' ? state.purchasedQty : current.purchasedQty,
        lastError: typeof state.lastError === 'string' ? state.lastError : undefined,
        lastAttemptMs: state.lastAttemptMs,
        lastSuccessMs: state.lastSuccessMs,
      }
      next.status = normalizeTaskStatus(
        {
          id: next.id,
          name: next.goodsTitle,
          itemId: next.itemId,
          skuId: next.skuId,
          shopId: next.shopId,
          mode: next.mode,
          targetQty: next.targetQty,
          perOrderQty: next.perOrderQty,
          rushAtMs: next.rushAtMs,
          enabled: next.enabled,
        },
        state,
        Boolean(this.engine?.running),
      )
      this.tasks.splice(idx, 1, next)
    },
  },
})
