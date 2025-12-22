import { defineStore } from 'pinia'
import type { Account } from '@/types/core'
import { apiLoginByPassword, apiLoginBySmsCode } from '@/services/api'
import { beDeleteAccount, beListAccounts, beUpsertAccount, type BackendAccount } from '@/services/backend'

const DEFAULT_WXAPP_UA =
  'Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 MicroMessenger/8.0.66(0x18004235) NetType/WIFI Language/zh_CN'

function normalizeAccount(raw: BackendAccount): Account {
  const token = typeof raw.token === 'string' && raw.token.trim() ? raw.token : undefined
  return {
    id: raw.id,
    username: typeof raw.username === 'string' && raw.username.trim() ? raw.username.trim() : undefined,
    mobile: raw.mobile,
    token,
    userAgent: typeof raw.userAgent === 'string' && raw.userAgent.trim() ? raw.userAgent : undefined,
    deviceId: typeof raw.deviceId === 'string' && raw.deviceId.trim() ? raw.deviceId : undefined,
    uuid: typeof raw.uuid === 'string' && raw.uuid.trim() ? raw.uuid : undefined,
    proxy: typeof raw.proxy === 'string' && raw.proxy.trim() ? raw.proxy : undefined,
    cookies: raw.cookies,
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
      const existing =
        (payload.id ? this.accounts.find((a) => a.id === payload.id) : null) ??
        this.accounts.find((a) => a.mobile === payload.mobile) ??
        null

      const created = await beUpsertAccount({
        id: payload.id,
        username: payload.username ?? existing?.username,
        mobile: payload.mobile,
        token: payload.token ?? existing?.token,
        userAgent: payload.userAgent ?? existing?.userAgent,
        deviceId: payload.deviceId ?? existing?.deviceId,
        uuid: payload.uuid ?? existing?.uuid,
        proxy: payload.proxy ?? existing?.proxy,
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
      const userAgent = normalizeWXAppUserAgent(payload.userAgent?.trim() || existing?.userAgent)
      const deviceId = payload.deviceId?.trim() || existing?.deviceId || randomHex(16)
      const uuid = payload.uuid?.trim() || existing?.uuid || `${Date.now()}_${randomHex(10)}`

      const saved = await this.upsert({
        id: existing?.id,
        username: existing?.username,
        mobile,
        proxy: payload.proxy?.trim() || existing?.proxy,
        userAgent,
        deviceId,
        uuid,
      })

      const idx = this.accounts.findIndex((a) => a.id === saved.id)
      if (idx >= 0) this.accounts[idx] = { ...this.accounts[idx]!, status: 'logging_in' }

      try {
        await apiLoginBySmsCode({
          mobile,
          smsCode,
          deviceType: 'WXAPP',
          userAgent,
          deviceId,
          uuid,
        })
        await this.refresh()
      } catch (e) {
        const idx2 = this.accounts.findIndex((a) => a.id === saved.id)
        if (idx2 >= 0) this.accounts[idx2] = { ...this.accounts[idx2]!, status: 'error' }
        throw e
      }
    },
    async loginByPassword(payload: {
      mobile: string
      password: string
      proxy?: string
      userAgent?: string
      deviceId?: string
      uuid?: string
    }) {
      const mobile = payload.mobile.trim()
      const password = payload.password
      if (!mobile) throw new Error('请输入手机号')
      if (!password) throw new Error('请输入密码')

      const existing = this.accounts.find((a) => a.mobile === mobile) ?? null
      const userAgent = normalizeWXAppUserAgent(payload.userAgent?.trim() || existing?.userAgent)
      const deviceId = payload.deviceId?.trim() || existing?.deviceId || randomHex(16)
      const uuid = payload.uuid?.trim() || existing?.uuid || `${Date.now()}_${randomHex(10)}`

      const saved = await this.upsert({
        id: existing?.id,
        username: existing?.username,
        mobile,
        proxy: payload.proxy?.trim() || existing?.proxy,
        userAgent,
        deviceId,
        uuid,
      })

      const idx = this.accounts.findIndex((a) => a.id === saved.id)
      if (idx >= 0) this.accounts[idx] = { ...this.accounts[idx]!, status: 'logging_in' }

      try {
        await apiLoginByPassword({
          identify: mobile,
          password,
          deviceType: 'WXAPP',
          userAgent,
          deviceId,
          uuid,
        })
        await this.refresh()
      } catch (e) {
        const idx2 = this.accounts.findIndex((a) => a.id === saved.id)
        if (idx2 >= 0) this.accounts[idx2] = { ...this.accounts[idx2]!, status: 'error' }
        throw e
      }
    },
    async logout(id: string) {
      const target = this.accounts.find((a) => a.id === id)
      if (!target) return

      await beUpsertAccount({
        id: target.id,
        username: target.username,
        mobile: target.mobile,
        token: '',
        userAgent: target.userAgent,
        deviceId: target.deviceId,
        uuid: target.uuid,
        proxy: target.proxy,
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

function normalizeWXAppUserAgent(ua?: string): string {
  const v = String(ua ?? '').trim()
  if (!v) return DEFAULT_WXAPP_UA
  const s = v.toLowerCase()
  if (s.includes('micromessenger') || s.includes('mobile') || s.includes('iphone') || s.includes('android')) return v
  return DEFAULT_WXAPP_UA
}
