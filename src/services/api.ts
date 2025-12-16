import axios from 'axios'
import { http } from '@/services/http'

export const FIXED_DEVICE_ID = '9b1be8c5f55fbf03a36ba7cfc6db4e54'

export interface ApiEnvelope<T> {
  success: boolean
  data: T
  message?: string
  code?: number | string
}

export interface CaptchaData {
  token: string
  imageUrl: string
}

export async function apiGetCaptcha(): Promise<CaptchaData> {
  try {
    const resp = await http.get<ApiEnvelope<CaptchaData>>('/api/user/web/get-captcha')
    if (!resp.data?.success) {
      throw new Error(resp.data?.message || '获取图形验证码失败')
    }
    return resp.data.data
  } catch (e) {
    throw new Error(extractApiErrorMessage(e, '获取图形验证码失败'))
  }
}

export interface SendSmsCodeParams {
  mobile: string
  captcha: string
  token: string
}

export async function apiSendSmsCode(params: SendSmsCodeParams): Promise<boolean> {
  try {
    const resp = await http.post<ApiEnvelope<boolean>>('/api/user/web/login/login-send-sms-code', params)
    if (!resp.data?.success) {
      throw new Error(resp.data?.message || '发送短信验证码失败')
    }
    return Boolean(resp.data.data)
  } catch (e) {
    throw new Error(extractApiErrorMessage(e, '发送短信验证码失败'))
  }
}

export interface LoginBySmsCodeParams {
  mobile: string
  smsCode: string
  app?: boolean
  deviceId?: string
  deviceType?: string
  userAgent?: string
  uuid?: string
  deviceSource?: string
}

export type LoginBySmsCodeResponse = Record<string, unknown>

export async function apiLoginBySmsCode(params: LoginBySmsCodeParams): Promise<LoginBySmsCodeResponse> {
  const payload = {
    mobile: params.mobile,
    smsCode: params.smsCode,
    app: params.app ?? true,
    deviceId: params.deviceId ?? defaultDeviceId(),
    deviceType: params.deviceType ?? 'WXAPP',
    userAgent: params.userAgent ?? defaultUserAgent(),
    uuid: params.uuid ?? defaultUuid(),
    deviceSource: params.deviceSource ?? defaultDeviceSource(),
  }

  try {
    const resp = await http.post<LoginBySmsCodeResponse>('/api/user/web/login/login-by-sms-code', payload)
    const data = resp.data as any
    if (data?.error) throw new Error(String(data.error))
    if (data?.success === false) throw new Error(String(data?.message ?? '登录失败'))
    return resp.data
  } catch (e) {
    throw new Error(extractApiErrorMessage(e, '登录失败'))
  }
}

export interface LoginParams {
  username: string
  password: string
}

export async function apiLogin(params: LoginParams): Promise<{ token: string }> {
  // TODO: Implement SMS login based on the target site's API
  void params
  return { token: 'mock_token' }
}

export async function apiFetchGoods(): Promise<unknown> {
  // TODO: Implement based on the target site's API
  return http.get('/mock/goods')
}

export async function apiPlaceOrder(): Promise<unknown> {
  // TODO: Implement based on the target site's API
  return http.post('/mock/order')
}

function extractApiErrorMessage(error: unknown, fallback: string): string {
  if (axios.isAxiosError(error)) {
    const data = error.response?.data as any

    if (data) {
      if (typeof data === 'string') {
        const text = data.trim()
        try {
          const parsed = JSON.parse(text)
          if (parsed && typeof parsed.error === 'string') return parsed.error
          if (parsed && typeof parsed.message === 'string') return parsed.message
        } catch {
          // ignore
        }
        return text
      }

      if (typeof data.error === 'string') return data.error
      if (typeof data.message === 'string') return data.message
    }

    return error.message || fallback
  }

  if (error instanceof Error) return error.message
  return fallback
}

function randomHex(bytes: number): string {
  const buf = new Uint8Array(bytes)
  if (typeof crypto !== 'undefined' && 'getRandomValues' in crypto) {
    crypto.getRandomValues(buf)
  } else {
    for (let i = 0; i < bytes; i += 1) buf[i] = Math.floor(Math.random() * 256)
  }
  return Array.from(buf, (b) => b.toString(16).padStart(2, '0')).join('')
}

function defaultDeviceId(): string {
  return FIXED_DEVICE_ID
}

function defaultUuid(): string {
  return `${Date.now()}_${randomHex(10)}`
}

function defaultUserAgent(): string {
  if (typeof navigator === 'undefined') return ''
  return navigator.userAgent
}

function defaultDeviceSource(): string {
  if (typeof window === 'undefined') return 'unknown'
  const w = window.screen?.width ?? window.innerWidth
  const h = window.screen?.height ?? window.innerHeight
  return `${w}*${h} devices`
}
