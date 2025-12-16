export type AccountStatus = 'idle' | 'logging_in' | 'logged_in' | 'running' | 'error'

export interface Account {
  id: string
  nickname: string
  username: string
  password?: string
  token?: string
  status: AccountStatus
  lastActiveAt?: string
  remark?: string
}

export interface GoodsItem {
  id: string
  title: string
  price: number
  stock: number
  startAt?: string
  endAt?: string
  tags?: string[]
}

export type TaskStatus = 'idle' | 'scheduled' | 'running' | 'success' | 'failed' | 'stopped'

export interface Task {
  id: string
  goodsId: string
  goodsTitle: string
  accountIds: string[]
  quantity: number
  scheduleAt?: string
  status: TaskStatus
  createdAt: string
  lastMessage?: string
  successCount: number
  failCount: number
}

export type LogLevel = 'info' | 'success' | 'warning' | 'error'

export interface LogEntry {
  id: string
  at: string
  level: LogLevel
  accountId?: string
  taskId?: string
  message: string
}
