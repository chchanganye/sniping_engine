import { http } from '@/services/http'

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
  const resp = await http.get<ApiEnvelope<CaptchaData>>('/api/user/web/get-captcha')
  if (!resp.data?.success) {
    throw new Error(resp.data?.message || '获取图形验证码失败')
  }
  return resp.data.data
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
