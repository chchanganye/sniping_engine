import { defineStore } from 'pinia'
import dayjs from 'dayjs'
import type { Account, AccountStatus } from '@/types/core'
import { uid } from '@/utils/id'
import { sleep } from '@/utils/sleep'
import { useLogsStore } from '@/stores/logs'

export const useAccountsStore = defineStore('accounts', {
  state: () => ({
    accounts: [
      {
        id: uid('acc'),
        nickname: '主号',
        username: '13800000001',
        password: '',
        status: 'idle' as AccountStatus,
        lastActiveAt: dayjs().subtract(10, 'minute').toISOString(),
        remark: '示例账号（可删除）',
      },
      {
        id: uid('acc'),
        nickname: '副号',
        username: '13800000002',
        password: '',
        status: 'idle' as AccountStatus,
        lastActiveAt: dayjs().subtract(25, 'minute').toISOString(),
        remark: '示例账号（可删除）',
      },
    ] as Account[],
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
    addAccount(payload: { nickname: string; username: string; password?: string; remark?: string }) {
      const account: Account = {
        id: uid('acc'),
        nickname: payload.nickname.trim(),
        username: payload.username.trim(),
        password: payload.password ?? '',
        status: 'idle',
        lastActiveAt: dayjs().toISOString(),
        remark: payload.remark?.trim(),
      }
      this.accounts.unshift(account)
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
    },
    removeAccount(id: string) {
      this.accounts = this.accounts.filter((a) => a.id !== id)
    },
    async login(
      id: string,
      payload?: { captchaToken?: string; captchaCode?: string; smsCode?: string },
    ) {
      const target = this.accounts.find((a) => a.id === id)
      if (!target) return
      if (target.status === 'logging_in') return

      const logs = useLogsStore()
      target.status = 'logging_in'
      if (payload) {
        logs.addLog({
          level: 'info',
          accountId: id,
          message: `账号「${target.nickname}」开始短信登录…（captchaToken=${payload.captchaToken ? '有' : '无'}）`,
        })
      } else {
        logs.addLog({ level: 'info', accountId: id, message: `账号「${target.nickname}」开始登录…` })
      }

      await sleep(800)

      if (!target.username) {
        target.status = 'error'
        logs.addLog({ level: 'error', accountId: id, message: '登录失败：账号为空' })
        return
      }

      target.token = `mock_${uid('token')}`
      target.status = 'logged_in'
      target.lastActiveAt = dayjs().toISOString()
      logs.addLog({ level: 'success', accountId: id, message: `账号「${target.nickname}」登录成功（mock）` })
    },
    logout(id: string) {
      const target = this.accounts.find((a) => a.id === id)
      if (!target) return
      const logs = useLogsStore()
      target.token = undefined
      target.status = 'idle'
      target.lastActiveAt = dayjs().toISOString()
      logs.addLog({ level: 'info', accountId: id, message: `账号「${target.nickname}」已退出` })
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
