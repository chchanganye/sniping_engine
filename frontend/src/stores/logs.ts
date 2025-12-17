import { defineStore } from 'pinia'
import type { LogEntry, LogLevel } from '@/types/core'
import { uid } from '@/utils/id'
import { useTasksStore } from '@/stores/tasks'

type BusMessage =
  | { type: 'log'; time: number; data: { level: string; msg: string; fields?: Record<string, any> } }
  | { type: 'task_state'; time: number; data: any }
  | { type: string; time: number; data: any }

function mapLevel(level: string): LogLevel {
  const v = (level || '').toLowerCase()
  if (v === 'error') return 'error'
  if (v === 'warn' || v === 'warning') return 'warning'
  if (v === 'success') return 'success'
  return 'info'
}

function buildWsURL(path: string): string {
  const loc = window.location
  const proto = loc.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${loc.host}${path}`
}

function compactFields(fields?: Record<string, any>): string {
  if (!fields) return ''
  const picked: Record<string, any> = {}
  for (const k of ['accountId', 'targetId', 'orderId', 'traceId', 'url', 'method', 'error']) {
    if (fields[k] != null) picked[k] = fields[k]
  }
  const keys = Object.keys(picked)
  if (keys.length === 0) return ''
  try {
    return JSON.stringify(picked)
  } catch {
    return ''
  }
}

export const useLogsStore = defineStore('logs', {
  state: () => ({
    logs: [] as LogEntry[],
    connected: false,
    connecting: false,
    lastError: '' as string,
  }),
  actions: {
    addLog(payload: Omit<LogEntry, 'id' | 'at'> & { at?: string }) {
      this.logs.unshift({
        id: uid('log'),
        at: payload.at ?? new Date().toISOString(),
        level: payload.level,
        accountId: payload.accountId,
        taskId: payload.taskId,
        message: payload.message,
      })
      if (this.logs.length > 2000) this.logs.length = 2000
    },
    clear() {
      this.logs = []
    },
    connect() {
      if (typeof window === 'undefined') return
      if (this.connected || this.connecting) return

      const tasksStore = useTasksStore()
      this.connecting = true
      this.lastError = ''

      const url = buildWsURL('/ws')
      const ws = new WebSocket(url)
      let closedByUser = false

      const cleanup = () => {
        this.connected = false
        this.connecting = false
      }

      ws.onopen = () => {
        this.connected = true
        this.connecting = false
      }

      ws.onmessage = (evt) => {
        try {
          const msg = JSON.parse(String(evt.data)) as BusMessage
          if (!msg || typeof msg !== 'object') return

          if (msg.type === 'task_state') {
            tasksStore.applyTaskState(msg.data)
            return
          }

          if (msg.type === 'log') {
            const at = typeof msg.time === 'number' ? new Date(msg.time).toISOString() : new Date().toISOString()
            const fields = msg.data?.fields as Record<string, any> | undefined
            const suffix = compactFields(fields)
            const text = suffix ? `${msg.data?.msg ?? ''} ${suffix}`.trim() : String(msg.data?.msg ?? '')

            this.addLog({
              at,
              level: mapLevel(String(msg.data?.level ?? 'info')),
              accountId: typeof fields?.accountId === 'string' ? fields.accountId : undefined,
              taskId: typeof fields?.targetId === 'string' ? fields.targetId : undefined,
              message: text || '(empty log)',
            })
          }
        } catch {
          // ignore
        }
      }

      ws.onerror = () => {
        this.lastError = 'WebSocket error'
      }

      ws.onclose = () => {
        cleanup()
        if (closedByUser) return
        window.setTimeout(() => this.connect(), 1000)
      }

      // store disconnect handle on instance
      ;(this as any)._ws = ws
      ;(this as any)._wsClose = () => {
        closedByUser = true
        try {
          ws.close()
        } catch {
          // ignore
        }
        cleanup()
      }
    },
    disconnect() {
      const close = (this as any)._wsClose as (() => void) | undefined
      if (close) close()
      ;(this as any)._ws = undefined
      ;(this as any)._wsClose = undefined
    },
  },
})

