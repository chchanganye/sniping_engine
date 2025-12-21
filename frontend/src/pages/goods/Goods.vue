<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { storeToRefs } from 'pinia'
import { Plus } from '@element-plus/icons-vue'
import type { ShippingAddress, ShopCategoryNode } from '@/types/core'
import { useAccountsStore } from '@/stores/accounts'
import { useGoodsStore } from '@/stores/goods'
import { useTasksStore } from '@/stores/tasks'

const accountsStore = useAccountsStore()
const goodsStore = useGoodsStore()
const tasksStore = useTasksStore()

const { accounts } = storeToRefs(accountsStore)
const {
  addresses,
  addressesLoading,
  selectedAddressId,
  longitude,
  latitude,
  categories,
  categoriesLoading,
  selectedCategoryId,
  skuGroups,
  selectedGroupId,
  goods,
  goodsLoading,
} = storeToRefs(goodsStore)

const accountId = ref<string>('')

const accountOptions = computed(() =>
  accounts.value
    .filter((a) => a.token)
    .map((a) => ({ label: `${a.mobile}`, value: a.id })),
)

const currentAccount = computed(() => accounts.value.find((a) => a.id === accountId.value) ?? null)

const addressIdModel = computed<number | undefined>({
  get: () => selectedAddressId.value,
  set: (value) => goodsStore.setSelectedAddressId(typeof value === 'number' ? value : undefined),
})

function addressLabel(a: ShippingAddress) {
  const parts = [a.province, a.city, a.region, a.street, a.detail].filter((v) => typeof v === 'string' && v.trim())
  const base = parts.join('')
  const receiver = a.receiveUserName ? `（${a.receiveUserName}）` : ''
  const mobile = a.mobile || a.phone ? ` ${a.mobile || a.phone}` : ''
  return `${base}${receiver}${mobile}`.trim() || String(a.id)
}

async function refreshAddresses() {
  const token = currentAccount.value?.token
  if (!token) {
    ElMessage.warning('请先在「账号管理」配置 Token')
    return
  }
  try {
    await goodsStore.loadAddresses(token)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '获取收货地址失败')
  }
}

async function refreshCategories() {
  const token = currentAccount.value?.token
  if (!token) return
  try {
    await goodsStore.loadCategories(token)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '获取分类失败')
  }
}

async function loadGoodsByCategory(frontCategoryId: number) {
  const token = currentAccount.value?.token
  if (!token) return
  try {
    await goodsStore.loadGoodsByCategory(token, frontCategoryId, 1, 500)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '获取分类商品失败')
  }
}

async function onTreeNodeClick(node: ShopCategoryNode) {
  goodsStore.setSelectedCategoryId(node.id)
  goodsStore.setSelectedGroupId(undefined)
  await loadGoodsByCategory(node.id)
}

async function addToTargetList(item: any) {
  if (!item?.id) return
  await tasksStore.upsertFromGoods(item).catch(() => null)
  ElMessage.success('已添加到抢购工作台')
}

function formatPrice(value?: number) {
  if (typeof value !== 'number') return '-'
  return `￥${(value / 100).toFixed(2)}`
}

const groupModel = computed<string>({
  get: () => (typeof selectedGroupId.value === 'number' ? String(selectedGroupId.value) : 'all'),
  set: (value) => {
    if (value === 'all') {
      goodsStore.setSelectedGroupId(undefined)
      return
    }
    const id = Number(value)
    goodsStore.setSelectedGroupId(Number.isFinite(id) ? id : undefined)
  },
})

const keyword = ref('')
const page = ref(1)
const pageSize = ref(20)

const groupGoods = computed(() => {
  if (typeof selectedGroupId.value !== 'number') return goods.value
  const key = String(selectedGroupId.value)
  return goods.value.filter((g) => g.categoryId === key)
})

const filteredGoods = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  const list = groupGoods.value
  if (!kw) return list
  return list.filter((g) => {
    const titleHit = (g.title ?? '').toLowerCase().includes(kw)
    const idHit = (g.id ?? '').toLowerCase().includes(kw)
    const catHit = (g.categoryName ?? '').toLowerCase().includes(kw)
    return titleHit || idHit || catHit
  })
})

const total = computed(() => filteredGoods.value.length)

const pageGoods = computed(() => {
  const start = (page.value - 1) * pageSize.value
  const end = start + pageSize.value
  return filteredGoods.value.slice(start, end)
})

watch([keyword, pageSize], () => {
  page.value = 1
})

watch(selectedGroupId, () => {
  page.value = 1
})

watch(accountId, () => {
  void refreshAddresses()
})

watch(selectedAddressId, async (id) => {
  if (!id) return
  await refreshCategories()
})

watch(
  categories,
  async (list) => {
    if (!Array.isArray(list) || list.length === 0) return
    if (selectedCategoryId.value) return
    const first = list[0]
    if (!first) return
    const target = first.childrenList?.[0] ?? first
    goodsStore.setSelectedCategoryId(target.id)
    await loadGoodsByCategory(target.id)
    goodsStore.setSelectedGroupId(undefined)
  },
  { deep: false },
)

onMounted(async () => {
  await accountsStore.ensureLoaded()
  if (accountOptions.value.length > 0 && !accountId.value) {
    const first = accountOptions.value[0]
    if (!first) return
    accountId.value = first.value
  }
})
</script>

<template>
  <div class="page">
    <el-row :gutter="12">
      <el-col :xs="24" :lg="24">
        <el-card shadow="never">
          <template #header>
            <div class="toolbar">
              <el-space :size="10" wrap>
                <el-select v-model="accountId" placeholder="选择已登录账号" style="width: 220px">
                  <el-option v-for="opt in accountOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
                </el-select>
                <el-button :loading="addressesLoading" @click="refreshAddresses" :disabled="!accountId">
                  刷新地址
                </el-button>
                <el-select
                  v-model="addressIdModel"
                  placeholder="选择收货地址"
                  style="width: 520px"
                  :loading="addressesLoading"
                  filterable
                  clearable
                  :disabled="addresses.length === 0"
                >
                  <el-option v-for="a in addresses" :key="a.id" :label="addressLabel(a)" :value="a.id" />
                </el-select>
                <el-tag v-if="longitude != null && latitude != null" type="info" effect="light">
                  经度 {{ longitude }} / 纬度 {{ latitude }}
                </el-tag>
              </el-space>
            </div>
          </template>

          <div v-if="accountOptions.length === 0" style="color: #909399">
            暂无可用账号，请先到「账号管理」添加账号并配置 Token。
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="12" style="margin-top: 12px">
      <el-col :xs="24" :lg="6">
        <el-card shadow="never" header="商品分类">
          <el-tree
            v-loading="categoriesLoading"
            :data="categories"
            node-key="id"
            :props="{ label: 'name', children: 'childrenList' }"
            highlight-current
            :current-node-key="selectedCategoryId"
            @node-click="onTreeNodeClick"
          />
          <div v-if="!categoriesLoading && categories.length === 0" style="padding: 8px 0; color: #909399">
            暂无分类数据
          </div>
        </el-card>
      </el-col>

      <el-col :xs="24" :lg="18">
        <el-card shadow="never">
          <template #header>
            <div class="goods-toolbar">
              <el-space :size="8" wrap>
                <el-input v-model="keyword" placeholder="搜索：名称 / ID / 分类" style="width: 260px" clearable />
              </el-space>
              <div style="color: #909399">共 {{ total }} 条</div>
            </div>
          </template>

          <div v-if="skuGroups.length > 0" style="margin-bottom: 10px">
            <el-radio-group v-model="groupModel" size="small">
              <el-radio-button label="all">全部</el-radio-button>
              <el-radio-button v-for="g in skuGroups" :key="g.categoryId" :label="String(g.categoryId)">
                {{ g.categoryName || g.categoryId }}
              </el-radio-button>
            </el-radio-group>
          </div>

          <el-table v-loading="goodsLoading" :data="pageGoods" row-key="id" style="width: 100%">
            <el-table-column label="图片" width="86">
              <template #default="{ row }">
                <el-image
                  v-if="row.imageUrl"
                  :src="row.imageUrl"
                  fit="cover"
                  style="width: 56px; height: 56px; border-radius: 6px"
                  :preview-src-list="[row.imageUrl]"
                  preview-teleported
                />
                <span v-else style="color: #c0c4cc">-</span>
              </template>
            </el-table-column>
            <el-table-column prop="title" label="名称" min-width="200" show-overflow-tooltip />
            <el-table-column prop="id" label="商品ID" width="190" show-overflow-tooltip />
            <el-table-column label="分类" min-width="140" show-overflow-tooltip>
              <template #default="{ row }">
                <span>{{ row.categoryName || '-' }}</span>
              </template>
            </el-table-column>
            <el-table-column label="价格" width="120">
              <template #default="{ row }">{{ formatPrice(row.price) }}</template>
            </el-table-column>
            <el-table-column label="库存" width="90">
              <template #default="{ row }">{{ typeof row.stock === 'number' ? row.stock : '-' }}</template>
            </el-table-column>
            <el-table-column label="操作" width="90">
              <template #default="{ row }">
                <el-tooltip content="添加到抢购工作台" placement="top">
                  <el-button circle size="small" type="primary" :icon="Plus" @click="addToTargetList(row)" />
                </el-tooltip>
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

          <div v-if="!goodsLoading && skuGroups.length === 0" style="padding: 8px 0; color: #909399">
            暂无商品
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

.goods-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
</style>
