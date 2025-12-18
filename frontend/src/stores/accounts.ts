import { defineStore } from 'pinia'
import type { Account } from '@/types/core'
import { apiLoginBySmsCode } from '@/services/api'
import { beDeleteAccount, beListAccounts, beUpsertAccount, type BackendAccount } from '@/services/backend'

function normalizeAccount(raw: BackendAccount): Account {
  const token = typeof raw.token === 'string' && raw.token.trim() ? raw.token : undefined
  return {
    id: raw.id,
    mobile: raw.mobile,
    token,
    userAgent: typeof raw.userAgent === 'string' && raw.userAgent.trim() ? raw.userAgent : undefined,
    deviceId: typeof raw.deviceId === 'string' && raw.deviceId.trim() ? raw.deviceId : undefined,
    uuid: typeof raw.uuid === 'string' && raw.uuid.trim() ? raw.uuid : undefined,
    proxy: typeof raw.proxy === 'string' && raw.proxy.trim() ? raw.proxy : undefined,
    status: token ? 'logged_in' : 'idle',
    createdAt: raw.createdAt,
    updatedAt: raw.updatedAt,
  }
}

export const useAccountsStore = defineStore('accounts', {
  state: () => ({
    loading: false,
    accounts: [] as Account[],
    loaded: false,
  }),
  getters: {
    summary: (state) => {
      const total = state.accounts.length
      const loggedIn = state.accounts.filter((a) => Boolean(a.token)).length
      const running = 0
      const errors = state.accounts.filter((a) => a.status === 'error').length
      return { total, loggedIn, running, errors }
    },
  },
  actions: {
    async refresh() {
      this.loading = true
      try {
        const list = await beListAccounts()
        this.accounts = list.map(normalizeAccount)
        this.loaded = true
      } finally {
        this.loading = false
      }
    },
    async ensureLoaded() {
      if (this.loaded) return
      await this.refresh()
    },
    async upsert(payload: Partial<Account> & Pick<Account, 'mobile'>) {
      const created = await beUpsertAccount({
        id: payload.id,
        mobile: payload.mobile,
        token: payload.token,
        userAgent: payload.userAgent,
        deviceId: payload.deviceId,
        uuid: payload.uuid,
        proxy: payload.proxy,
      })
      const normalized = normalizeAccount(created)
      const idx = this.accounts.findIndex((a) => a.id === normalized.id)
      if (idx >= 0) this.accounts.splice(idx, 1, normalized)
      else this.accounts.unshift(normalized)
      return normalized
    },
    async loginBySms(payload: {
      mobile: string
      smsCode: string
      proxy?: string
      userAgent?: string
      deviceId?: string
      uuid?: string
    }) {
      const mobile = payload.mobile.trim()
      const smsCode = payload.smsCode.trim()
      if (!mobile) throw new Error('请输入手机号')
      if (!smsCode) throw new Error('请输入短信验证码')

      const existing = this.accounts.find((a) => a.mobile === mobile) ?? null
      const userAgent =
        payload.userAgent?.trim() ||
        existing?.userAgent ||
        (typeof navigator !== 'undefined' ? navigator.userAgent : '') ||
        undefined
      const deviceId = payload.deviceId?.trim() || existing?.deviceId || randomHex(16)
      const uuid = payload.uuid?.trim() || existing?.uuid || `${Date.now()}_${randomHex(10)}`

      await this.upsert({
        id: existing?.id,
        mobile,
        proxy: payload.proxy?.trim() || existing?.proxy,
        userAgent,
        deviceId,
        uuid,
      })

      await apiLoginBySmsCode({
        mobile,
        smsCode,
        deviceType: 'WXAPP',
        userAgent,
        deviceId,
        uuid,
      })

      await this.refresh()
    },
    async remove(id: string) {
      await beDeleteAccount(id)
      this.accounts = this.accounts.filter((a) => a.id !== id)
    },
  },
})

function randomHex(bytes: number): string {
  const buf = new Uint8Array(bytes)
  if (typeof crypto !== 'undefined' && 'getRandomValues' in crypto) {
    crypto.getRandomValues(buf)
  } else {
    for (let i = 0; i < bytes; i += 1) buf[i] = Math.floor(Math.random() * 256)
  }
  return Array.from(buf, (b) => b.toString(16).padStart(2, '0')).join('')
}
