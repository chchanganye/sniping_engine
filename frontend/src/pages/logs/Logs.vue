<script setup lang="ts">
import { computed, onMounted, reactive } from 'vue'
import { ElMessage } from 'element-plus'
import dayjs from 'dayjs'
import { storeToRefs } from 'pinia'
import LogLevelTag from '@/components/LogLevelTag.vue'
import { useAccountsStore } from '@/stores/accounts'
import { useLogsStore } from '@/stores/logs'
import { useTasksStore } from '@/stores/tasks'

const accountsStore = useAccountsStore()
const tasksStore = useTasksStore()
const logsStore = useLogsStore()

const { accounts } = storeToRefs(accountsStore)
const { tasks } = storeToRefs(tasksStore)
const { logs, connected } = storeToRefs(logsStore)

onMounted(() => {
  void accountsStore.ensureLoaded()
  void tasksStore.ensureLoaded()
  logsStore.connect()
})

const filters = reactive({
  accountId: '',
  taskId: '',
  level: '' as '' | 'info' | 'success' | 'warning' | 'error',
  keyword: '',
})

const filteredLogs = computed(() => {
  const kw = filters.keyword.trim().toLowerCase()
  return logs.value.filter((l) => {
    if (filters.accountId && l.accountId !== filters.accountId) return false
    if (filters.taskId && l.taskId !== filters.taskId) return false
    if (filters.level && l.level !== filters.level) return false
    if (kw && !l.message.toLowerCase().includes(kw)) return false
    return true
  })
})

function formatTime(value?: string) {
  if (!value) return '-'
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss')
}

function clearLogs() {
  logsStore.clear()
  ElMessage.success('已清空日志')
}
</script>

<template>
  <div class="page">
    <el-card shadow="never" header="筛选">
      <el-row :gutter="12">
        <el-col :xs="24" :sm="8" :md="6">
          <el-select v-model="filters.accountId" placeholder="按账号筛选" clearable filterable style="width: 100%">
            <el-option v-for="a in accounts" :key="a.id" :label="a.mobile" :value="a.id" />
          </el-select>
        </el-col>
        <el-col :xs="24" :sm="8" :md="6">
          <el-select v-model="filters.taskId" placeholder="按任务筛选" clearable filterable style="width: 100%">
            <el-option v-for="t in tasks" :key="t.id" :label="t.goodsTitle" :value="t.id" />
          </el-select>
        </el-col>
        <el-col :xs="24" :sm="8" :md="5">
          <el-select v-model="filters.level" placeholder="按级别筛选" clearable style="width: 100%">
            <el-option label="信息" value="info" />
            <el-option label="成功" value="success" />
            <el-option label="警告" value="warning" />
            <el-option label="错误" value="error" />
          </el-select>
        </el-col>
        <el-col :xs="24" :sm="12" :md="7">
          <el-input v-model="filters.keyword" placeholder="关键词搜索（message）" clearable />
        </el-col>
      </el-row>

      <div style="margin-top: 10px; display: flex; align-items: center; justify-content: space-between; gap: 12px">
        <el-space :size="8">
          <el-button type="danger" @click="clearLogs">清空日志</el-button>
        <el-tag :type="connected ? 'success' : 'info'" effect="light">
            {{ connected ? 'WS 已连接' : 'WS 未连接' }}
          </el-tag>
        </el-space>
      </div>
    </el-card>

    <el-card shadow="never" header="日志列表" style="margin-top: 12px">
      <el-table :data="filteredLogs" size="small" style="width: 100%" height="520">
        <el-table-column label="级别" width="70">
          <template #default="{ row }">
            <LogLevelTag :level="row.level" />
          </template>
        </el-table-column>
        <el-table-column label="时间" width="170">
          <template #default="{ row }">{{ formatTime(row.at) }}</template>
        </el-table-column>
        <el-table-column label="账号" width="160" show-overflow-tooltip>
          <template #default="{ row }">
            {{ accounts.find((a) => a.id === row.accountId)?.mobile ?? '-' }}
          </template>
        </el-table-column>
        <el-table-column label="任务" width="220" show-overflow-tooltip>
          <template #default="{ row }">
            {{ tasks.find((t) => t.id === row.taskId)?.goodsTitle ?? '-' }}
          </template>
        </el-table-column>
        <el-table-column prop="message" label="内容" min-width="360" show-overflow-tooltip />
      </el-table>

      <div v-if="filteredLogs.length === 0" style="padding: 8px 0; color: #909399">暂无日志</div>
    </el-card>
  </div>
</template>
