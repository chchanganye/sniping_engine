import { defineStore } from 'pinia'
import dayjs from 'dayjs'
import type { Task, TaskMode } from '@/types/core'
import { sleep } from '@/utils/sleep'
import { useLogsStore } from '@/stores/logs'
import { useAccountsStore } from '@/stores/accounts'
import { useGoodsStore } from '@/stores/goods'
import {
  apiListShippingAddresses,
  apiTradeCreateOrder,
  apiTradeRenderOrder,
  buildTradeCreateOrderPayloadFromRender,
  buildTradeRenderOrderPayload,
  FIXED_DEVICE_ID,
} from '@/services/api'

const taskControllers = new Map<string, AbortController>()

function resolveDivisionIds(address: any): string | undefined {
  const candidates = [address?.divisionIds, address?.divisionLevels, address?.divisionIdLevels]
  const text = candidates.find((v) => typeof v === 'string' && v.trim())
  if (typeof text === 'string' && text.trim()) return text.trim()

  const rawLevels = address?.divisionLevels
  if (Array.isArray(rawLevels)) {
    const nums = rawLevels.filter((v) => typeof v === 'number' && Number.isFinite(v))
    if (nums.length > 0) return nums.join(',')
  }

  const parts = [address?.provinceId, address?.cityId, address?.regionId].filter(
    (v): v is number => typeof v === 'number' && Number.isFinite(v),
  )
  return parts.length > 0 ? parts.join(',') : undefined
}

export const useTasksStore = defineStore('tasks', {
  state: () => ({
    tasks: [] as Task[],
  }),
  getters: {
    summary: (state) => {
      const total = state.tasks.length
      const running = state.tasks.filter((t) => t.status === 'running').length
      const success = state.tasks.filter((t) => t.status === 'success').length
      const failed = state.tasks.filter((t) => t.status === 'failed').length
      return { total, running, success, failed }
    },
  },
  actions: {
    syncFromTargetGoods() {
      const goodsStore = useGoodsStore()
      const nextIds = new Set(goodsStore.targetGoods.map((g) => g.id))

      for (const item of goodsStore.targetGoods) {
        const existing = this.tasks.find((t) => t.goodsId === item.id)
        if (existing) {
          existing.goodsTitle = item.title
          continue
        }
        const task: Task = {
          id: item.id,
          goodsId: item.id,
          goodsTitle: item.title,
          mode: 'rush',
          quantity: 1,
          scheduleAt: undefined,
          status: 'idle',
          createdAt: dayjs().toISOString(),
          successCount: 0,
          failCount: 0,
        }
        this.tasks.unshift(task)
      }

      const removed = this.tasks.filter((t) => !nextIds.has(t.goodsId))
      for (const task of removed) {
        const controller = taskControllers.get(task.id)
        if (controller) controller.abort()
        taskControllers.delete(task.id)
      }
      this.tasks = this.tasks.filter((t) => nextIds.has(t.goodsId))
    },
    updateTaskConfig(goodsId: string, patch: Partial<Pick<Task, 'mode' | 'quantity' | 'scheduleAt'>>) {
      const task = this.tasks.find((t) => t.goodsId === goodsId)
      if (!task) return
      if (patch.mode) task.mode = patch.mode
      if (typeof patch.quantity === 'number' && Number.isFinite(patch.quantity)) {
        task.quantity = Math.max(1, Math.floor(patch.quantity))
      }
      if (typeof patch.scheduleAt === 'string') task.scheduleAt = patch.scheduleAt
    },
    async runTask(taskId: string, controller: AbortController) {
      try {
        const task = this.tasks.find((t) => t.id === taskId)
        if (!task) return

        const logs = useLogsStore()
        const accountsStore = useAccountsStore()
        const goodsStore = useGoodsStore()

        const goods = goodsStore.targetGoods.find((g) => g.id === task.goodsId) ?? goodsStore.goods.find((g) => g.id === task.goodsId)
        const sku = goods?.raw as any

        const itemId = Number(sku?.itemId ?? goods?.id)
        const skuId = Number(sku?.skuId ?? sku?.itemId ?? goods?.id)
        const shopId = Number(sku?.storeId)
        const skuName = typeof sku?.name === 'string' ? sku.name : goods?.title ?? null

        if (!goods || !Number.isFinite(itemId) || !Number.isFinite(skuId)) {
          task.status = 'failed'
          task.lastMessage = '缺少商品信息，请先在「商品列表」加入目标清单'
          logs.addLog({ level: 'error', taskId, message: task.lastMessage })
          return
        }

        if (!Number.isFinite(shopId)) {
          task.status = 'failed'
          task.lastMessage = '缺少 shopId/storeId（下单所需），请重新加载商品列表后再加入目标清单'
          logs.addLog({ level: 'error', taskId, message: task.lastMessage })
          return
        }

        const candidates = accountsStore.accounts.filter((a) => Boolean(a.token))

        if (candidates.length === 0) {
          task.status = 'failed'
          task.lastMessage = '没有可用账号（需要先登录获取 token）'
          logs.addLog({ level: 'error', taskId, message: task.lastMessage })
          return
        }

        const accountContexts: Array<{
          accountId: string
          username: string
          token: string
          addressId: number
          devicesId: string
          divisionIds?: string
        }> = []

        for (const account of candidates) {
          if (controller.signal.aborted) break
          try {
            const list = await apiListShippingAddresses(account.token as string, { app: 'o2o', isAllCover: 1 })
            const addr = list.find((a) => a.isDefault) ?? list[0]
            if (!addr) throw new Error('该账号没有收货地址')
            accountContexts.push({
              accountId: account.id,
              username: account.username,
              token: account.token as string,
              addressId: addr.id,
              devicesId: account.deviceId ?? FIXED_DEVICE_ID,
              divisionIds: resolveDivisionIds(addr),
            })
          } catch (e) {
            const message = e instanceof Error ? e.message : '获取收货地址失败'
            task.failCount += 1
            logs.addLog({
              level: 'warning',
              taskId,
              accountId: account.id,
              message: `账号 ${account.username} 获取收货地址失败：${message}`,
            })
          }
        }

        if (controller.signal.aborted) {
          if (task.status === 'running') {
            task.status = 'stopped'
            task.lastMessage = '已停止'
          }
          return
        }

        if (accountContexts.length === 0) {
          task.status = 'failed'
          task.lastMessage = '所有账号均无法获取收货地址'
          logs.addLog({ level: 'error', taskId, message: task.lastMessage })
          return
        }

        const mode: TaskMode = task.mode
        const targetTotal = Math.max(1, Math.floor(task.quantity || 1))
        task.quantity = targetTotal

        if (task.scheduleAt) {
          const startAt = dayjs(task.scheduleAt)
          if (startAt.isValid() && startAt.isAfter(dayjs())) {
            task.status = 'scheduled'
            task.lastMessage = `等待开始：${startAt.format('YYYY-MM-DD HH:mm:ss')}`
            while (!controller.signal.aborted && dayjs().isBefore(startAt)) {
              await sleep(200)
            }
          }
        }

        if (controller.signal.aborted) {
          task.status = 'stopped'
          task.lastMessage = '已停止'
          logs.addLog({ level: 'warning', taskId, message: '任务已停止' })
          return
        }

        task.status = 'running'
        task.lastMessage = mode === 'scan' ? '扫货中…' : '抢购中…'
        logs.addLog({ level: 'info', taskId, message: `开始执行：${task.goodsTitle}（${mode === 'scan' ? '扫货' : '抢购'}）` })

        const pollDelayMs = mode === 'scan' ? 800 : 120

        let lastError = ''
        while (!controller.signal.aborted && task.successCount < targetTotal) {
          for (const ctx of accountContexts) {
            if (controller.signal.aborted) break

            try {
              const renderPayload = buildTradeRenderOrderPayload({
                sku: {
                  itemId,
                  skuId,
                  shopId,
                  skuName,
                },
                quantity: 1,
                deviceSource: 'WXAPP',
                orderSource: 'product.detail.page',
                devicesId: ctx.devicesId,
                divisionIds: ctx.divisionIds,
                addressId: ctx.addressId,
              })

              const renderData = await apiTradeRenderOrder(ctx.token, renderPayload, { signal: controller.signal })
              const canBuy = Boolean(renderData?.purchaseStatus?.canBuy)
              if (!canBuy) {
                task.lastMessage = mode === 'scan' ? '暂无库存，持续扫货中…' : '未满足购买条件，继续抢购…'
                continue
              }

              const createPayload = buildTradeCreateOrderPayloadFromRender(renderData, {
                deviceSource: 'WXAPP',
                buyConfig: renderPayload.buyConfig,
                orderSource: renderPayload.orderSource,
              })

              const data = await apiTradeCreateOrder(ctx.token, createPayload, { signal: controller.signal })
              task.successCount += 1
              task.lastMessage = `下单成功 ${task.successCount}/${targetTotal}（purchaseOrderId=${data.purchaseOrderId ?? '-'}）`
              logs.addLog({
                level: 'success',
                taskId,
                accountId: ctx.accountId,
                message: `抢购下单成功：${skuName ?? goods.title}（purchaseOrderId=${data.purchaseOrderId ?? '-'}）`,
              })

              if (task.successCount >= targetTotal) break
            } catch (e) {
              const message = e instanceof Error ? e.message : '创建订单失败'
              task.failCount += 1
              lastError = message
              if (task.failCount === 1 || task.failCount % 10 === 0) {
                logs.addLog({ level: 'warning', taskId, accountId: ctx.accountId, message: `下单失败：${message}` })
              }
            }
          }

          if (task.successCount >= targetTotal) break
          if (!controller.signal.aborted) await sleep(pollDelayMs)
        }

        if (controller.signal.aborted) {
          task.status = 'stopped'
          task.lastMessage = '已停止'
          logs.addLog({ level: 'warning', taskId, message: '任务已停止' })
          return
        }

        if (task.successCount >= targetTotal) {
          task.status = 'success'
          task.lastMessage = `已完成：${task.successCount}/${targetTotal}`
          logs.addLog({ level: 'success', taskId, message: `任务完成：${task.goodsTitle}` })
          return
        }

        task.status = 'failed'
        task.lastMessage = lastError ? `执行失败：${lastError}` : '执行失败'
        logs.addLog({ level: 'error', taskId, message: task.lastMessage })
      } finally {
        const current = taskControllers.get(taskId)
        if (current === controller) taskControllers.delete(taskId)
      }
    },
    startTask(goodsId: string) {
      this.syncFromTargetGoods()
      const target = this.tasks.find((t) => t.goodsId === goodsId)
      if (!target) return
      const logs = useLogsStore()
      if (target.status === 'running') return
      target.successCount = 0
      target.failCount = 0
      const startAt = target.scheduleAt ? dayjs(target.scheduleAt) : null
      if (startAt && startAt.isValid() && startAt.isAfter(dayjs())) {
        target.status = 'scheduled'
        target.lastMessage = `等待开始：${startAt.format('YYYY-MM-DD HH:mm:ss')}`
        logs.addLog({ level: 'info', taskId: target.id, message: `任务「${target.goodsTitle}」已排队，等待开始` })
      } else {
        target.status = 'running'
        target.lastMessage = '任务开始运行…'
        logs.addLog({ level: 'info', taskId: target.id, message: `任务「${target.goodsTitle}」开始运行` })
      }

      const existing = taskControllers.get(target.id)
      if (existing) existing.abort()
      const controller = new AbortController()
      taskControllers.set(target.id, controller)
      void this.runTask(target.id, controller)
    },
    startAll() {
      this.syncFromTargetGoods()
      for (const task of this.tasks) {
        if (task.status === 'running' || task.status === 'scheduled') continue
        this.startTask(task.goodsId)
      }
    },
    stopTask(goodsId: string) {
      const target = this.tasks.find((t) => t.goodsId === goodsId)
      if (!target) return
      const logs = useLogsStore()
      const controller = taskControllers.get(target.id)
      if (controller) controller.abort()
      taskControllers.delete(target.id)
      target.status = 'stopped'
      target.lastMessage = '已停止'
      logs.addLog({ level: 'warning', taskId: target.id, message: `任务「${target.goodsTitle}」已停止` })
    },
    stopAll() {
      for (const task of this.tasks) {
        this.stopTask(task.goodsId)
      }
    },
  },
})
