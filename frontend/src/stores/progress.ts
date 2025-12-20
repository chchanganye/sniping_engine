import { defineStore } from 'pinia'
import { h } from 'vue'
import dayjs from 'dayjs'
import { ElNotification } from 'element-plus'

export type ProgressPhase = 'start' | 'info' | 'success' | 'warning' | 'error'

export interface ProgressEventPayload {
  opId: string
  kind: string
  step: string
  phase: ProgressPhase | string
  message?: string
  targetId?: string
  accountId?: string
  fields?: Record<string, any>
}

export interface ProgressEvent extends ProgressEventPayload {
  time: number
}

export type ProgressSessionStatus = 'running' | 'success' | 'warning' | 'error'

export interface ProgressSession {
  opId: string
  kind: string
  title: string
  targetId?: string
  accountId?: string
  status: ProgressSessionStatus
  createdAt: number
  endedAt?: number
  events: ProgressEvent[]
}

type NotifyHandle = { close: () => void }

let notifyHandle: NotifyHandle | null = null
let notifyOpID = ''
let notifyTimer: number | null = null

function clampSessions(list: ProgressSession[], max: number) {
  if (max <= 0) return
  if (list.length <= max) return
  list.length = max
}

function phaseType(phase: string): 'success' | 'warning' | 'error' | 'info' {
  const v = (phase || '').toLowerCase()
  if (v === 'success') return 'success'
  if (v === 'warning') return 'warning'
  if (v === 'error') return 'error'
  return 'info'
}

function statusType(status: ProgressSessionStatus): 'success' | 'warning' | 'error' | 'info' {
  if (status === 'success') return 'success'
  if (status === 'warning') return 'warning'
  if (status === 'error') return 'error'
  return 'info'
}

function formatTime(ms: number) {
  if (!Number.isFinite(ms)) return '--:--:--'
  return dayjs(ms).format('HH:mm:ss')
}

function eventSummary(ev: ProgressEvent) {
  const msg = (ev.message || '').trim()
  const api = typeof ev.fields?.api === 'string' ? String(ev.fields.api).trim() : ''
  if (msg && api && !msg.includes(api)) return `${msg}（${api}）`
  return msg || ev.step || '(empty)'
}

function closeNotify() {
  if (notifyHandle) {
    try {
      notifyHandle.close()
    } catch {
      // ignore
    }
  }
  notifyHandle = null
  notifyOpID = ''
}

export const useProgressStore = defineStore('progress', {
  state: () => ({
    activeOpId: '' as string,
    sessions: [] as ProgressSession[],
  }),
  getters: {
    runningCount: (state) => state.sessions.filter((s) => s.status === 'running').length,
    activeSession: (state) => state.sessions.find((s) => s.opId === state.activeOpId) ?? state.sessions[0] ?? null,
  },
  actions: {
    begin(payload: { opId: string; kind: string; title: string; targetId?: string }) {
      const opId = payload.opId.trim()
      if (!opId) return

      const existing = this.sessions.find((s) => s.opId === opId)
      if (existing) {
        this.activeOpId = opId
        this.notify(opId)
        return
      }

      this.sessions.unshift({
        opId,
        kind: payload.kind,
        title: payload.title,
        targetId: payload.targetId,
        status: 'running',
        createdAt: Date.now(),
        events: [],
      })
      clampSessions(this.sessions, 30)

      this.activeOpId = opId
      this.notify(opId)
    },
    addEvent(time: number, data: ProgressEventPayload) {
      const opId = String(data?.opId ?? '').trim()
      if (!opId) return

      let session = this.sessions.find((s) => s.opId === opId)
      if (!session) {
        session = {
          opId,
          kind: String(data?.kind ?? ''),
          title: String(data?.kind ?? '执行进度'),
          targetId: typeof data?.targetId === 'string' ? data.targetId : undefined,
          status: 'running',
          createdAt: Date.now(),
          events: [],
        }
        this.sessions.unshift(session)
        clampSessions(this.sessions, 30)
      }

      const event: ProgressEvent = {
        time: typeof time === 'number' && Number.isFinite(time) ? time : Date.now(),
        opId,
        kind: String(data?.kind ?? ''),
        step: String(data?.step ?? ''),
        phase: String(data?.phase ?? 'info'),
        message: typeof data?.message === 'string' ? data.message : undefined,
        targetId: typeof data?.targetId === 'string' ? data.targetId : undefined,
        accountId: typeof data?.accountId === 'string' ? data.accountId : undefined,
        fields: typeof data?.fields === 'object' && data?.fields ? (data.fields as Record<string, any>) : undefined,
      }
      session.events.push(event)
      if (session.events.length > 300) session.events.splice(0, session.events.length - 300)

      if (event.accountId && !session.accountId) session.accountId = event.accountId
      if (event.targetId && !session.targetId) session.targetId = event.targetId

      const phase = (event.phase || '').toLowerCase()
      if (phase === 'error') {
        session.status = 'error'
        session.endedAt = event.time
      } else if (phase === 'warning' && session.status === 'running') {
        session.status = 'warning'
      } else if (event.step === 'done' && phase === 'success') {
        session.status = 'success'
        session.endedAt = event.time
      } else if (session.status === 'success' || session.status === 'error') {
        // keep terminal status
      } else {
        session.status = 'running'
      }

      this.notify(opId)
    },
    clear(opId: string) {
      this.sessions = this.sessions.filter((s) => s.opId !== opId)
      if (this.activeOpId === opId) this.activeOpId = this.sessions[0]?.opId ?? ''
    },
    clearAll() {
      this.sessions = []
      this.activeOpId = ''
      closeNotify()
    },

    notify(opId: string) {
      if (typeof window === 'undefined') return
      const id = (opId || '').trim()
      if (!id) return

      if (notifyTimer != null) {
        window.clearTimeout(notifyTimer)
      }
      notifyTimer = window.setTimeout(() => {
        notifyTimer = null

        const session = this.sessions.find((s) => s.opId === id)
        if (!session) return

        const lines = session.events.slice(-8)
        const lastLine = lines.length > 0 ? lines[lines.length - 1] : undefined
        const topStatus = session.status === 'running' ? '进行中' : session.status === 'warning' ? '需处理' : session.status === 'error' ? '失败' : '完成'

        const message = h('div', { style: 'min-width: 320px' }, [
          h('div', { style: 'display:flex;align-items:center;justify-content:space-between;gap:10px;margin-bottom:6px' }, [
            h('div', { style: 'font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap' }, session.title),
            h(
              'span',
              {
                style:
                  'font-size:12px;color:#606266;border:1px solid #ebeef5;border-radius:999px;padding:2px 8px;background:#fff;flex:none',
              },
              topStatus,
            ),
          ]),
          h('div', { style: 'font-size:12px;color:#909399;margin-bottom:8px' }, `最近更新：${formatTime(lastLine?.time ?? Date.now())}`),
          h(
            'div',
            { style: 'max-height: 220px; overflow:auto; padding-right: 6px' },
            lines.map((ev) =>
              h(
                'div',
                { style: 'display:flex;gap:8px;align-items:flex-start;margin:4px 0' },
                [
                  h('span', { style: 'color:#909399;flex:none' }, formatTime(ev.time)),
                  h('span', { style: `flex:none;color:${phaseType(String(ev.phase)) === 'error' ? '#f56c6c' : phaseType(String(ev.phase)) === 'warning' ? '#e6a23c' : phaseType(String(ev.phase)) === 'success' ? '#67c23a' : '#409eff'}` }, '•'),
                  h('span', { style: 'color:#303133;word-break:break-word' }, eventSummary(ev)),
                ],
              ),
            ),
          ),
        ])

        if (notifyHandle && notifyOpID === id) {
          closeNotify()
        } else if (notifyHandle && notifyOpID !== id) {
          closeNotify()
        }

        notifyOpID = id
        const type = statusType(session.status)
        notifyHandle = ElNotification({
          title: '执行进度',
          message,
          type,
          position: 'bottom-right',
          duration: session.status === 'running' ? 0 : 4500,
          showClose: true,
          onClose: () => {
            if (notifyOpID === id) {
              notifyHandle = null
              notifyOpID = ''
            }
          },
        }) as any
      }, 120)
    },
  },
})
