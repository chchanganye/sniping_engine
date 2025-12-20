import { defineStore } from 'pinia'
import { h } from 'vue'
import { ElNotification } from 'element-plus'
import ProgressNotifyView from '@/components/ProgressNotifyView.vue'

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
let notifyType: 'success' | 'warning' | 'error' | 'info' | '' = ''

function clampSessions(list: ProgressSession[], max: number) {
  if (max <= 0) return
  if (list.length <= max) return
  list.length = max
}

function statusType(status: ProgressSessionStatus): 'success' | 'warning' | 'error' | 'info' {
  if (status === 'success') return 'success'
  if (status === 'warning') return 'warning'
  if (status === 'error') return 'error'
  return 'info'
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
  notifyType = ''
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
      const session = this.sessions.find((s) => s.opId === id)
      if (!session) return

      const type = statusType(session.status)
      const shouldReopen = !notifyHandle || notifyOpID !== id || notifyType !== type
      if (!shouldReopen) return

      closeNotify()

      notifyOpID = id
      notifyType = type
      notifyHandle = ElNotification({
        title: '',
        message: h(ProgressNotifyView, { opId: id }),
        type,
        customClass: 'progress-notify',
        position: 'bottom-right',
        duration: session.status === 'running' ? 0 : 3000,
        showClose: true,
        onClose: () => {
          if (notifyOpID === id) {
            notifyHandle = null
            notifyOpID = ''
            notifyType = ''
          }
        },
      }) as any
    },
  },
})
