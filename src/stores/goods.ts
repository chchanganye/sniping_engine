import { defineStore } from 'pinia'
import type { GoodsItem } from '@/types/core'
import { apiListDesignPages } from '@/services/api'
import { useLogsStore } from '@/stores/logs'

function normalizeText(value: unknown): string {
  if (value == null) return ''
  return String(value)
}

export const useGoodsStore = defineStore('goods', {
  state: () => ({
    loading: false,
    goods: [] as GoodsItem[],
    selectedGoodsId: undefined as string | undefined,
  }),
  getters: {
    selectedGoods: (state) => state.goods.find((g) => g.id === state.selectedGoodsId),
  },
  actions: {
    setSelectedGoods(id: string) {
      this.selectedGoodsId = id
    },
    async refresh(host = 'm.4008117117.com') {
      this.loading = true
      const logs = useLogsStore()
      try {
        const pages = await apiListDesignPages(host)
        const goods: GoodsItem[] = pages.map((p) => {
          const id = normalizeText(p.id)
          const title = normalizeText(p.name || p.title || p.path || p.id)
          return {
            id,
            title,
            tags: [normalizeText(p.pageType)].filter(Boolean),
            path: normalizeText(p.path),
            pageCategoryId: typeof p.pageCategoryId === 'number' ? p.pageCategoryId : undefined,
            siteId: typeof p.siteId === 'number' ? p.siteId : undefined,
            isIndex: typeof p.isIndex === 'boolean' ? p.isIndex : undefined,
            pageType: normalizeText(p.pageType) || undefined,
            createdAt: normalizeText(p.createdAt),
            updatedAt: normalizeText(p.updatedAt),
            raw: p,
          }
        })
        this.goods = goods
        const stillExists = this.selectedGoodsId && goods.some((g) => g.id === this.selectedGoodsId)
        this.selectedGoodsId = stillExists ? this.selectedGoodsId : goods[0]?.id
      } catch (e) {
        const message = e instanceof Error ? e.message : '获取商品列表失败'
        logs.addLog({ level: 'error', message: `获取商品列表失败：${message}` })
        throw e
      } finally {
        this.loading = false
      }
    },
  },
})
