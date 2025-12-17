import { defineStore } from 'pinia'
import type { GoodsItem, ShippingAddress, ShopCategoryNode, StoreSkuCategoryGroup, StoreSkuModel } from '@/types/core'
import { apiFetchShopCategoryTree, apiListShippingAddresses, apiSearchStoreSkuByCategory } from '@/services/api'
import { useLogsStore } from '@/stores/logs'

const ROOT_FRONT_CATEGORY_ID = 4403

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
  const children = level >= 2 ? [] : rawChildren.map(normalizeCategoryNode)
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
    extra: raw?.extra,
    childrenList: children,
  }
}

function normalizeSku(raw: any): StoreSkuModel {
  return {
    id: Number(raw?.id),
    skuId: Number(raw?.skuId ?? raw?.itemId ?? raw?.id),
    itemId: Number(raw?.itemId ?? raw?.skuId ?? raw?.id),
    storeId: typeof raw?.storeId === 'number' ? raw.storeId : normalizeNumber(raw?.storeId),
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

function skuToGoodsItem(sku: StoreSkuModel, group?: StoreSkuCategoryGroup): GoodsItem {
  return {
    id: normalizeText(sku.skuId || sku.itemId || sku.id),
    title: sku.name,
    price: sku.price ?? undefined,
    stock: sku.inStock ?? undefined,
    imageUrl: sku.mainImage ?? undefined,
    categoryId: group ? normalizeText(group.categoryId) : undefined,
    categoryName: group?.categoryName || undefined,
    raw: sku,
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
    addressesLoading: false,
    addresses: [] as ShippingAddress[],
    selectedAddressId: undefined as number | undefined,
    longitude: undefined as number | undefined,
    latitude: undefined as number | undefined,

    categoriesLoading: false,
    categories: [] as ShopCategoryNode[],
    selectedCategoryId: undefined as number | undefined,

    goodsLoading: false,
    activeFrontCategoryId: undefined as number | undefined,
    skuGroups: [] as StoreSkuCategoryGroup[],
    selectedGroupId: undefined as number | undefined,
    goods: [] as GoodsItem[],
    selectedGoodsId: undefined as string | undefined,
  }),
  getters: {
    selectedGoods: (state) => state.goods.find((g) => g.id === state.selectedGoodsId),
    selectedAddress: (state) => state.addresses.find((a) => a.id === state.selectedAddressId),
    locationReady: (state) => typeof state.longitude === 'number' && typeof state.latitude === 'number',
  },
  actions: {
    setSelectedGoods(id: string) {
      this.selectedGoodsId = id
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
      const logs = useLogsStore()
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
      } catch (e) {
        const message = e instanceof Error ? e.message : '获取收货地址失败'
        logs.addLog({ level: 'error', message: `获取收货地址失败：${message}` })
        throw e
      } finally {
        this.addressesLoading = false
      }
    },
    async loadCategories() {
      if (!this.locationReady) throw new Error('缺少经纬度，请先选择收货地址')
      this.categoriesLoading = true
      const logs = useLogsStore()
      try {
        const data = await apiFetchShopCategoryTree({
          frontCategoryId: ROOT_FRONT_CATEGORY_ID,
          longitude: this.longitude as number,
          latitude: this.latitude as number,
          isFinish: true,
        })
        const normalized = Array.isArray(data) ? data.map(normalizeCategoryNode) : []
        this.categories = normalized

        const exists =
          typeof this.selectedCategoryId === 'number' &&
          treeHasCategoryId(normalized, this.selectedCategoryId)
        if (!exists) this.selectedCategoryId = undefined
      } catch (e) {
        const message = e instanceof Error ? e.message : '获取分类失败'
        logs.addLog({ level: 'error', message: `获取分类失败：${message}` })
        throw e
      } finally {
        this.categoriesLoading = false
      }
    },
    async loadGoodsByCategory(frontCategoryId: number, pageNo = 1, pageSize = 500) {
      if (!this.locationReady) throw new Error('缺少经纬度，请先选择收货地址')
      const cid = Number(frontCategoryId)
      if (!Number.isFinite(cid)) throw new Error('分类ID不正确')

      this.goodsLoading = true
      this.activeFrontCategoryId = cid
      const logs = useLogsStore()
      try {
        const groups = await apiSearchStoreSkuByCategory({
          pageNo,
          pageSize,
          frontCategoryId: cid,
          longitude: this.longitude as number,
          latitude: this.latitude as number,
          isFinish: true,
        })
        const normalizedGroups = Array.isArray(groups) ? groups.map(normalizeSkuGroup) : []
        this.skuGroups = normalizedGroups
        this.goods = normalizedGroups.flatMap((g) => g.storeSkuModelList.map((sku) => skuToGoodsItem(sku, g)))

        const groupExists = typeof this.selectedGroupId === 'number' &&
          normalizedGroups.some((g) => g.categoryId === this.selectedGroupId)
        if (!groupExists) this.selectedGroupId = undefined

        const stillExists = this.selectedGoodsId && this.goods.some((g) => g.id === this.selectedGoodsId)
        if (!stillExists) this.selectedGoodsId = undefined
      } catch (e) {
        const message = e instanceof Error ? e.message : '获取分类商品失败'
        logs.addLog({ level: 'error', message: `获取分类商品失败：${message}` })
        throw e
      } finally {
        this.goodsLoading = false
      }
    },
  },
})
