<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import dayjs from 'dayjs'
import { storeToRefs } from 'pinia'
import { useGoodsStore } from '@/stores/goods'

const goodsStore = useGoodsStore()
const { goods, selectedGoodsId, selectedGoods, loading } = storeToRefs(goodsStore)

const keyword = ref('')
const page = ref(1)
const pageSize = ref(20)

const filteredGoods = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return goods.value
  return goods.value.filter((g) => {
    const titleHit = (g.title ?? '').toLowerCase().includes(kw)
    const pathHit = (g.path ?? '').toLowerCase().includes(kw)
    return titleHit || pathHit
  })
})

const total = computed(() => filteredGoods.value.length)

const pageGoods = computed(() => {
  const start = (page.value - 1) * pageSize.value
  const end = start + pageSize.value
  return filteredGoods.value.slice(start, end)
})

function formatTime(value?: string) {
  if (!value) return '-'
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss')
}

function setTarget(id: string) {
  goodsStore.setSelectedGoods(id)
  ElMessage.success('已设为目标商品')
}

async function refresh() {
  try {
    await goodsStore.refresh('m.4008117117.com')
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '获取商品列表失败')
  }
}

onMounted(() => {
  void refresh()
})

watch(keyword, () => {
  page.value = 1
})
</script>

<template>
  <div class="page">
    <el-row :gutter="12">
      <el-col :xs="24" :lg="16">
        <el-card shadow="never">
          <template #header>
            <div class="toolbar">
              <el-space :size="8">
                <el-input v-model="keyword" placeholder="搜索名称或路径" style="width: 260px" clearable />
                <el-button :loading="loading" @click="refresh">刷新</el-button>
              </el-space>
              <div style="color: #909399">共 {{ total }} 条</div>
            </div>
          </template>

          <el-table :data="pageGoods" row-key="id" style="width: 100%">
            <el-table-column prop="id" label="ID" width="90" />
            <el-table-column prop="title" label="名称" min-width="240" show-overflow-tooltip />
            <el-table-column prop="path" label="路径" min-width="260" show-overflow-tooltip />
            <el-table-column prop="pageCategoryId" label="分类ID" width="90" />
            <el-table-column label="类型" width="110">
              <template #default="{ row }">
                <el-tag v-if="row.pageType" size="small" effect="light">{{ row.pageType }}</el-tag>
                <span v-else style="color: #909399">-</span>
              </template>
            </el-table-column>
            <el-table-column label="更新时间" width="170">
              <template #default="{ row }">{{ formatTime(row.updatedAt) }}</template>
            </el-table-column>
            <el-table-column label="创建时间" width="170">
              <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
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

          <div style="display: flex; justify-content: flex-end; margin-top: 12px">
            <el-pagination
              v-model:current-page="page"
              v-model:page-size="pageSize"
              :total="total"
              :page-sizes="[10, 20, 50, 100]"
              layout="total, sizes, prev, pager, next, jumper"
              background
            />
          </div>
        </el-card>
      </el-col>

      <el-col :xs="24" :lg="8">
        <el-card shadow="never" header="目标商品">
          <div v-if="!selectedGoods" style="color: #909399">尚未选择目标商品</div>
          <el-descriptions v-else :column="1" size="small" border>
            <el-descriptions-item label="ID">{{ selectedGoods.id }}</el-descriptions-item>
            <el-descriptions-item label="名称">{{ selectedGoods.title }}</el-descriptions-item>
            <el-descriptions-item label="路径">{{ selectedGoods.path || '-' }}</el-descriptions-item>
            <el-descriptions-item label="分类ID">{{ selectedGoods.pageCategoryId ?? '-' }}</el-descriptions-item>
            <el-descriptions-item label="类型">{{ selectedGoods.pageType || '-' }}</el-descriptions-item>
            <el-descriptions-item label="更新时间">{{ formatTime(selectedGoods.updatedAt) }}</el-descriptions-item>
            <el-descriptions-item label="创建时间">{{ formatTime(selectedGoods.createdAt) }}</el-descriptions-item>
          </el-descriptions>
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
