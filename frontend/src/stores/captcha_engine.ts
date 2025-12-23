import { defineStore } from 'pinia'
import { beCaptchaEngineState, type CaptchaEngineStatus } from '@/services/backend'

type CaptchaState = 'stopped' | 'starting' | 'ready' | 'error' | 'unknown'

export const useCaptchaEngineStore = defineStore('captchaEngine', {
  state: () => ({
    status: null as CaptchaEngineStatus | null,
    state: 'unknown' as CaptchaState,
    loading: false,
    lastError: '' as string,
  }),
  actions: {
    async refresh() {
      if (this.loading) return
      this.loading = true
      this.lastError = ''
      try {
        const s = await beCaptchaEngineState()
        this.status = s
        this.state = (s?.state as CaptchaState) || 'unknown'
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e)
        this.lastError = msg
        this.state = 'unknown'
      } finally {
        this.loading = false
      }
    },
    startPolling() {
      if (typeof window === 'undefined') return
      if ((this as any)._timer) return

      const tick = async () => {
        await this.refresh()
        const next = this.state === 'ready' ? 8000 : 1500
        ;(this as any)._timer = window.setTimeout(tick, next)
      }

      void tick()
    },
    stopPolling() {
      const t = (this as any)._timer as number | undefined
      if (t) window.clearTimeout(t)
      ;(this as any)._timer = undefined
    },
  },
})

