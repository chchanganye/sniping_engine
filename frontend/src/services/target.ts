import axios from 'axios'
import { http } from '@/services/http'
import type { ShippingAddress, ShopCategoryNode, StoreSkuCategoryGroup } from '@/types/core'

export interface ApiEnvelope<T> {
  success: boolean
  data: T
  message?: string
  error?: string
  code?: number | string
}

function extractApiErrorMessage(error: unknown, fallback: string): string {
  if (axios.isAxiosError(error)) {
    const data = error.response?.data as any
    if (data) {
      if (typeof data === 'string') return data.trim() || fallback
      if (typeof data.error === 'string') return data.error
      if (typeof data.message === 'string') return data.message
    }
    return error.message || fallback
  }
  if (error instanceof Error) return error.message
  return fallback
}

export async function apiListShippingAddresses(
  token: string,
  params?: { app?: string; isAllCover?: number | string | boolean },
): Promise<ShippingAddress[]> {
  if (!token) throw new Error('缺少 token')
  try {
    const resp = await http.get<ApiEnvelope<ShippingAddress[]>>('/api/user/web/shipping-address/self/list-all', {
      params: {
        app: params?.app ?? 'o2o',
        isAllCover: params?.isAllCover ?? 1,
      },
      headers: {
        Authorization: `Bearer ${token}`,
        token,
        'x-token': token,
      },
    })
    if ((resp.data as any)?.error) throw new Error(String((resp.data as any).error))
    if (!resp.data?.success) throw new Error(resp.data?.message || '获取收货地址失败')
    return Array.isArray(resp.data.data) ? resp.data.data : []
  } catch (e) {
    throw new Error(extractApiErrorMessage(e, '获取收货地址失败'))
  }
}

export async function apiFetchShopCategoryTree(params: {
  frontCategoryId: number
  longitude: number
  latitude: number
  isFinish?: boolean
}): Promise<ShopCategoryNode[]> {
  try {
    const resp = await http.get<ApiEnvelope<ShopCategoryNode[]>>('/api/item/shop-category/tree', {
      params: {
        frontCategoryId: params.frontCategoryId,
        longitude: params.longitude,
        latitude: params.latitude,
        isFinish: params.isFinish ?? true,
      },
    })
    if ((resp.data as any)?.error) throw new Error(String((resp.data as any).error))
    if (!resp.data?.success) throw new Error(resp.data?.message || '获取分类失败')
    return Array.isArray(resp.data.data) ? resp.data.data : []
  } catch (e) {
    throw new Error(extractApiErrorMessage(e, '获取分类失败'))
  }
}

export async function apiSearchStoreSkuByCategory(params: {
  pageNo: number
  pageSize: number
  frontCategoryId: number
  longitude: number
  latitude: number
  isFinish?: boolean
}): Promise<StoreSkuCategoryGroup[]> {
  try {
    const resp = await http.get<ApiEnvelope<StoreSkuCategoryGroup[]>>(
      '/api/item/store/item/searchStoreSkuByCategory',
      {
        params: {
          pageNo: params.pageNo,
          pageSize: params.pageSize,
          frontCategoryId: params.frontCategoryId,
          longitude: params.longitude,
          latitude: params.latitude,
          isFinish: params.isFinish ?? true,
        },
      },
    )
    if ((resp.data as any)?.error) throw new Error(String((resp.data as any).error))
    if (!resp.data?.success) throw new Error(resp.data?.message || '获取分类商品失败')
    return Array.isArray(resp.data.data) ? resp.data.data : []
  } catch (e) {
    throw new Error(extractApiErrorMessage(e, '获取分类商品失败'))
  }
}

