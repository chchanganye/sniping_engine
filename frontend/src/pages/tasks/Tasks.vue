<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { storeToRefs } from 'pinia'
import { Delete, Refresh } from '@element-plus/icons-vue'
import { useAccountsStore } from '@/stores/accounts'
import { useGoodsStore } from '@/stores/goods'
import { useTasksStore } from '@/stores/tasks'
import type { TaskMode, Task } from '@/types/core'

const accountsStore = useAccountsStore()
const goodsStore = useGoodsStore()
const tasksStore = useTasksStore()

const { accounts } = storeToRefs(accountsStore)
const { targetGoods } = storeToRefs(goodsStore)
const { tasks, engineRunning, engineLoading, loading } = storeToRefs(tasksStore)

onMounted(() => {
  void accountsStore.ensureLoaded()
  void tasksStore.refresh().catch(() => null)
})

const goodsMap = computed(() => {
  const map = new Map<string, any>()
  for (const g of targetGoods.value) map.set(g.id, g)
  return map
})

const modeOptions: Array<{ label: string; value: TaskMode }> = [
  { label: '抢购', value: 'rush' },
  { label: '扫货', value: 'scan' },
]

const enabledCount = computed(() => tasks.value.filter((t) => t.enabled).length)

async function start() {
  if (accounts.value.length === 0) {
    ElMessage.warning('请先添加账号')
    return
  }
  if (enabledCount.value === 0) {
    ElMessage.warning('请先启用至少 1 个目标任务')
    return
  }
  await tasksStore.startEngine()
  ElMessage.success('已启动')
}

async function stop() {
  await tasksStore.stopEngine()
  ElMessage.warning('已停止')
}

async function remove(row: Task) {
  await ElMessageBox.confirm(`确认删除目标任务：${row.goodsTitle}？`, '提示', { type: 'warning' }).catch(() => null)
  await tasksStore.removeTask(row.id)
  ElMessage.success('已删除')
}

function onRushAtChange(row: Task, value: Date | null) {
  const ms = value instanceof Date ? value.getTime() : undefined
  row.rushAtMs = ms
  void tasksStore.updateTask(row.id, { rushAtMs: ms })
}

function statusMeta(row: Task) {
  if (!row.enabled) return { type: 'info' as const, text: '未监控' }
  switch (row.status) {
    case 'success':
      return { type: 'success' as const, text: '已完成' }
    case 'scheduled':
      return { type: 'warning' as const, text: '等待中' }
    case 'failed':
      return { type: 'danger' as const, text: '异常' }
    case 'stopped':
      return { type: 'info' as const, text: '已停止' }
    case 'running':
      return { type: 'primary' as const, text: row.mode === 'scan' ? '监控中' : '抢购中' }
    default:
      return { type: 'info' as const, text: engineRunning.value ? '执行中' : '未运行' }
  }
}
</script>

<template>
  <div class="page">
    <el-card shadow="never" header="抢购工作台">
      <div class="toolbar">
        <div style="color: #606266">
          当前目标任务：<b>{{ tasks.length }}</b>，启用：<b>{{ enabledCount }}</b>，引擎状态：
          <el-tag :type="engineRunning ? 'success' : 'info'" size="small" effect="light">
            {{ engineRunning ? '运行中' : '未运行' }}
          </el-tag>
        </div>
        <el-space :size="8" wrap>
          <el-button :loading="loading" :icon="Refresh" @click="tasksStore.refresh()">刷新</el-button>
          <el-button type="success" :disabled="engineRunning" :loading="engineLoading" @click="start">开始执行</el-button>
          <el-button type="warning" :disabled="!engineRunning" :loading="engineLoading" @click="stop">停止执行</el-button>
        </el-space>
      </div>

      <el-divider />

      <el-table :data="tasks" row-key="id" size="small" style="width: 100%">
        <el-table-column label="商品" min-width="260">
          <template #default="{ row }">
            <div style="display: flex; align-items: center; gap: 10px; min-width: 0">
              <el-image
                v-if="row.imageUrl || goodsMap.get(String(row.itemId))?.imageUrl"
                :src="row.imageUrl || goodsMap.get(String(row.itemId))?.imageUrl"
                fit="cover"
                style="width: 44px; height: 44px; border-radius: 6px; flex: 0 0 auto"
              />
              <div v-else style="width: 44px; height: 44px; border-radius: 6px; background: #f2f3f5; flex: 0 0 auto" />
              <div style="min-width: 0">
                <div style="font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
                  {{ row.goodsTitle }}
                </div>
              </div>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="启用" width="90">
          <template #default="{ row }">
            <el-switch
              v-model="row.enabled"
              :disabled="engineRunning"
              @change="() => tasksStore.updateTask(row.id, { enabled: row.enabled })"
            />
          </template>
        </el-table-column>

        <el-table-column label="模式" width="120">
          <template #default="{ row }">
            <el-select
              v-model="row.mode"
              size="small"
              style="width: 100%"
              :disabled="engineRunning"
              @change="() => tasksStore.updateTask(row.id, { mode: row.mode })"
            >
              <el-option v-for="opt in modeOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
            </el-select>
          </template>
        </el-table-column>

        <el-table-column label="抢购时间" width="220">
          <template #default="{ row }">
            <el-date-picker
              v-if="row.mode === 'rush'"
              :model-value="row.rushAtMs ? new Date(row.rushAtMs) : null"
              type="datetime"
              placeholder="不填则立即"
              size="small"
              style="width: 100%"
              :disabled="engineRunning"
              @update:model-value="(v: Date | null) => onRushAtChange(row, v)"
            />
            <span v-else style="color: #c0c4cc">-</span>
          </template>
        </el-table-column>

        <el-table-column label="目标数量" width="150">
          <template #default="{ row }">
            <el-input-number
              v-model="row.targetQty"
              :min="1"
              :max="9999"
              size="small"
              controls-position="right"
              style="width: 100%"
              :disabled="engineRunning"
              @change="() => tasksStore.updateTask(row.id, { targetQty: row.targetQty })"
            />
          </template>
        </el-table-column>

        <el-table-column label="单次数量" width="150">
          <template #default="{ row }">
            <el-input-number
              v-model="row.perOrderQty"
              :min="1"
              :max="999"
              size="small"
              controls-position="right"
              style="width: 100%"
              :disabled="engineRunning"
              @change="() => tasksStore.updateTask(row.id, { perOrderQty: row.perOrderQty })"
            />
          </template>
        </el-table-column>

        <el-table-column label="状态" min-width="220">
          <template #default="{ row }">
            <el-space :size="6" wrap>
              <el-tooltip v-if="row.lastError && row.status === 'failed'" :content="row.lastError" placement="top">
                <el-tag :type="statusMeta(row).type" size="small" effect="light">{{ statusMeta(row).text }}</el-tag>
              </el-tooltip>
              <el-tag v-else :type="statusMeta(row).type" size="small" effect="light">{{ statusMeta(row).text }}</el-tag>
              <el-tag type="info" size="small" effect="light">已抢 {{ row.purchasedQty }}/{{ row.targetQty }}</el-tag>
            </el-space>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="80">
          <template #default="{ row }">
            <el-tooltip content="删除" placement="top">
              <el-button circle size="small" type="danger" :icon="Delete" :disabled="engineRunning" @click="remove(row)" />
            </el-tooltip>
          </template>
        </el-table-column>
      </el-table>

      <div v-if="tasks.length === 0" style="padding: 8px 0; color: #909399">暂无目标任务</div>
    </el-card>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
</style>
