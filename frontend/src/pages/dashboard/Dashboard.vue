<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import dayjs from 'dayjs'
import { useRouter } from 'vue-router'
import StatusTag from '@/components/StatusTag.vue'
import LogLevelTag from '@/components/LogLevelTag.vue'
import { useAccountsStore } from '@/stores/accounts'
import { useTasksStore } from '@/stores/tasks'
import { useLogsStore } from '@/stores/logs'

const router = useRouter()

const accountsStore = useAccountsStore()
const tasksStore = useTasksStore()
const logsStore = useLogsStore()

const { accounts, summary: accountSummary } = storeToRefs(accountsStore)
const { tasks, summary: taskSummary, engineRunning } = storeToRefs(tasksStore)
const { logs } = storeToRefs(logsStore)

onMounted(() => {
  void accountsStore.ensureLoaded()
  void tasksStore.ensureLoaded()
  logsStore.connect()
})

const recentLogs = computed(() => logs.value.slice(0, 20))
const recentTasks = computed(() => tasks.value.slice(0, 5))

function formatTime(value?: string) {
  if (!value) return '-'
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss')
}
</script>

<template>
  <div class="page">
    <el-alert
      title="前端只负责配置与监控；任务执行由 Go 后端引擎负责。"
      type="info"
      :closable="false"
      show-icon
    />

    <el-row :gutter="12" style="margin-top: 12px">
      <el-col :xs="12" :sm="6">
        <el-card shadow="never">
          <el-statistic title="账号总数" :value="accountSummary.total" />
        </el-card>
      </el-col>
      <el-col :xs="12" :sm="6">
        <el-card shadow="never">
          <el-statistic title="已配置 Token" :value="accountSummary.loggedIn" />
        </el-card>
      </el-col>
      <el-col :xs="12" :sm="6">
        <el-card shadow="never">
          <el-statistic title="目标任务" :value="taskSummary.total" />
        </el-card>
      </el-col>
      <el-col :xs="12" :sm="6">
        <el-card shadow="never">
          <div style="display: flex; align-items: center; justify-content: space-between">
            <div style="color: #909399; font-size: 12px">引擎状态</div>
            <el-tag :type="engineRunning ? 'success' : 'info'" effect="light">
              {{ engineRunning ? '运行中' : '未运行' }}
            </el-tag>
          </div>
          <div style="margin-top: 8px; font-size: 22px; font-weight: 600">{{ taskSummary.running }}</div>
          <div style="color: #909399; font-size: 12px">运行/排队中的任务数</div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="12" style="margin-top: 12px">
      <el-col :xs="24" :lg="14">
        <el-card shadow="never" header="账号概览">
          <div style="margin-bottom: 10px">
            <el-button size="small" @click="router.push('/accounts')">去配置账号</el-button>
          </div>
          <el-table :data="accounts" size="small" style="width: 100%">
            <el-table-column prop="mobile" label="手机号" min-width="160" />
            <el-table-column label="状态" width="110">
              <template #default="{ row }">
                <StatusTag kind="account" :status="row.status" />
              </template>
            </el-table-column>
            <el-table-column label="Token" width="100">
              <template #default="{ row }">
                <el-tag :type="row.token ? 'success' : 'info'" size="small" effect="light">
                  {{ row.token ? '已配置' : '未配置' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="proxy" label="独立代理" min-width="200" show-overflow-tooltip />
            <el-table-column prop="updatedAt" label="更新时间" width="200" show-overflow-tooltip />
          </el-table>
        </el-card>
      </el-col>

      <el-col :xs="24" :lg="10">
        <el-card shadow="never" header="最新日志（Top 20）">
          <el-table :data="recentLogs" size="small" style="width: 100%" height="320">
            <el-table-column label="级别" width="70">
              <template #default="{ row }">
                <LogLevelTag :level="row.level" />
              </template>
            </el-table-column>
            <el-table-column label="时间" width="160">
              <template #default="{ row }">
                {{ formatTime(row.at) }}
              </template>
            </el-table-column>
            <el-table-column prop="message" label="内容" min-width="240" show-overflow-tooltip />
          </el-table>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="12" style="margin-top: 12px">
      <el-col :xs="24">
        <el-card shadow="never" header="最近任务（Top 5）">
          <div style="margin-bottom: 10px">
            <el-button size="small" @click="router.push('/tasks')">去配置任务</el-button>
          </div>
          <el-table :data="recentTasks" size="small" style="width: 100%">
            <el-table-column prop="goodsTitle" label="商品" min-width="220" show-overflow-tooltip />
            <el-table-column label="状态" width="110">
              <template #default="{ row }">
                <StatusTag kind="task" :status="row.status" />
              </template>
            </el-table-column>
            <el-table-column label="进度" width="120">
              <template #default="{ row }">{{ row.purchasedQty }}/{{ row.targetQty }}</template>
            </el-table-column>
            <el-table-column prop="updatedAt" label="更新时间" width="170" show-overflow-tooltip />
            <el-table-column prop="lastError" label="最新错误" min-width="220" show-overflow-tooltip />
          </el-table>
          <div v-if="recentTasks.length === 0" style="padding: 8px 0; color: #909399">
            还没有任务：可到“抢购工作台”导入目标并配置任务。
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

