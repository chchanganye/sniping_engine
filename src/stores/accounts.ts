import { defineStore } from 'pinia'
import dayjs from 'dayjs'
import type { Account } from '@/types/core'
import { uid } from '@/utils/id'
import { sleep } from '@/utils/sleep'
import { useLogsStore } from '@/stores/logs'
import { apiGetCurrentUser, apiLoginBySmsCode, FIXED_DEVICE_ID } from '@/services/api'

const ACCOUNTS_STORAGE_KEY = 'rq_accounts_v1'
const UUID_BY_MOBILE_KEY = 'rq_uuid_by_mobile_v1'

function safeParseJson<T>(raw: string | null): T | null {
  if (!raw) return null
  try {
    return JSON.parse(raw) as T
  } catch {
    return null
  }
}

function randomHex20(): string {
  const bytes = 10 // 20 hex chars
  const buf = new Uint8Array(bytes)
  if (typeof crypto !== 'undefined' && 'getRandomValues' in crypto) {
    crypto.getRandomValues(buf)
  } else {
    for (let i = 0; i < bytes; i += 1) buf[i] = Math.floor(Math.random() * 256)
  }
  return Array.from(buf, (b) => b.toString(16).padStart(2, '0')).join('')
}

function createUuid(): string {
  return `${Date.now()}_${randomHex20()}`
}

function getOrCreateUuidForMobile(mobile: string): string {
  if (typeof window === 'undefined') return createUuid()
  const map = safeParseJson<Record<string, string>>(window.localStorage.getItem(UUID_BY_MOBILE_KEY)) ?? {}
  const key = mobile.trim()
  const existing = map[key]
  if (existing) return existing
  const next = createUuid()
  map[key] = next
  window.localStorage.setItem(UUID_BY_MOBILE_KEY, JSON.stringify(map))
  return next
}

function loadAccountsFromStorage(): Account[] | null {
  if (typeof window === 'undefined') return null
  const raw = window.localStorage.getItem(ACCOUNTS_STORAGE_KEY)
  const parsed = safeParseJson<any[]>(raw)
  if (!parsed || !Array.isArray(parsed)) return null

  return parsed
    .filter((a) => a && typeof a.id === 'string' && typeof a.username === 'string')
    .map((a) => {
      const username = String(a.username)
      const account: Account = {
        id: String(a.id),
        nickname: typeof a.nickname === 'string' ? a.nickname : `账号${username.slice(-4)}`,
        username,
        status: 'idle',
        lastActiveAt: dayjs().toISOString(),
        remark: typeof a.remark === 'string' ? a.remark : undefined,
        uuid: typeof a.uuid === 'string' ? a.uuid : getOrCreateUuidForMobile(username),
        deviceId: typeof a.deviceId === 'string' ? a.deviceId : undefined,
        token: typeof a.token === 'string' ? a.token : undefined,
        profile: a.profile && typeof a.profile === 'object' ? a.profile : undefined,
      }
      return account
    })
}

function saveAccountsToStorage(accounts: Account[]) {
  if (typeof window === 'undefined') return
  const payload = accounts.map((a) => ({
    id: a.id,
    nickname: a.nickname,
    username: a.username,
    remark: a.remark,
    uuid: a.uuid,
    deviceId: a.deviceId,
    token: a.token,
    profile: a.profile,
  }))
  window.localStorage.setItem(ACCOUNTS_STORAGE_KEY, JSON.stringify(payload))
}

function ensureDeviceId(account: Account): string {
  // Target requires a stable deviceId; use fixed value to simulate.
  account.deviceId = FIXED_DEVICE_ID
  return account.deviceId
}

function ensureUuid(account: Account): string {
  if (account.uuid) return account.uuid
  account.uuid = getOrCreateUuidForMobile(account.username)
  return account.uuid
}

function extractToken(payload: unknown): string | undefined {
  const data = payload as any
  const candidates = [
    data?.token,
    data?.data?.token,
    data?.data?.accessToken,
    data?.data?.access_token,
    data?.data?.jwt,
    data?.data?.extra?.token,
    data?.extra?.token,
  ]
  return candidates.find((v) => typeof v === 'string')
}

export const useAccountsStore = defineStore('accounts', {
  state: () => ({
    accounts: loadAccountsFromStorage() ?? ([] as Account[]),
  }),
  getters: {
    summary: (state) => {
      const total = state.accounts.length
      const loggedIn = state.accounts.filter((a) => a.status === 'logged_in' || a.status === 'running')
        .length
      const running = state.accounts.filter((a) => a.status === 'running').length
      const errors = state.accounts.filter((a) => a.status === 'error').length
      return { total, loggedIn, running, errors }
    },
  },
  actions: {
    persist() {
      saveAccountsToStorage(this.accounts)
    },
    addAccount(payload: { nickname: string; username: string; password?: string; remark?: string }) {
      const username = payload.username.trim()
      const account: Account = {
        id: uid('acc'),
        nickname: payload.nickname.trim(),
        username,
        password: payload.password ?? '',
        status: 'idle',
        lastActiveAt: dayjs().toISOString(),
        remark: payload.remark?.trim(),
        uuid: getOrCreateUuidForMobile(username),
        deviceId: FIXED_DEVICE_ID,
      }
      this.accounts.unshift(account)
      this.persist()
      return account
    },
    updateAccount(id: string, patch: Partial<Pick<Account, 'nickname' | 'username' | 'password' | 'remark'>>) {
      const target = this.accounts.find((a) => a.id === id)
      if (!target) return
      if (typeof patch.nickname === 'string') target.nickname = patch.nickname.trim()
      if (typeof patch.username === 'string') target.username = patch.username.trim()
      if (typeof patch.password === 'string') target.password = patch.password
      if (typeof patch.remark === 'string') target.remark = patch.remark.trim()
      target.lastActiveAt = dayjs().toISOString()
      this.persist()
    },
    removeAccount(id: string) {
      this.accounts = this.accounts.filter((a) => a.id !== id)
      this.persist()
    },
    async login(
      id: string,
      payload?: { smsCode?: string },
    ) {
      const target = this.accounts.find((a) => a.id === id)
      if (!target) return { ok: false, message: '账号不存在' }
      if (target.status === 'logging_in') return { ok: false, message: '账号登录中' }

      const logs = useLogsStore()

      if (!payload?.smsCode) {
        logs.addLog({
          level: 'warning',
          accountId: id,
          message: `账号「${target.nickname}」缺少短信验证码，请在「账号管理」里使用短信登录`,
        })
        return { ok: false, message: '缺少短信验证码' }
      }

      target.status = 'logging_in'
      logs.addLog({ level: 'info', accountId: id, message: `账号「${target.nickname}」开始短信登录…` })

      await sleep(800)

      if (!target.username) {
        target.status = 'error'
        logs.addLog({ level: 'error', accountId: id, message: '登录失败：账号为空' })
        return { ok: false, message: '账号为空' }
      }

      try {
        const deviceId = ensureDeviceId(target)
        const uuid = ensureUuid(target)
        const data = await apiLoginBySmsCode({
          mobile: target.username,
          smsCode: payload.smsCode,
          deviceType: 'WXAPP',
          deviceId,
          uuid,
        })
        target.auth = data
        target.token = extractToken(data)
        target.status = 'logged_in'
        target.lastActiveAt = dayjs().toISOString()
        logs.addLog({ level: 'success', accountId: id, message: `账号「${target.nickname}」登录成功` })

        if (target.token) {
          try {
            const profile = await apiGetCurrentUser(target.token)
            target.profile = profile
            const profileToken = extractToken(profile)
            if (profileToken) target.token = profileToken
          } catch (e) {
            const message = e instanceof Error ? e.message : '获取用户信息失败'
            logs.addLog({ level: 'warning', accountId: id, message: `账号「${target.nickname}」用户信息获取失败：${message}` })
          }
        }

        this.persist()
        return { ok: true }
      } catch (e) {
        const message = e instanceof Error ? e.message : '登录失败'
        target.status = 'error'
        target.lastActiveAt = dayjs().toISOString()
        logs.addLog({ level: 'error', accountId: id, message: `账号「${target.nickname}」登录失败：${message}` })
        this.persist()
        return { ok: false, message }
      }
    },
    logout(id: string) {
      const target = this.accounts.find((a) => a.id === id)
      if (!target) return
      const logs = useLogsStore()
      target.token = undefined
      target.auth = undefined
      target.profile = undefined
      target.status = 'idle'
      target.lastActiveAt = dayjs().toISOString()
      logs.addLog({ level: 'info', accountId: id, message: `账号「${target.nickname}」已退出` })
      this.persist()
    },
    start(id: string) {
      const target = this.accounts.find((a) => a.id === id)
      if (!target) return
      const logs = useLogsStore()
      if (target.status !== 'logged_in' && target.status !== 'running') {
        logs.addLog({ level: 'warning', accountId: id, message: `账号「${target.nickname}」未登录，无法启动` })
        return
      }
      target.status = 'running'
      target.lastActiveAt = dayjs().toISOString()
      logs.addLog({ level: 'info', accountId: id, message: `账号「${target.nickname}」开始运行（mock）` })
    },
    stop(id: string) {
      const target = this.accounts.find((a) => a.id === id)
      if (!target) return
      const logs = useLogsStore()
      if (target.status !== 'running') return
      target.status = 'logged_in'
      target.lastActiveAt = dayjs().toISOString()
      logs.addLog({ level: 'info', accountId: id, message: `账号「${target.nickname}」已停止运行` })
    },
    touch(id: string) {
      const target = this.accounts.find((a) => a.id === id)
      if (!target) return
      target.lastActiveAt = dayjs().toISOString()
    },
  },
})
