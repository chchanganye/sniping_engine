import { defineStore } from 'pinia'
import dayjs from 'dayjs'
import type { Task, TaskStatus } from '@/types/core'
import { uid } from '@/utils/id'
import { useLogsStore } from '@/stores/logs'

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
      target.status = 'running'
      target.lastMessage = '任务开始运行（mock）'
      logs.addLog({ level: 'info', taskId: id, message: `任务「${target.goodsTitle}」开始运行（mock）` })
    },
    stopTask(id: string) {
      const target = this.tasks.find((t) => t.id === id)
      if (!target) return
      const logs = useLogsStore()
      target.status = 'stopped'
      target.lastMessage = '已停止'
      logs.addLog({ level: 'warning', taskId: id, message: `任务「${target.goodsTitle}」已停止` })
    },
    removeTask(id: string) {
      this.tasks = this.tasks.filter((t) => t.id !== id)
    },
  },
})
