import { defineStore } from 'pinia'
import dayjs from 'dayjs'
import type { LogEntry, LogLevel } from '@/types/core'
import { uid } from '@/utils/id'

export const useLogsStore = defineStore('logs', {
  state: () => ({
    logs: [
      {
        id: uid('log'),
        at: dayjs().toISOString(),
        level: 'info' as LogLevel,
        message: '已加载 UI 骨架（尚未对接真实 API）',
      },
    ] as LogEntry[],
  }),
  actions: {
    addLog(payload: Omit<LogEntry, 'id' | 'at'> & { at?: string }) {
      this.logs.unshift({
        id: uid('log'),
        at: payload.at ?? dayjs().toISOString(),
        level: payload.level,
        accountId: payload.accountId,
        taskId: payload.taskId,
        message: payload.message,
      })
    },
    clear() {
      this.logs = []
    },
  },
})
