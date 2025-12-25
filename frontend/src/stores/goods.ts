import { defineStore } from 'pinia'
import type { GoodsItem, ShippingAddress, ShopCategoryNode, StoreSkuCategoryGroup, StoreSkuModel } from '@/types/core'
import {
  apiFetchShopCategoryTree,
  apiListShippingAddresses,
  apiListShopCategoryByParent,
  apiSearchPointsSkuByCategory,
  apiSearchStoreSkuByCategory,
} from '@/services/target'

const ROOT_FRONT_CATEGORY_ID = 4403
const POINTS_ROOT_FRONT_CATEGORY_ID = 3567
const POINTS_DEFAULT_STORE_ID = 1100182001
const POINTS_DEFAULT_TYPE = '7_8_9_10'

function normalizeText(value: unknown): string {
  if (value == null) return ''
  return String(value)
}

function normalizeNumber(value: unknown): number | undefined {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : undefined
}

function normalizeCategoryNode(raw: any): ShopCategoryNode {
  const level = Number(raw?.level)
  const rawChildren = Array.isArray(raw?.childrenList) ? raw.childrenList : []
  const bindChildren = rawChildren
    .filter((c: any) => c && c.hasBind === true)
    .map((c: any) => ({ id: Number(c?.id), name: normalizeText(c?.name) }))
    .filter((c: any) => Number.isFinite(c.id) && c.id > 0)

  const children = level >= 2 ? [] : rawChildren.map(normalizeCategoryNode)

  const rawExtra = raw?.extra
  const extra =
    rawExtra && typeof rawExtra === 'object' && !Array.isArray(rawExtra)
      ? { ...(rawExtra as any), _bindChildren: bindChildren }
      : { raw: rawExtra, _bindChildren: bindChildren }
  return {
    id: Number(raw?.id),
    pid: Number(raw?.pid),
    level,
    name: normalizeText(raw?.name),
    hasChildren: Boolean(raw?.hasChildren),
    hasBind: typeof raw?.hasBind === 'boolean' ? raw.hasBind : undefined,
    logo: typeof raw?.logo === 'string' ? raw.logo : null,
    index: typeof raw?.index === 'number' ? raw.index : undefined,
    path: typeof raw?.path === 'string' ? raw.path : undefined,
    extra,
    childrenList: children,
  }
}

function normalizeSku(raw: any): StoreSkuModel {
  const skuCode =
    typeof raw?.itemCode === 'string'
      ? raw.itemCode
      : typeof raw?.skuCode === 'string'
        ? raw.skuCode
        : null
  const fullUnit =
    typeof raw?.fullUnit === 'string'
      ? raw.fullUnit
      : typeof raw?.saleUnitName === 'string' && raw.saleUnitName
        ? raw.saleUnitName
        : null
  return {
    id: Number(raw?.id),
    skuId: Number(raw?.skuId ?? raw?.itemId ?? raw?.id),
    itemId: Number(raw?.itemId ?? raw?.skuId ?? raw?.id),
    storeId: typeof raw?.storeId === 'number' ? raw.storeId : normalizeNumber(raw?.storeId),
    sellerId: typeof raw?.shopId === 'number' ? raw.shopId : normalizeNumber(raw?.shopId),
    categoryId: normalizeNumber(raw?.categoryId) ?? null,
    skuCode,
    fullUnit,
    name: normalizeText(raw?.name),
    mainImage: typeof raw?.mainImage === 'string' ? raw.mainImage : null,
    price: normalizeNumber(raw?.price) ?? null,
    originalPrice: normalizeNumber(raw?.originalPrice) ?? null,
    inStock: normalizeNumber(raw?.inStock) ?? null,
    purchaseLimit: normalizeNumber(raw?.purchaseLimit) ?? null,
    maxPurchaseLimit: normalizeNumber(raw?.maxPurchaseLimit) ?? null,
    riskFlag: typeof raw?.riskFlag === 'string' ? raw.riskFlag : null,
  }
}

function normalizeSkuGroup(raw: any): StoreSkuCategoryGroup {
  return {
    categoryId: Number(raw?.categoryId),
    categoryName: normalizeText(raw?.categoryName),
    logo: typeof raw?.logo === 'string' ? raw.logo : null,
    extra: raw?.extra,
    storeSkuModelList: Array.isArray(raw?.storeSkuModelList) ? raw.storeSkuModelList.map(normalizeSku) : [],
  }
}

type GroupHint = { id: number; name?: string }

function buildDisplayGroups(apiGroups: StoreSkuCategoryGroup[], hints?: GroupHint[]): StoreSkuCategoryGroup[] {
  if (!Array.isArray(hints) || hints.length === 0) return apiGroups
  const map = new Map<number, StoreSkuCategoryGroup>()
  for (const g of apiGroups) map.set(g.categoryId, g)
  return hints
    .map((h) => {
      const id = Number(h?.id)
      if (!Number.isFinite(id) || id <= 0) return null
      const fromApi = map.get(id)
      if (fromApi) return fromApi
      return {
        categoryId: id,
        categoryName: normalizeText(h?.name) || String(id),
        logo: null,
        extra: undefined,
        storeSkuModelList: [],
      } as StoreSkuCategoryGroup
    })
    .filter((v): v is StoreSkuCategoryGroup => Boolean(v))
}

function skuToGoodsItem(sku: StoreSkuModel, group?: StoreSkuCategoryGroup): GoodsItem {
  return {
    id: normalizeText(sku.itemId || sku.skuId || sku.id),
    title: sku.name,
    price: sku.price ?? undefined,
    stock: sku.inStock ?? undefined,
    imageUrl: sku.mainImage ?? undefined,
    categoryId: group ? normalizeText(group.categoryId) : undefined,
    categoryName: group?.categoryName || undefined,
    raw: sku,
  }
}

function skuToPointsGoodsItem(sku: StoreSkuModel, group?: StoreSkuCategoryGroup): GoodsItem {
  return {
    ...skuToGoodsItem(sku, group),
    priceUnit: 'points',
  }
}

function treeHasCategoryId(nodes: ShopCategoryNode[], id: number): boolean {
  for (const node of nodes) {
    if (node.id === id) return true
    if (node.childrenList && node.childrenList.length > 0) {
      if (treeHasCategoryId(node.childrenList, id)) return true
    }
  }
  return false
}

export const useGoodsStore = defineStore('goods', {
  state: () => ({
    mode: 'normal' as 'normal' | 'points',
    addressesLoading: false,
    addresses: [] as ShippingAddress[],
    selectedAddressId: undefined as number | undefined,
    longitude: undefined as number | undefined,
    latitude: undefined as number | undefined,

    categoriesLoading: false,
    categories: [] as ShopCategoryNode[],
    selectedCategoryId: undefined as number | undefined,

    goodsLoading: false,
    apiSkuGroups: [] as StoreSkuCategoryGroup[],
    skuGroups: [] as StoreSkuCategoryGroup[],
    selectedGroupId: undefined as number | undefined,
    goods: [] as GoodsItem[],

    pointsStoreId: POINTS_DEFAULT_STORE_ID,
    pointsRootCategoryId: POINTS_ROOT_FRONT_CATEGORY_ID,
    pointsType: POINTS_DEFAULT_TYPE,
  }),
  getters: {
    selectedAddress: (state) => state.addresses.find((a) => a.id === state.selectedAddressId),
    locationReady: (state) => typeof state.longitude === 'number' && typeof state.latitude === 'number',
  },
  actions: {
    setMode(mode: 'normal' | 'points') {
      this.mode = mode === 'points' ? 'points' : 'normal'
      this.categories = []
      this.selectedCategoryId = undefined
      this.apiSkuGroups = []
      this.skuGroups = []
      this.selectedGroupId = undefined
      this.goods = []
    },
    setPointsStoreId(value: number) {
      const v = Number(value)
      if (!Number.isFinite(v) || v <= 0) return
      this.pointsStoreId = Math.floor(v)
    },
    setPointsRootCategoryId(value: number) {
      const v = Number(value)
      if (!Number.isFinite(v) || v <= 0) return
      this.pointsRootCategoryId = Math.floor(v)
    },
    setSelectedGroupId(id: number | undefined) {
      this.selectedGroupId = typeof id === 'number' ? id : undefined
    },
    setSelectedCategoryId(id: number | undefined) {
      this.selectedCategoryId = typeof id === 'number' ? id : undefined
    },
    setSelectedAddressId(id: number | undefined) {
      this.selectedAddressId = typeof id === 'number' ? id : undefined
      const addr = this.addresses.find((a) => a.id === this.selectedAddressId)
      this.longitude = addr ? normalizeNumber(addr.longitude) : undefined
      this.latitude = addr ? normalizeNumber(addr.latitude) : undefined
    },
    async loadAddresses(token: string) {
      this.addressesLoading = true
      try {
        const list = await apiListShippingAddresses(token, { app: 'o2o', isAllCover: 1 })
        this.addresses = Array.isArray(list) ? list : []

        const currentExists = this.selectedAddressId && this.addresses.some((a) => a.id === this.selectedAddressId)
        if (currentExists) {
          this.setSelectedAddressId(this.selectedAddressId)
          return
        }

        const defaultAddress = this.addresses.find((a) => a.isDefault) ?? this.addresses[0]
        this.setSelectedAddressId(defaultAddress?.id)
      } finally {
        this.addressesLoading = false
      }
    },
    async loadCategories(token: string) {
      this.categoriesLoading = true
      try {
        if (this.mode === 'points') {
          const list = await apiListShopCategoryByParent(token, {
            frontCategoryId: this.pointsRootCategoryId,
            storeId: this.pointsStoreId,
            type: this.pointsType,
          })
          const children = Array.isArray(list) ? list.map(normalizeCategoryNode) : []
          const root: ShopCategoryNode = {
            id: this.pointsRootCategoryId,
            pid: 0,
            level: 0,
            name: '积分商品',
            hasChildren: true,
            childrenList: children,
          }
          this.categories = [root]
          this.selectedCategoryId = undefined
        } else {
          if (!this.locationReady) throw new Error('缺少经纬度，请先选择收货地址')
          const data = await apiFetchShopCategoryTree(token, {
            frontCategoryId: ROOT_FRONT_CATEGORY_ID,
            longitude: this.longitude as number,
            latitude: this.latitude as number,
            isFinish: true,
          })
          const normalized = Array.isArray(data) ? data.map(normalizeCategoryNode) : []
          this.categories = normalized

          const exists =
            typeof this.selectedCategoryId === 'number' && treeHasCategoryId(normalized, this.selectedCategoryId)
          if (!exists) this.selectedCategoryId = undefined
        }
      } finally {
        this.categoriesLoading = false
      }
    },
    async loadGoodsByCategory(
      token: string,
      frontCategoryId: number,
      pageNo = 1,
      pageSize = 500,
      groupHints?: GroupHint[],
    ) {
      const cid = Number(frontCategoryId)
      if (!Number.isFinite(cid)) throw new Error('分类ID不正确')

      this.goodsLoading = true
      try {
        if (this.mode === 'points') {
          const groups = await apiSearchPointsSkuByCategory(token, {
            frontCategoryId: cid,
            pageNo,
            pageSize,
            storeIds: this.pointsStoreId,
            promotionRender: false,
          })
          const normalizedGroups = Array.isArray(groups) ? groups.map(normalizeSkuGroup) : []
          this.apiSkuGroups = normalizedGroups
          this.skuGroups = buildDisplayGroups(normalizedGroups, groupHints)
          this.goods = normalizedGroups.flatMap((g) => g.storeSkuModelList.map((sku) => skuToPointsGoodsItem(sku, g)))

          const groupExists =
            typeof this.selectedGroupId === 'number' &&
            this.skuGroups.some((g) => g.categoryId === this.selectedGroupId)
          if (!groupExists) this.selectedGroupId = undefined
        } else {
          if (!this.locationReady) throw new Error('缺少经纬度，请先选择收货地址')
          const groups = await apiSearchStoreSkuByCategory(token, {
            pageNo,
            pageSize,
            frontCategoryId: cid,
            longitude: this.longitude as number,
            latitude: this.latitude as number,
            isFinish: true,
          })
          const normalizedGroups = Array.isArray(groups) ? groups.map(normalizeSkuGroup) : []
          this.apiSkuGroups = normalizedGroups
          this.skuGroups = buildDisplayGroups(normalizedGroups, groupHints)
          this.goods = normalizedGroups.flatMap((g) => g.storeSkuModelList.map((sku) => skuToGoodsItem(sku, g)))

          const groupExists =
            typeof this.selectedGroupId === 'number' &&
            this.skuGroups.some((g) => g.categoryId === this.selectedGroupId)
          if (!groupExists) this.selectedGroupId = undefined
        }
      } finally {
        this.goodsLoading = false
      }
    },
  },
})
