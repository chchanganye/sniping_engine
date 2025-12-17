import { defineStore } from 'pinia'
import dayjs from 'dayjs'
import type { Task, TaskStatus } from '@/types/core'
import { uid } from '@/utils/id'
import { sleep } from '@/utils/sleep'
import { useLogsStore } from '@/stores/logs'
import { useAccountsStore } from '@/stores/accounts'
import { useGoodsStore } from '@/stores/goods'
import { apiListShippingAddresses, apiTradeCreateOrder, buildTradeCreateOrderPayload, FIXED_DEVICE_ID } from '@/services/api'

const taskControllers = new Map<string, AbortController>()

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
    async runCreateOrderRush(taskId: string, controller: AbortController) {
      try {
        const task = this.tasks.find((t) => t.id === taskId)
        if (!task) return

        const logs = useLogsStore()
        const accountsStore = useAccountsStore()
        const goodsStore = useGoodsStore()

        const goods = goodsStore.goods.find((g) => g.id === task.goodsId) ?? goodsStore.targetGoods.find((g) => g.id === task.goodsId)
        const sku = goods?.raw as any

        const itemId = Number(sku?.itemId ?? goods?.id)
        const skuId = Number(sku?.skuId ?? sku?.itemId ?? goods?.id)
        const shopId = Number(sku?.storeId)
        const sellerId = typeof sku?.sellerId === 'number' ? sku.sellerId : undefined
        const skuCode = typeof sku?.skuCode === 'string' ? sku.skuCode : null
        const categoryId = typeof sku?.categoryId === 'number' ? sku.categoryId : null
        const skuName = typeof sku?.name === 'string' ? sku.name : goods?.title ?? null
        const mainImage = typeof sku?.mainImage === 'string' ? sku.mainImage : (goods?.imageUrl ?? null)
        const salePrice = typeof sku?.price === 'number' ? sku.price : null
        const fullUnit = typeof sku?.fullUnit === 'string' ? sku.fullUnit : null

        if (!goods || !Number.isFinite(itemId) || !Number.isFinite(skuId)) {
          task.status = 'failed'
          task.lastMessage = '缺少商品信息，请先在「商品列表」加载并选择目标商品'
          logs.addLog({ level: 'error', taskId, message: task.lastMessage })
          return
        }

        if (!Number.isFinite(shopId)) {
          task.status = 'failed'
          task.lastMessage = '缺少 shopId/storeId（下单所需），请重新加载商品列表'
          logs.addLog({ level: 'error', taskId, message: task.lastMessage })
          return
        }

        const candidates = task.accountIds
          .map((id) => accountsStore.accounts.find((a) => a.id === id) ?? null)
          .filter((a): a is NonNullable<typeof a> => Boolean(a))
          .filter((a) => Boolean(a.token))

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
          deviceId?: string
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
              deviceId: account.deviceId,
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

        const maxAttempts = 60
        const intervalMs = 120

        let lastError = ''

        for (let attempt = 1; attempt <= maxAttempts; attempt += 1) {
          if (controller.signal.aborted) break

          task.lastMessage = `第 ${attempt}/${maxAttempts} 轮抢购中…`

          for (const ctx of accountContexts) {
            if (controller.signal.aborted) break
            try {
              const payload = buildTradeCreateOrderPayload({
                addressId: ctx.addressId,
                quantity: task.quantity,
                deviceSource: 'WXAPP',
                orderSource: 'product.detail.page',
                devicesId: ctx.deviceId ?? FIXED_DEVICE_ID,
                settleAccountId: typeof sellerId === 'number' ? String(sellerId) : '',
                sku: {
                  itemId,
                  skuId,
                  shopId,
                  skuCode,
                  categoryId,
                  skuName,
                  mainImage,
                  salePrice,
                  fullUnit,
                  itemAttributes: null,
                },
              })

              const data = await apiTradeCreateOrder(ctx.token, payload, { signal: controller.signal })
              task.successCount += 1
              task.status = 'success'
              task.lastMessage = `下单成功（purchaseOrderId=${data.purchaseOrderId ?? '-'}）`
              logs.addLog({
                level: 'success',
                taskId,
                accountId: ctx.accountId,
                message: `抢购下单成功：${skuName ?? goods.title}（purchaseOrderId=${data.purchaseOrderId ?? '-'}）`,
              })

              controller.abort()
              break
            } catch (e) {
              const message = e instanceof Error ? e.message : '创建订单失败'
              task.failCount += 1
              lastError = message
              if (attempt === 1 || attempt % 10 === 0) {
                logs.addLog({
                  level: 'warning',
                  taskId,
                  accountId: ctx.accountId,
                  message: `第 ${attempt} 轮下单失败：${message}`,
                })
              }
            }
          }

          if (task.status === 'success') return
          if (controller.signal.aborted) break
          await sleep(intervalMs)
        }

        if (task.status === 'success') return

        if (controller.signal.aborted) {
          if (task.status === 'running') {
            task.status = 'stopped'
            task.lastMessage = '已停止'
            logs.addLog({ level: 'warning', taskId, message: '任务已停止' })
          }
          return
        }

        task.status = 'failed'
        task.lastMessage = lastError ? `抢购失败：${lastError}` : '抢购失败'
        logs.addLog({ level: 'error', taskId, message: task.lastMessage })
      } finally {
        const current = taskControllers.get(taskId)
        if (current === controller) taskControllers.delete(taskId)
      }
    },
    createTask(payload: {
      goodsId: string
      goodsTitle: string
      accountIds: string[]
      quantity: number
      scheduleAt?: string
    }) {
      const task: Task = {
        id: uid('task'),
        goodsId: payload.goodsId,
        goodsTitle: payload.goodsTitle,
        accountIds: payload.accountIds,
        quantity: payload.quantity,
        scheduleAt: payload.scheduleAt,
        status: payload.scheduleAt ? ('scheduled' as TaskStatus) : ('idle' as TaskStatus),
        createdAt: dayjs().toISOString(),
        successCount: 0,
        failCount: 0,
      }
      this.tasks.unshift(task)
      const logs = useLogsStore()
      logs.addLog({ level: 'info', taskId: task.id, message: `已创建任务「${task.goodsTitle}」` })
      return task
    },
    startTask(id: string) {
      const target = this.tasks.find((t) => t.id === id)
      if (!target) return
      const logs = useLogsStore()
      if (target.status === 'running') return
      target.successCount = 0
      target.failCount = 0
      target.status = 'running'
      target.lastMessage = '任务开始运行…'
      logs.addLog({ level: 'info', taskId: id, message: `任务「${target.goodsTitle}」开始运行` })

      const existing = taskControllers.get(id)
      if (existing) existing.abort()
      const controller = new AbortController()
      taskControllers.set(id, controller)
      void this.runCreateOrderRush(id, controller)
    },
    stopTask(id: string) {
      const target = this.tasks.find((t) => t.id === id)
      if (!target) return
      const logs = useLogsStore()
      const controller = taskControllers.get(id)
      if (controller) controller.abort()
      taskControllers.delete(id)
      target.status = 'stopped'
      target.lastMessage = '已停止'
      logs.addLog({ level: 'warning', taskId: id, message: `任务「${target.goodsTitle}」已停止` })
    },
    removeTask(id: string) {
      const controller = taskControllers.get(id)
      if (controller) controller.abort()
      taskControllers.delete(id)
      this.tasks = this.tasks.filter((t) => t.id !== id)
    },
  },
})
