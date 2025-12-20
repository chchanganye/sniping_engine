export type AccountStatus = 'idle' | 'logging_in' | 'logged_in' | 'running' | 'error'

export interface Account {
  id: string
  username?: string
  mobile: string
  token?: string
  userAgent?: string
  deviceId?: string
  uuid?: string
  proxy?: string
  status: AccountStatus
  createdAt?: string
  updatedAt?: string
}

export interface CurrentUser {
  id: number
  tenantId: number | null
  username: string | null
  nickname: string | null
  avatar: string | null
  mobile: string | null
  email: string | null
  enabled: boolean
  locked: boolean
  extra?: {
    token?: string
    [key: string]: unknown
  } | null
  createdAt?: number
  updatedAt?: number
  lastLoginAt?: number
  firstLogin?: boolean
  shopId?: number | null
  [key: string]: unknown
}

export interface ShippingAddress {
  id: number
  userId?: number
  receiveUserName?: string | null
  phone?: string | null
  mobile?: string | null
  provinceId?: number | null
  province?: string | null
  cityId?: number | null
  city?: string | null
  regionId?: number | null
  region?: string | null
  streetId?: number | null
  street?: string | null
  detail?: string | null
  isDefault?: boolean
  longitude: number
  latitude: number
  isAllCover?: boolean
  createdAt?: number
  updatedAt?: number
  [key: string]: unknown
}

export interface ShopCategoryNode {
  id: number
  pid: number
  level: number
  name: string
  hasChildren: boolean
  hasBind?: boolean
  logo?: string | null
  index?: number
  path?: string
  childrenList: ShopCategoryNode[]
  extra?: unknown
  [key: string]: unknown
}

export interface StoreSkuModel {
  id: number
  skuId: number
  itemId: number
  storeId?: number
  sellerId?: number
  categoryId?: number | null
  skuCode?: string | null
  fullUnit?: string | null
  name: string
  mainImage?: string | null
  price?: number | null
  originalPrice?: number | null
  inStock?: number | null
  purchaseLimit?: number | null
  maxPurchaseLimit?: number | null
  riskFlag?: string | null
  [key: string]: unknown
}

export interface StoreSkuCategoryGroup {
  categoryId: number
  categoryName: string
  logo?: string | null
  extra?: unknown
  storeSkuModelList: StoreSkuModel[]
  [key: string]: unknown
}

export interface GoodsItem {
  id: string
  title: string
  price?: number
  stock?: number
  imageUrl?: string
  categoryId?: string
  categoryName?: string
  startAt?: string
  endAt?: string
  tags?: string[]
  path?: string
  pageCategoryId?: number
  siteId?: number
  isIndex?: boolean
  pageType?: string
  createdAt?: string
  updatedAt?: string
  raw?: unknown
}

export type TaskMode = 'rush' | 'scan'

export type TaskStatus = 'idle' | 'scheduled' | 'running' | 'success' | 'failed' | 'stopped'

export interface Task {
  id: string
  goodsTitle: string
  imageUrl?: string
  mode: TaskMode
  itemId: number
  skuId: number
  shopId?: number
  targetQty: number
  perOrderQty: number
  rushAtMs?: number
  enabled: boolean
  status: TaskStatus
  purchasedQty: number
  needCaptcha?: boolean
  lastError?: string
  lastAttemptMs?: number
  lastSuccessMs?: number
  createdAt?: string
  updatedAt?: string
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
