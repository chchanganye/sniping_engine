import axios from 'axios'
import { http } from '@/services/http'
import type { CurrentUser } from '@/types/core'

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

export interface DesignPageItem {
  id: number
  name: string
  pageCategoryId?: number
  siteId?: number
  path?: string
  title?: string
  keywords?: string
  description?: string
  isIndex?: boolean
  pageType?: string
  createdAt?: string
  updatedAt?: string
  deletedAt?: string | null
  [key: string]: unknown
}

export async function apiFetchDesignPages(host = 'm.4008117117.com'): Promise<DesignPageItem[]> {
  try {
    const resp = await http.get<unknown>('/api/design/page/list', { params: { host } })
    const data: any = resp.data

    if (Array.isArray(data)) return data as DesignPageItem[]
    if (data?.success === true && Array.isArray(data?.data)) return data.data as DesignPageItem[]
    if (data?.error) throw new Error(String(data.error))

    throw new Error('商品列表返回结构不符合预期')
  } catch (e) {
    throw new Error(extractApiErrorMessage(e, '获取商品列表失败'))
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

export async function apiGetCurrentUser(token: string): Promise<CurrentUser> {
  if (!token) throw new Error('缺少 token')
  try {
    const resp = await http.get<ApiEnvelope<CurrentUser>>('/api/user/web/current-user', {
      headers: {
        Authorization: `Bearer ${token}`,
        token,
        'x-token': token,
      },
    })
    if ((resp.data as any)?.error) throw new Error(String((resp.data as any).error))
    if (!resp.data?.success) throw new Error(resp.data?.message || '获取用户信息失败')
    return resp.data.data
  } catch (e) {
    throw new Error(extractApiErrorMessage(e, '获取用户信息失败'))
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

export interface DesignPageDto {
  id: number
  name: string
  pageCategoryId?: number
  siteId?: number
  path?: string
  title?: string
  keywords?: string
  description?: string
  layout?: unknown
  isIndex?: boolean
  autoReleaseTime?: string | null
  autoExpirationTime?: string | null
  status?: unknown
  pageType?: string
  ext?: unknown
  createdAt?: string
  updatedAt?: string
  deletedAt?: string | null
}

export async function apiListDesignPages(host = 'm.4008117117.com'): Promise<DesignPageDto[]> {
  try {
    const resp = await http.get<ApiEnvelope<DesignPageDto[]> | DesignPageDto[]>('/api/design/page/list', {
      params: { host },
    })

    const data = resp.data as any
    if (Array.isArray(data)) return data as DesignPageDto[]
    if (data?.success === true && Array.isArray(data?.data)) return data.data as DesignPageDto[]
    if (Array.isArray(data?.data)) return data.data as DesignPageDto[]
    throw new Error('商品列表结构不符合预期')
  } catch (e) {
    throw new Error(extractApiErrorMessage(e, '获取商品列表失败'))
  }
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
