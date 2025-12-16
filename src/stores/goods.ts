import { defineStore } from 'pinia'
import dayjs from 'dayjs'
import type { GoodsItem } from '@/types/core'
import { uid } from '@/utils/id'
import { sleep } from '@/utils/sleep'

function createMockGoods(): GoodsItem[] {
  const now = dayjs()
  return [
    {
      id: uid('goods'),
      title: '车险组合包（示例）',
      price: 9.9,
      stock: 120,
      startAt: now.subtract(1, 'hour').toISOString(),
      endAt: now.add(5, 'hour').toISOString(),
      tags: ['爆款', '限时'],
    },
    {
      id: uid('goods'),
      title: '道路救援年卡（示例）',
      price: 19.9,
      stock: 45,
      startAt: now.toISOString(),
      endAt: now.add(1, 'day').toISOString(),
      tags: ['会员', '福利'],
    },
    {
      id: uid('goods'),
      title: '加油券 50 元（示例）',
      price: 49,
      stock: 6,
      startAt: now.add(30, 'minute').toISOString(),
      endAt: now.add(2, 'hour').toISOString(),
      tags: ['稀缺'],
    },
  ]
}

export const useGoodsStore = defineStore('goods', {
  state: () => {
    const goods = createMockGoods()
    return {
      loading: false,
      goods,
      selectedGoodsId: goods[0]?.id as string | undefined,
    }
  },
  getters: {
    selectedGoods: (state) => state.goods.find((g) => g.id === state.selectedGoodsId),
  },
  actions: {
    setSelectedGoods(id: string) {
      this.selectedGoodsId = id
    },
    async refreshMock() {
      this.loading = true
      await sleep(600)
      this.goods = createMockGoods()
      this.selectedGoodsId = this.goods[0]?.id
      this.loading = false
    },
  },
})
