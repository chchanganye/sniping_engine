<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import dayjs from 'dayjs'
import { storeToRefs } from 'pinia'
import { useGoodsStore } from '@/stores/goods'

const goodsStore = useGoodsStore()
const { goods, selectedGoodsId, selectedGoods, loading } = storeToRefs(goodsStore)

const keyword = ref('')

const filteredGoods = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return goods.value
  return goods.value.filter((g) => g.title.toLowerCase().includes(kw))
})

function formatRange(start?: string, end?: string) {
  if (!start && !end) return '-'
  const s = start ? dayjs(start).format('MM-DD HH:mm') : '-'
  const e = end ? dayjs(end).format('MM-DD HH:mm') : '-'
  return `${s} ~ ${e}`
}

function setTarget(id: string) {
  goodsStore.setSelectedGoods(id)
  ElMessage.success('已设为目标商品')
}
</script>

<template>
  <div class="page">
    <el-row :gutter="12">
      <el-col :xs="24" :lg="16">
        <el-card shadow="never">
          <template #header>
            <div class="toolbar">
              <el-space :size="8">
                <el-input v-model="keyword" placeholder="搜索商品名称" style="width: 260px" clearable />
                <el-button :loading="loading" @click="goodsStore.refreshMock">刷新列表（mock）</el-button>
              </el-space>
              <div style="color: #909399">后续将按目标站点 API 返回的商品列表替换这里的 mock 数据。</div>
            </div>
          </template>

          <el-table :data="filteredGoods" row-key="id" style="width: 100%">
            <el-table-column prop="title" label="商品名称" min-width="240" show-overflow-tooltip />
            <el-table-column label="价格" width="90">
              <template #default="{ row }">￥{{ row.price }}</template>
            </el-table-column>
            <el-table-column prop="stock" label="库存" width="70" />
            <el-table-column label="活动时间" min-width="180">
              <template #default="{ row }">{{ formatRange(row.startAt, row.endAt) }}</template>
            </el-table-column>
            <el-table-column label="标签" min-width="140">
              <template #default="{ row }">
                <el-space :size="6" wrap>
                  <el-tag v-for="t in row.tags ?? []" :key="t" size="small" effect="light">{{ t }}</el-tag>
                  <span v-if="(row.tags ?? []).length === 0" style="color: #909399">-</span>
                </el-space>
              </template>
            </el-table-column>
            <el-table-column label="目标" width="70">
              <template #default="{ row }">
                <el-tag v-if="row.id === selectedGoodsId" type="success" size="small">目标</el-tag>
                <span v-else style="color: #c0c4cc">-</span>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="140">
              <template #default="{ row }">
                <el-button size="small" type="primary" @click="setTarget(row.id)" :disabled="row.id === selectedGoodsId">
                  设为目标
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>

      <el-col :xs="24" :lg="8">
        <el-card shadow="never" header="目标商品">
          <div v-if="!selectedGoods" style="color: #909399">尚未选择目标商品</div>
          <el-descriptions v-else :column="1" size="small" border>
            <el-descriptions-item label="名称">{{ selectedGoods.title }}</el-descriptions-item>
            <el-descriptions-item label="价格">￥{{ selectedGoods.price }}</el-descriptions-item>
            <el-descriptions-item label="库存">{{ selectedGoods.stock }}</el-descriptions-item>
            <el-descriptions-item label="活动">{{ formatRange(selectedGoods.startAt, selectedGoods.endAt) }}</el-descriptions-item>
            <el-descriptions-item label="标签">
              <el-space :size="6" wrap>
                <el-tag v-for="t in selectedGoods.tags ?? []" :key="t" size="small" effect="light">{{ t }}</el-tag>
                <span v-if="(selectedGoods.tags ?? []).length === 0" style="color: #909399">-</span>
              </el-space>
            </el-descriptions-item>
          </el-descriptions>
          <div style="margin-top: 10px; color: #909399">
            创建抢购任务时，可直接使用这里的目标商品。
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
</style>
