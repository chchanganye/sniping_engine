import { defineStore } from 'pinia'
import type { LogCategory, LogEntry, LogLevel } from '@/types/core'
import { uid } from '@/utils/id'
import { useTasksStore } from '@/stores/tasks'
import { useProgressStore, type ProgressEventPayload } from '@/stores/progress'

type BusMessage =
  | { type: 'log'; time: number; data: { level: string; msg: string; fields?: Record<string, any> } }
  | { type: 'progress'; time: number; data: ProgressEventPayload }
  | { type: 'task_state'; time: number; data: any }
  | { type: string; time: number; data: any }

function mapLevel(level: string): LogLevel {
  const v = (level || '').toLowerCase()
  if (v === 'error') return 'error'
  if (v === 'warn' || v === 'warning') return 'warning'
  if (v === 'success') return 'success'
  return 'info'
}

function detectLogCategory(msg: string, fields?: Record<string, any>): LogCategory {
  const m = (msg || '').toLowerCase()
  const api = typeof fields?.api === 'string' ? fields.api.trim().toLowerCase() : ''
  const url = typeof fields?.url === 'string' ? fields.url.trim().toLowerCase() : ''

  if (
    api ||
    url ||
    m.includes('http request') ||
    m.includes('proxy request') ||
    m.includes('upstream request') ||
    m.includes('上游请求失败') ||
    m.includes('发送网络请求') ||
    m.includes('代理请求')
  ) {
    return 'network'
  }
  if (fields?.targetId || fields?.accountId) {
    return 'rush'
  }
  if (m.includes('engine') || m.includes('server') || m.includes('captcha engine') || m.includes('settings')) {
    return 'system'
  }
  return 'other'
}

function buildWsURL(path: string): string {
  const loc = window.location
  const proto = loc.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${loc.host}${path}`
}

function replaceAllLiteral(input: string, search: string, replacement: string) {
  if (!search) return input
  return input.split(search).join(replacement)
}

function normalizeErrorText(raw: string) {
  let s = (raw || '').trim()
  if (!s) return ''
  s = replaceAllLiteral(s, 'verification.code.failed', '验证码校验失败')
  s = replaceAllLiteral(s, 'create-order', '创建订单')
  s = replaceAllLiteral(s, 'render-order', '预下单')
  s = replaceAllLiteral(s, 'captcha', '验证码')
  s = replaceAllLiteral(s, 'context deadline exceeded', '请求超时')
  return s
}

function summarizeRequest(fields?: Record<string, any>) {
  const method = typeof fields?.method === 'string' ? fields.method.trim().toUpperCase() : ''
  const url = typeof fields?.url === 'string' ? fields.url.trim() : ''
  const api = typeof fields?.api === 'string' ? fields.api.trim() : ''
  const picked = api || url
  if (!picked) return ''
  return method ? `${method} ${picked}` : picked
}

function formatIds(fields?: Record<string, any>) {
  if (!fields) return ''
  const taskId = typeof fields.targetId === 'string' ? fields.targetId.trim() : ''
  const accountId = typeof fields.accountId === 'string' ? fields.accountId.trim() : ''
  const orderId = fields.orderId != null ? String(fields.orderId).trim() : ''
  const parts: string[] = []
  if (taskId) parts.push(`任务：${taskId}`)
  if (accountId) parts.push(`账号：${accountId}`)
  if (orderId) parts.push(`订单：${orderId}`)
  return parts.length ? `（${parts.join('，')}）` : ''
}

function friendlyLogMessage(msg: string, fields?: Record<string, any>) {
  const m = (msg || '').trim()
  if (!m) return ''

  if (m === 'server starting') return `服务启动中…${fields?.addr ? `（${String(fields.addr)}）` : ''}`
  if (m === 'http server listening') return `服务已启动，正在监听：${String(fields?.addr ?? '-')}`
  if (m === 'shutdown signal received') return `收到退出信号，正在停止服务…${fields?.signal ? `（${String(fields.signal)}）` : ''}`
  if (m === 'server stopped') return '服务已停止'
  if (m === 'listen failed') return `监听端口失败：${normalizeErrorText(String(fields?.error ?? '')) || '未知错误'}`
  if (m === 'http server error') return `服务异常：${normalizeErrorText(String(fields?.error ?? '')) || '未知错误'}`

  if (m === 'engine start') return `引擎已启动${fields?.provider ? `（${String(fields.provider)}）` : ''}`
  if (m === 'engine stopped') return '引擎已停止'
  if (m === 'target waiting for rush time') return '等待开抢时间…'
  if (m === 'preflight: canBuy=false') return '预下单结果：当前不可购买'
  if (m === '预下单失败') return `预下单失败：${normalizeErrorText(String(fields?.error ?? '')) || '未知错误'}${formatIds(fields)}`
  if (m === '预下单成功，准备下单') {
    const needCaptcha = fields?.needCaptcha === true ? '需要验证码' : '不需要验证码'
    return `预下单成功：${needCaptcha}${formatIds(fields)}`
  }
  if (m === '提交订单中') return `正在提交订单…${formatIds(fields)}`
  if (m === '下单失败') return `下单失败：${normalizeErrorText(String(fields?.error ?? '')) || '未知错误'}${formatIds(fields)}`
  if (m === '开始测试抢购') return `开始测试抢购…${formatIds(fields)}`
  if (m === '测试下单失败') return `测试下单失败：${normalizeErrorText(String(fields?.error ?? '')) || '未知错误'}${formatIds(fields)}`

  if (m === 'captcha engine starting') return '验证码引擎状态：启动中'
  if (m === 'captcha engine ready') return `验证码引擎状态：已就绪（预热页：${String(fields?.warmPages ?? '-')}，池：${String(fields?.pagePoolSize ?? '-')}）`
  if (m === 'captcha engine warmup failed') return `验证码引擎状态：启动失败（${normalizeErrorText(String(fields?.error ?? '')) || '未知错误'}）`

  if (m === 'captcha solving') return `验证码处理中…${formatIds(fields)}`

  if (m === 'captcha solved') {
    const attempts = fields?.attempts != null ? String(fields.attempts) : '-'
    const sec = fields?.costSec != null ? String(fields.costSec) : fields?.costMs != null ? String(Number(fields.costMs) / 1000) : '-'
    return `验证码通过：尝试${attempts}次，总耗时${sec}秒${formatIds(fields)}`
  }
  if (m === 'captcha solve failed') {
    const attempts = fields?.attempts != null ? String(fields.attempts) : '-'
    const sec = fields?.costSec != null ? String(fields.costSec) : fields?.costMs != null ? String(Number(fields.costMs) / 1000) : '-'
    const reason = normalizeErrorText(String(fields?.error ?? '')) || '未知错误'
    return `验证码失败：尝试${attempts}次，总耗时${sec}秒，原因：${reason}${formatIds(fields)}`
  }

  if (m === '验证码池开始维护') {
    const at = fields?.activateAtMs != null ? String(fields.activateAtMs) : ''
    return at ? `验证码池开始维护（activateAtMs=${at}）` : '验证码池开始维护'
  }
  if (m === '验证码池：生成失败') {
    const attempts = fields?.attempts != null ? String(fields.attempts) : '-'
    const sec = fields?.costSec != null ? String(fields.costSec) : fields?.costMs != null ? String(Number(fields.costMs) / 1000) : '-'
    const reason = normalizeErrorText(String(fields?.error ?? '')) || '未知错误'
    return `验证码池生成失败：${reason}（尝试${attempts}次，耗时${sec}秒）`
  }
  if (m === '验证码池：手动补充开始') {
    const count = fields?.count != null ? String(fields.count) : '-'
    return `验证码池手动补充开始：${count}条`
  }
  if (m === '验证码池：手动补充完成') {
    const added = fields?.added != null ? String(fields.added) : '-'
    const failed = fields?.failed != null ? String(fields.failed) : '-'
    const size = fields?.size != null ? String(fields.size) : ''
    return size ? `验证码池手动补充完成：新增${added}，失败${failed}（当前${size}）` : `验证码池手动补充完成：新增${added}，失败${failed}`
  }
  if (m === '验证码池：手动补充失败') {
    const added = fields?.added != null ? String(fields.added) : '-'
    const failed = fields?.failed != null ? String(fields.failed) : '-'
    const reason = normalizeErrorText(String(fields?.error ?? '')) || '未知错误'
    return `验证码池手动补充失败：新增${added}，失败${failed}，原因：${reason}`
  }
  if (m === '验证码池：人工补充完成') {
    const added = fields?.added != null ? String(fields.added) : '-'
    const size = fields?.size != null ? String(fields.size) : ''
    return size ? `验证码池人工补充完成：新增${added}（当前${size}）` : `验证码池人工补充完成：新增${added}`
  }

  if (m === 'http request') return `正在发送网络请求：${summarizeRequest(fields) || '请求中…'}`
  if (m === 'proxy request') return `正在代理请求：${summarizeRequest(fields) || '请求中…'}`

  if (m === 'upstream request failed' || m === '上游请求失败') {
    const reason = normalizeErrorText(String(fields?.error ?? '')) || '上游返回异常'
    const api = typeof fields?.api === 'string' ? String(fields.api).trim() : ''
    return api ? `上游请求失败（${api}）：${reason}` : `上游请求失败：${reason}`
  }

  if (m === 'task error') return `任务执行失败：${normalizeErrorText(String(fields?.error ?? '')) || '未知错误'}${formatIds(fields)}`
  if (m === 'order created') return `下单成功${formatIds(fields)}`
  if (m === 'order created (test)') return `测试下单成功${formatIds(fields)}`

  if (m === 'email sent' || m === '通知邮件已发送') {
    const count = fields?.count != null ? String(fields.count) : ''
    const summary = count ? `（汇总${count}条）` : ''
    const to = fields?.to ? `（${String(fields.to)}）` : ''
    return `通知邮件已发送${summary}${to}${formatIds(fields)}`
  }
  if (m === 'email send failed' || m === '邮件发送失败') {
    const count = fields?.count != null ? String(fields.count) : ''
    const summary = count ? `（汇总${count}条）` : ''
    return `邮件发送失败${summary}：${normalizeErrorText(String(fields?.error ?? '')) || '未知错误'}${formatIds(fields)}`
  }
  if (m === 'email notify disabled' || m === '邮件通知未启用') {
    const count = fields?.count != null ? String(fields.count) : ''
    return count ? `邮件通知未启用（已忽略${count}条结果）` : '邮件通知未启用'
  }
  if (m === 'email settings invalid' || m === '邮件配置无效') {
    return `邮件配置无效：${normalizeErrorText(String(fields?.error ?? '')) || '请检查设置'}`
  }
  if (m === 'load email settings failed' || m === '读取邮件配置失败') {
    return `读取邮件配置失败：${normalizeErrorText(String(fields?.error ?? '')) || '未知错误'}`
  }
  if (m === 'email notify dropped (queue full)' || m === '邮件通知丢弃：队列已满') {
    return `邮件通知丢弃：队列已满${formatIds(fields)}`
  }

  // 已经是中文/或无法识别：尽量把常见错误码转成中文
  const normalized = normalizeErrorText(m)
  return normalized === m ? m : normalized
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
        category: payload.category,
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
      const progressStore = useProgressStore()
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

          if (msg.type === 'progress') {
            const time = typeof msg.time === 'number' ? msg.time : Date.now()
            progressStore.addEvent(time, msg.data)
            return
          }

          if (msg.type === 'log') {
            const at = typeof msg.time === 'number' ? new Date(msg.time).toISOString() : new Date().toISOString()
            const fields = msg.data?.fields as Record<string, any> | undefined
            const rawMsg = String(msg.data?.msg ?? '')
            const category = detectLogCategory(rawMsg, fields)
            const text = friendlyLogMessage(rawMsg, fields) || '（空日志）'

            this.addLog({
              at,
              level: mapLevel(String(msg.data?.level ?? 'info')),
              category,
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
