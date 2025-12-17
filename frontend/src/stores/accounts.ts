import { defineStore } from 'pinia'
import type { Account } from '@/types/core'
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
    async remove(id: string) {
      await beDeleteAccount(id)
      this.accounts = this.accounts.filter((a) => a.id !== id)
    },
  },
})

