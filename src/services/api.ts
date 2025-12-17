import axios from 'axios'
import { http } from '@/services/http'
import type { CurrentUser, ShippingAddress, ShopCategoryNode, StoreSkuCategoryGroup } from '@/types/core'

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
    const resp = await http.get<ApiEnvelope<StoreSkuCategoryGroup[]>>('/api/item/store/item/searchStoreSkuByCategory', {
      params: {
        pageNo: params.pageNo,
        pageSize: params.pageSize,
        frontCategoryId: params.frontCategoryId,
        longitude: params.longitude,
        latitude: params.latitude,
        isFinish: params.isFinish ?? true,
      },
    })
    if ((resp.data as any)?.error) throw new Error(String((resp.data as any).error))
    if (!resp.data?.success) throw new Error(resp.data?.message || '获取分类商品失败')
    return Array.isArray(resp.data.data) ? resp.data.data : []
  } catch (e) {
    throw new Error(extractApiErrorMessage(e, '获取分类商品失败'))
  }
}

export type TradeDeviceSource = 'H5' | 'WXAPP' | string

export interface TradeRenderOrderLine {
  skuId: number
  itemId: number
  quantity: number
  promotionTag?: unknown
  activityId?: unknown
  extra?: Record<string, unknown>
  shopId: number
  [key: string]: unknown
}

export interface TradeRenderOrderRequest {
  deviceSource: TradeDeviceSource
  orderSource?: string
  buyConfig?: Record<string, unknown>
  itemName?: string | null
  orderLineList: TradeRenderOrderLine[]
  divisionIds?: string
  addressId?: number | null
  couponParams?: TradeCreateOrderCouponParam[]
  benefitParams?: TradeCreateOrderBenefitParam[]
  delivery?: Record<string, unknown>
  extra?: Record<string, unknown>
  devicesId?: string
  [key: string]: unknown
}

export interface TradeRenderOrderResponseData {
  extra?: Record<string, unknown>
  orderList?: unknown[]
  addressInfoList?: Array<Record<string, unknown>>
  deliveryInfoList?: unknown[]
  invoiceInfoList?: unknown[]
  totalSkuNum?: number
  shipFee?: unknown
  memberPointsDeductionInfo?: Record<string, unknown>
  shopDiscountFee?: number
  platformDiscountFee?: number
  totalTaxFee?: number
  skuTotalFee?: number
  priceInfo?: Record<string, unknown>
  totalFee?: number
  purchaseStatus?: Record<string, unknown> & { canBuy?: boolean }
  visibleInfo?: Record<string, unknown>
  benefitInfo?: unknown
  couponInfo?: unknown
  sellCouponInfo?: unknown
  orderLineList?: unknown[]
  couponParams?: TradeCreateOrderCouponParam[]
  benefitParams?: TradeCreateOrderBenefitParam[]
  delivery?: Record<string, unknown>
  [key: string]: unknown
}

export async function apiTradeRenderOrder(
  token: string,
  payload: TradeRenderOrderRequest,
  options?: { signal?: AbortSignal },
): Promise<TradeRenderOrderResponseData> {
  if (!token) throw new Error('缺少 token')
  try {
    const resp = await http.post<ApiEnvelope<TradeRenderOrderResponseData>>('/api/trade/buy/render-order', payload, {
      headers: {
        Authorization: `Bearer ${token}`,
        token,
        'x-token': token,
      },
      signal: options?.signal,
    })
    if ((resp.data as any)?.error) throw new Error(String((resp.data as any).error))
    if (!resp.data?.success) throw new Error(resp.data?.message || '渲染订单失败')
    return resp.data.data
  } catch (e) {
    throw new Error(extractApiErrorMessage(e, '渲染订单失败'))
  }
}

export interface BuildTradeRenderOrderPayloadParams {
  sku: {
    itemId: number
    skuId: number
    shopId: number
    skuName?: string | null
  }
  quantity: number
  devicesId?: string
  divisionIds?: string
  addressId?: number | null
  deviceSource?: TradeDeviceSource
  orderSource?: string
}

export function buildTradeRenderOrderPayload(params: BuildTradeRenderOrderPayloadParams): TradeRenderOrderRequest {
  const deviceSource: TradeDeviceSource = params.deviceSource ?? 'WXAPP'
  const orderSource = params.orderSource ?? 'product.detail.page'
  const itemId = Number(params.sku.itemId)
  const skuId = Number(params.sku.skuId)
  const shopId = Number(params.sku.shopId)

  return {
    deviceSource,
    orderSource,
    buyConfig: { lineGrouped: true, multipleCoupon: true },
    itemName: params.sku.skuName ?? null,
    orderLineList: [
      {
        skuId,
        itemId,
        quantity: params.quantity,
        promotionTag: null,
        activityId: null,
        extra: {},
        shopId,
      },
    ],
    divisionIds: params.divisionIds,
    addressId: typeof params.addressId === 'number' ? params.addressId : null,
    couponParams: [],
    benefitParams: [],
    delivery: {},
    extra: {
      renewOriginOrderId: '',
      renewOriginAddressId: '',
      activityGroupId: null,
    },
    devicesId: params.devicesId,
  }
}

function pickRenderAddressId(render: TradeRenderOrderResponseData): number | undefined {
  const list = Array.isArray(render.addressInfoList) ? render.addressInfoList : []
  const pick = list.find((a: any) => a?.checked === true) ?? list.find((a: any) => a?.isDefault === true) ?? list[0]
  const id = (pick as any)?.id
  return typeof id === 'number' && Number.isFinite(id) ? id : undefined
}

function pickRenderSkuName(render: TradeRenderOrderResponseData): string | null {
  const line0 = Array.isArray((render as any).orderLineList) ? (render as any).orderLineList[0] : undefined
  if (line0 && typeof line0.skuName === 'string') return line0.skuName

  const order0 = Array.isArray(render.orderList) ? (render.orderList as any)[0] : undefined
  const skuName = order0?.activityOrderList?.[0]?.orderLineGroups?.[0]?.orderLineList?.[0]?.skuName
  return typeof skuName === 'string' ? skuName : null
}

function pickRenderTotalFee(render: TradeRenderOrderResponseData): number | undefined {
  if (typeof render.totalFee === 'number' && Number.isFinite(render.totalFee)) return render.totalFee
  const candidate = (render.priceInfo as any)?.totalFee
  return typeof candidate === 'number' && Number.isFinite(candidate) ? candidate : undefined
}

export function buildTradeCreateOrderPayloadFromRender(
  render: TradeRenderOrderResponseData,
  params?: { deviceSource?: TradeDeviceSource; buyConfig?: Record<string, unknown>; orderSource?: string },
): TradeCreateOrderRequest {
  const deviceSource: TradeDeviceSource = params?.deviceSource ?? 'WXAPP'
  const renderOrderSource =
    params?.orderSource ??
    (typeof render.extra?.orderSource === 'string' ? String(render.extra.orderSource) : undefined) ??
    'product.detail.page'

  const addressId = pickRenderAddressId(render)
  if (typeof addressId !== 'number') {
    throw new Error('render-order 未返回可用的 addressId')
  }

  const orderList = Array.isArray(render.orderList) ? render.orderList : null
  if (!orderList) throw new Error('render-order 未返回 orderList')

  const priceInfo = render.priceInfo
  if (!priceInfo || typeof priceInfo !== 'object') throw new Error('render-order 未返回 priceInfo')

  const totalFee = pickRenderTotalFee(render)
  if (typeof totalFee !== 'number') throw new Error('render-order 未返回 totalFee')

  const extra = { ...(render.extra ?? {}), deviceSource }
  const itemName = pickRenderSkuName(render)

  const payload: any = {
    ...render,
    deviceSource,
    orderSource: renderOrderSource,
    buyConfig: params?.buyConfig ?? { lineGrouped: true, multipleCoupon: true },
    itemName,
    addressId,
    orderList,
    priceInfo,
    totalFee,
    extra,
    devicesId: (render as any).devicesId ?? (typeof render.extra?.devicesId === 'string' ? render.extra.devicesId : undefined),
  }
  if (payload.shipFeeInfo == null && render.shipFee != null) payload.shipFeeInfo = render.shipFee

  return payload as TradeCreateOrderRequest
}

export interface TradeCreateOrderCouponParam {
  activityId: number
  benefitId?: number | null
  shopId: number
  [key: string]: unknown
}

export interface TradeCreateOrderBenefitParam {
  activityId: number
  benefitId?: number | null
  shopId: number
  benefitType?: unknown
  amount?: unknown
  [key: string]: unknown
}

export interface TradeCreateOrderLineSkuAttr {
  attrKey: string
  attrVal: string
}

export interface TradeCreateOrderLine {
  itemId: number
  skuId: number
  skuCode?: string | null
  bundleId?: number | string | null
  quantity: number
  activityId?: number | string | null
  shopActivityId?: number | string | null
  extraParam?: unknown
  promotionTag?: unknown
  shopId: number
  lineId?: string
  categoryId?: number | null
  skuName?: string | null
  attrs?: TradeCreateOrderLineSkuAttr[] | null
  mainImage?: string | null
  outerSkuCode?: string | null
  status?: number | null
  salePrice?: number | null
  preferSalePrice?: number | null
  extra?: Record<string, unknown> | null
  summary?: Record<string, unknown> | null
  bizCode?: string | null
  itemAttributes?: Record<string, unknown> | null
  [key: string]: unknown
}

export interface TradeCreateOrderRequest {
  deviceSource: TradeDeviceSource
  orderSource?: string
  buyConfig?: Record<string, unknown>
  memberPointsDeductionInfo?: Record<string, unknown>
  itemName?: string | null
  mobile?: string | null
  invoice?: unknown
  addressId: number
  couponParams?: TradeCreateOrderCouponParam[]
  benefitParams?: TradeCreateOrderBenefitParam[]
  orderList: unknown[]
  extraParam?: unknown
  extra?: Record<string, unknown>
  delivery?: Record<string, unknown>
  [key: string]: unknown
}

export interface TradeCreateOrderResponseData {
  purchaseOrderId?: number
  orderInfos?: Array<{
    orderId: number
    orderLineInfos?: Array<{
      orderLineId: number
      skuId: number
      quantity: number
    }>
  }>
  buyerId?: number
  orderLineList?: unknown[]
  extra?: Record<string, unknown>
  [key: string]: unknown
}

export async function apiTradeCreateOrder(
  token: string,
  payload: TradeCreateOrderRequest,
  options?: { signal?: AbortSignal },
): Promise<TradeCreateOrderResponseData> {
  if (!token) throw new Error('缺少 token')
  try {
    const resp = await http.post<ApiEnvelope<TradeCreateOrderResponseData>>('/api/trade/buy/create-order', payload, {
      headers: {
        Authorization: `Bearer ${token}`,
        token,
        'x-token': token,
      },
      signal: options?.signal,
    })
    if ((resp.data as any)?.error) throw new Error(String((resp.data as any).error))
    if (!resp.data?.success) throw new Error(resp.data?.message || '创建订单失败')
    return resp.data.data
  } catch (e) {
    throw new Error(extractApiErrorMessage(e, '创建订单失败'))
  }
}

export interface BuildTradeCreateOrderSkuParams {
  itemId: number
  skuId: number
  shopId: number
  skuCode?: string | null
  categoryId?: number | null
  skuName?: string | null
  mainImage?: string | null
  salePrice?: number | null
  fullUnit?: string | null
  itemAttributes?: Record<string, unknown> | null
}

export interface BuildTradeCreateOrderPayloadParams {
  addressId: number
  quantity: number
  sku: BuildTradeCreateOrderSkuParams
  deviceSource?: TradeDeviceSource
  orderSource?: string
  devicesId?: string
  settleAccountId?: string
  settleAccountName?: string
  channelName?: string
  channelCode?: string
  operatorType?: string
  paymentMethod?: string
}

export function buildTradeCreateOrderPayload(params: BuildTradeCreateOrderPayloadParams): TradeCreateOrderRequest {
  const deviceSource: TradeDeviceSource = params.deviceSource ?? 'WXAPP'
  const orderSource = params.orderSource ?? 'product.detail.page'
  const itemId = Number(params.sku.itemId)
  const skuId = Number(params.sku.skuId)
  const shopId = Number(params.sku.shopId)

  const skuName = params.sku.skuName ?? null
  const mainImage = params.sku.mainImage ?? null
  const skuCode = params.sku.skuCode ?? null
  const categoryId = typeof params.sku.categoryId === 'number' ? params.sku.categoryId : null
  const salePrice = typeof params.sku.salePrice === 'number' ? params.sku.salePrice : null

  const fullUnit = (params.sku.fullUnit && String(params.sku.fullUnit)) || '个'
  const devicesId = params.devicesId
  const settleAccountId = params.settleAccountId ?? ''

  return {
    deviceSource,
    orderSource,
    buyConfig: { lineGrouped: true, multipleCoupon: true },
    memberPointsDeductionInfo: {
      available: false,
      visible: false,
      point: 0,
      chosenIntegral: 0,
      maxExchangeValue: 0,
      minExchangeValue: 100,
      exchangeUnit: 100,
      deductAmount: 0,
      exchangeRatio: 1,
      displayRemark: null,
      extra: null,
      presentIntegral: 0,
    },
    itemName: skuName,
    mobile: null,
    invoice: null,
    addressId: params.addressId,
    couponParams: [
      { activityId: -1, benefitId: null, shopId: 0 },
      { activityId: -1, benefitId: null, shopId },
    ],
    benefitParams: [
      { activityId: -1, benefitId: null, shopId: 0, benefitType: null, amount: null },
    ],
    orderList: [
      {
        activityOrderList: [
          {
            activityMatchedLine: {
              activity: null,
              valid: false,
              benefitId: null,
              benefitUsageInfo: null,
              display: null,
              matchedLineIds: null,
              errorMsg: null,
            },
            activityExist: false,
            orderLineList: null,
            orderLineGroups: [
              {
                orderLineList: [
                  {
                    itemId,
                    skuId,
                    skuCode,
                    bundleId: null,
                    quantity: params.quantity,
                    activityId: null,
                    shopActivityId: null,
                    extraParam: null,
                    promotionTag: null,
                    shopId,
                    lineId: `${itemId}_${shopId}`,
                    categoryId,
                    skuName,
                    attrs: fullUnit ? [{ attrKey: '规格', attrVal: fullUnit }] : [],
                    mainImage,
                    outerSkuCode: null,
                    status: 1,
                    salePrice,
                    preferSalePrice: salePrice,
                    extra: {
                      devicesId,
                      fullUnit,
                      itemType: '1',
                      shopType: null,
                      deliveryTimeType: null,
                      businessCode: 'express',
                      businessName: '快递',
                      businessType: '1',
                      unitQuantity: '1',
                    },
                    summary: null,
                    bizCode: 'express',
                    itemAttributes: params.sku.itemAttributes ?? null,
                  } satisfies TradeCreateOrderLine,
                ],
              },
            ],
          },
        ],
        shop: { id: shopId },
        buyerNote: null,
        extraParam: null,
      },
    ],
    extraParam: { cartLineIds: null },
    extra: {
      orderSource,
      settleAccountName: params.settleAccountName ?? '上海光明随心订电子商务有限公司',
      settleAccountId,
      advisorText:
        '光明健康顾问编号为7-8位数\\n光明健康顾问编号由字母及数字组成\\n光明健康顾问编号内字母为大写字母',
      customerName: null,
      renewOriginAddressId: '',
      devicesId,
      deviceSource,
      customerId: null,
      renewOriginOrderId: '',
      paymentMethod: params.paymentMethod ?? '0',
      channelName: params.channelName ?? '随心订',
      operatorType: params.operatorType ?? '1',
      activityGroupId: null,
      channelCode: params.channelCode ?? 'SXD',
      presentIntegralIsVisible: '1',
      captchaVerifyParam: null,
    },
    delivery: { code: 'express', deliveryTimeParam: {} },
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
