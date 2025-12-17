<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { storeToRefs } from 'pinia'
import StatusTag from '@/components/StatusTag.vue'
import { useAccountsStore } from '@/stores/accounts'
import { useGoodsStore } from '@/stores/goods'
import { useTasksStore } from '@/stores/tasks'
import type { TaskMode } from '@/types/core'

const accountsStore = useAccountsStore()
const goodsStore = useGoodsStore()
const tasksStore = useTasksStore()

const { accounts } = storeToRefs(accountsStore)
const { targetGoods } = storeToRefs(goodsStore)
const { tasks } = storeToRefs(tasksStore)

const loggedInAccounts = computed(() => accounts.value.filter((a) => Boolean(a.token)))
const loggedInCount = computed(() => loggedInAccounts.value.length)

const goodsMap = computed(() => {
  const map = new Map<string, any>()
  for (const g of targetGoods.value) map.set(g.id, g)
  return map
})

const modeOptions: Array<{ label: string; value: TaskMode }> = [
  { label: '抢购', value: 'rush' },
  { label: '扫货', value: 'scan' },
]

function sync() {
  tasksStore.syncFromTargetGoods()
}

function start(goodsId: string) {
  if (loggedInCount.value === 0) {
    ElMessage.warning('请先在「账号管理」登录账号')
    return
  }
  tasksStore.startTask(goodsId)
  ElMessage.success('已启动')
}

function stop(goodsId: string) {
  tasksStore.stopTask(goodsId)
  ElMessage.warning('已停止')
}

function startAll() {
  if (loggedInCount.value === 0) {
    ElMessage.warning('请先在「账号管理」登录账号')
    return
  }
  tasksStore.startAll()
  ElMessage.success('已启动全部任务')
}

function stopAll() {
  tasksStore.stopAll()
  ElMessage.warning('已停止全部任务')
}

async function removeFromTargetList(goodsId: string) {
  await ElMessageBox.confirm('确认从目标清单移除该商品？', '提示', { type: 'warning' }).catch(() => null)
  goodsStore.removeTargetGoods(goodsId)
  ElMessage.success('已移除')
}

onMounted(() => {
  sync()
})

watch(targetGoods, () => sync(), { deep: false })
</script>

<template>
  <div class="page">
    <el-card shadow="never" header="抢购工作台">
      <div style="display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap">
        <div style="color: #606266">
          已登录账号：<b>{{ loggedInCount }}</b>（启动后会轮流对目标商品执行：render-order 校验 → create-order 下单）
        </div>
        <el-space :size="8" wrap>
          <el-button @click="sync">同步目标清单</el-button>
          <el-button type="success" @click="startAll" :disabled="tasks.length === 0">全部开始</el-button>
          <el-button type="warning" @click="stopAll" :disabled="tasks.length === 0">全部停止</el-button>
        </el-space>
      </div>

      <el-divider />

      <el-table :data="tasks" row-key="id" size="small" style="width: 100%">
        <el-table-column label="商品" min-width="260">
          <template #default="{ row }">
            <div style="display: flex; align-items: center; gap: 10px; min-width: 0">
              <el-image
                v-if="goodsMap.get(row.goodsId)?.imageUrl"
                :src="goodsMap.get(row.goodsId)?.imageUrl"
                fit="cover"
                style="width: 44px; height: 44px; border-radius: 6px; flex: 0 0 auto"
              />
              <div style="min-width: 0">
                <div style="font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
                  {{ row.goodsTitle }}
                </div>
                <div style="color: #909399; font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
                  {{ row.goodsId }}
                </div>
              </div>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="模式" width="120">
          <template #default="{ row }">
            <el-select
              v-model="row.mode"
              size="small"
              style="width: 100%"
              :disabled="row.status === 'running' || row.status === 'scheduled'"
            >
              <el-option v-for="opt in modeOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
            </el-select>
          </template>
        </el-table-column>

        <el-table-column label="开始时间" width="220">
          <template #default="{ row }">
            <el-date-picker
              v-model="row.scheduleAt"
              type="datetime"
              placeholder="不填则立即开始"
              value-format="YYYY-MM-DDTHH:mm:ss.SSSZ"
              size="small"
              style="width: 100%"
              :disabled="row.status === 'running' || row.status === 'scheduled'"
            />
          </template>
        </el-table-column>

        <el-table-column label="目标数量" width="120">
          <template #default="{ row }">
            <el-input-number
              v-model="row.quantity"
              :min="1"
              :max="999"
              size="small"
              :disabled="row.status === 'running' || row.status === 'scheduled'"
            />
          </template>
        </el-table-column>

        <el-table-column label="进度" width="120">
          <template #default="{ row }">
            <div>{{ row.successCount }}/{{ row.quantity }}</div>
            <div style="color: #909399; font-size: 12px">失败 {{ row.failCount }}</div>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <StatusTag kind="task" :status="row.status" />
          </template>
        </el-table-column>

        <el-table-column label="最新信息" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ row.lastMessage || '-' }}</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="240">
          <template #default="{ row }">
            <el-space :size="8" wrap>
              <el-button
                size="small"
                type="success"
                @click="start(row.goodsId)"
                :disabled="row.status === 'running' || row.status === 'scheduled'"
              >
                开始
              </el-button>
              <el-button
                size="small"
                type="warning"
                @click="stop(row.goodsId)"
                :disabled="row.status !== 'running' && row.status !== 'scheduled'"
              >
                停止
              </el-button>
              <el-button size="small" type="danger" plain @click="removeFromTargetList(row.goodsId)">移出清单</el-button>
            </el-space>
          </template>
        </el-table-column>
      </el-table>

      <div v-if="tasks.length === 0" style="padding: 8px 0; color: #909399">
        暂无目标商品：请先到「商品列表」把商品加入「目标清单」。
      </div>
    </el-card>
  </div>
</template>
