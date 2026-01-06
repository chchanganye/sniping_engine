import { http } from '@/services/http'
import axios from 'axios'

export interface BackendAccount {
  id: string
  username?: string
  mobile: string
  token?: string
  userAgent?: string
  deviceId?: string
  uuid?: string
  proxy?: string
  cookies?: any[]
  createdAt?: string
  updatedAt?: string
}

export type TargetMode = 'rush' | 'scan'

export interface BackendTarget {
  id: string
  name?: string
  imageUrl?: string
  itemId: number
  skuId: number
  shopId?: number
  mode: TargetMode
  targetQty: number
  perOrderQty: number
  rushAtMs?: number
  rushLeadMs?: number
  enabled: boolean
  createdAt?: string
  updatedAt?: string
}

export interface EngineTaskState {
  targetId: string
  running: boolean
  purchasedQty: number
  targetQty: number
  needCaptcha?: boolean
  lastError?: string
  lastAttemptMs?: number
  lastSuccessMs?: number
}

export interface EngineState {
  running: boolean
  tasks: EngineTaskState[]
}

export interface EngineTestBuyResult {
  canBuy: boolean
  needCaptcha?: boolean
  success: boolean
  orderId?: string
  traceId?: string
  message?: string
}

export interface EnginePreflightResult {
  canBuy: boolean
  needCaptcha: boolean
  totalFee: number
  traceId?: string
  message?: string
}

export interface EmailSettings {
  enabled: boolean
  email: string
  authCode?: string
}

export interface LimitsSettings {
  maxPerTargetInFlight: number
  captchaMaxInFlight: number
}

export interface CaptchaEngineStatus {
  state: 'stopped' | 'starting' | 'ready' | 'error'
  startedAtMs?: number
  readyAtMs?: number
  lastError?: string
  warmPages?: number
  pagePoolSize?: number
  totalPages?: number
  idlePages?: number
  busyPages?: number
  refreshingPages?: number
  solveCount?: number
  totalSolveMs?: number
  lastSolveAtMs?: number
  lastSolveMs?: number
  lastAttempts?: number
}

export interface CaptchaPageInfo {
  id: string
  state: 'idle' | 'busy' | 'refreshing' | 'unknown'
  createdAtMs: number
  lastUsedAtMs: number
  lastOpenedAtMs: number
  lastError?: string
}

export interface CaptchaPagesStatus {
  nowMs: number
  total: number
  idle: number
  busy: number
  refreshing: number
  pagePool: number
  pages: CaptchaPageInfo[]
}

export interface CaptchaPoolSettings {
  warmupSeconds: number
  poolSize: number
  itemTtlSeconds: number
}

export interface NotifySettings {
  rushExpireDisableMinutes: number
}

export interface CaptchaPoolItemView {
  id: string
  createdAtMs: number
  expiresAtMs: number
  preview?: string
}

export interface CaptchaPoolStatus {
  nowMs: number
  activated: boolean
  activateAtMs: number
  desiredSize: number
  size: number
  settings: CaptchaPoolSettings
  items: CaptchaPoolItemView[]
}

type DataEnvelope<T> = { data: T }

export async function beListAccounts(): Promise<BackendAccount[]> {
  const resp = await http.get<DataEnvelope<BackendAccount[]>>('/api/v1/accounts')
  return resp.data.data ?? []
}

export async function beUpsertAccount(account: Partial<BackendAccount> & { mobile: string }): Promise<BackendAccount> {
  const resp = await http.post<DataEnvelope<BackendAccount>>('/api/v1/accounts', account)
  return resp.data.data
}

export async function beDeleteAccount(id: string): Promise<void> {
  await http.delete('/api/v1/accounts', { params: { id } })
}

export async function beListTargets(): Promise<BackendTarget[]> {
  const resp = await http.get<DataEnvelope<BackendTarget[]>>('/api/v1/targets')
  return resp.data.data ?? []
}

export async function beUpsertTarget(target: Partial<BackendTarget> & Pick<BackendTarget, 'itemId' | 'skuId' | 'mode' | 'targetQty' | 'enabled'>): Promise<BackendTarget> {
  const resp = await http.post<DataEnvelope<BackendTarget>>('/api/v1/targets', target)
  return resp.data.data
}

export async function beDeleteTarget(id: string): Promise<void> {
  await http.delete('/api/v1/targets', { params: { id } })
}

export async function beEngineStart(): Promise<void> {
  await http.post('/api/v1/engine/start')
}

export async function beEngineStop(): Promise<void> {
  await http.post('/api/v1/engine/stop')
}

export async function beEngineState(): Promise<EngineState> {
  const resp = await http.get<DataEnvelope<EngineState>>('/api/v1/engine/state')
  return resp.data.data
}

export async function beEnginePreflight(targetId: string): Promise<EnginePreflightResult> {
  try {
    const resp = await http.post<DataEnvelope<EnginePreflightResult>>('/api/v1/engine/preflight', { targetId })
    return resp.data.data
  } catch (e) {
    throw new Error(extractBackendErrorMessage(e, '预检失败'))
  }
}

export async function beEngineTestBuy(targetId: string, captchaVerifyParam?: string, opId?: string): Promise<EngineTestBuyResult> {
  try {
    const resp = await http.post<DataEnvelope<EngineTestBuyResult>>('/api/v1/engine/test-buy', {
      targetId,
      captchaVerifyParam: captchaVerifyParam?.trim() || undefined,
      opId: opId?.trim() || undefined,
    })
    return resp.data.data
  } catch (e) {
    throw new Error(extractBackendErrorMessage(e, '测试抢购失败'))
  }
}

export async function beGetEmailSettings(): Promise<EmailSettings> {
  const resp = await http.get<DataEnvelope<EmailSettings>>('/api/v1/settings/email')
  return resp.data.data
}

export async function beSaveEmailSettings(payload: Partial<EmailSettings>): Promise<EmailSettings> {
  const resp = await http.post<DataEnvelope<EmailSettings>>('/api/v1/settings/email', payload)
  return resp.data.data
}

export async function beTestEmail(payload?: Partial<EmailSettings>): Promise<void> {
  try {
    await http.post('/api/v1/settings/email/test', payload ?? {})
  } catch (e) {
    throw new Error(extractBackendErrorMessage(e, '发送测试邮件失败'))
  }
}

export async function beGetLimitsSettings(): Promise<LimitsSettings> {
  const resp = await http.get<DataEnvelope<LimitsSettings>>('/api/v1/settings/limits')
  return resp.data.data
}

export async function beSaveLimitsSettings(payload: Partial<LimitsSettings>): Promise<LimitsSettings> {
  const resp = await http.post<DataEnvelope<LimitsSettings>>('/api/v1/settings/limits', payload)
  return resp.data.data
}

export async function beGetCaptchaPoolSettings(): Promise<CaptchaPoolSettings> {
  const resp = await http.get<DataEnvelope<CaptchaPoolSettings>>('/api/v1/settings/captcha-pool')
  return resp.data.data
}

export async function beSaveCaptchaPoolSettings(payload: Partial<CaptchaPoolSettings>): Promise<CaptchaPoolSettings> {
  const resp = await http.post<DataEnvelope<CaptchaPoolSettings>>('/api/v1/settings/captcha-pool', payload)
  return resp.data.data
}

export async function beGetNotifySettings(): Promise<NotifySettings> {
  const resp = await http.get<DataEnvelope<NotifySettings>>('/api/v1/settings/notify')
  return resp.data.data
}

export async function beSaveNotifySettings(payload: Partial<NotifySettings>): Promise<NotifySettings> {
  const resp = await http.post<DataEnvelope<NotifySettings>>('/api/v1/settings/notify', payload)
  return resp.data.data
}

export async function beCaptchaEngineState(): Promise<CaptchaEngineStatus> {
  const resp = await http.get<DataEnvelope<CaptchaEngineStatus>>('/api/v1/captcha/state')
  return resp.data.data
}

export async function beCaptchaPoolStatus(): Promise<CaptchaPoolStatus> {
  const resp = await http.get<DataEnvelope<CaptchaPoolStatus>>('/api/v1/captcha/pool')
  return resp.data.data
}

export async function beCaptchaPoolFill(count: number): Promise<{ added: number; failed: number }> {
  const resp = await http.post<DataEnvelope<{ added: number; failed: number }>>('/api/v1/captcha/pool/fill', { count })
  return resp.data.data
}

export async function beCaptchaPagesStatus(): Promise<CaptchaPagesStatus> {
  const resp = await http.get<DataEnvelope<CaptchaPagesStatus>>('/api/v1/captcha/pages')
  return resp.data.data
}

export async function beCaptchaPagesRefresh(payload?: { forceRecreate?: boolean; ensurePages?: number }): Promise<{ refreshed: number; recreated: number; failed: number }> {
  const resp = await http.post<DataEnvelope<{ refreshed: number; recreated: number; failed: number }>>('/api/v1/captcha/pages/refresh', payload ?? {})
  return resp.data.data
}

function extractBackendErrorMessage(error: unknown, fallback: string): string {
  if (axios.isAxiosError(error)) {
    const data = error.response?.data as any
    if (data) {
      if (typeof data === 'string') return data.trim() || fallback
      if (typeof data.error === 'string' && data.error.trim()) return data.error.trim()
      if (typeof data.message === 'string' && data.message.trim()) return data.message.trim()
    }
    return error.message || fallback
  }
  if (error instanceof Error) return error.message
  return fallback
}
