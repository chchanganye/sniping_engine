<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import dayjs from 'dayjs'
import StatusTag from '@/components/StatusTag.vue'
import LogLevelTag from '@/components/LogLevelTag.vue'
import { useAccountsStore } from '@/stores/accounts'
import { useTasksStore } from '@/stores/tasks'
import { useLogsStore } from '@/stores/logs'

const accountsStore = useAccountsStore()
const tasksStore = useTasksStore()
const logsStore = useLogsStore()

const { accounts, summary: accountSummary } = storeToRefs(accountsStore)
const { tasks, summary: taskSummary } = storeToRefs(tasksStore)
const { logs } = storeToRefs(logsStore)

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
      title="当前为 UI 框架/假数据演示：下一步会按目标站点 API 逐步对接登录、商品列表、下单。"
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
          <el-statistic title="已登录" :value="accountSummary.loggedIn" />
        </el-card>
      </el-col>
      <el-col :xs="12" :sm="6">
        <el-card shadow="never">
          <el-statistic title="运行中账号" :value="accountSummary.running" />
        </el-card>
      </el-col>
      <el-col :xs="12" :sm="6">
        <el-card shadow="never">
          <el-statistic title="运行中任务" :value="taskSummary.running" />
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="12" style="margin-top: 12px">
      <el-col :xs="24" :lg="14">
        <el-card shadow="never" header="账号运行情况">
          <el-table :data="accounts" size="small" style="width: 100%">
            <el-table-column prop="nickname" label="昵称" min-width="120" />
            <el-table-column prop="username" label="登录名" min-width="150" />
            <el-table-column label="状态" width="110">
              <template #default="{ row }">
                <StatusTag kind="account" :status="row.status" />
              </template>
            </el-table-column>
            <el-table-column label="最近心跳" min-width="170">
              <template #default="{ row }">
                <span>{{ formatTime(row.lastActiveAt) }}</span>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="240">
              <template #default="{ row }">
                <el-space :size="8">
                  <el-button size="small" @click="accountsStore.login(row.id)" :disabled="row.status === 'logging_in'">
                    登录
                  </el-button>
                  <el-button size="small" type="success" @click="accountsStore.start(row.id)">
                    启动
                  </el-button>
                  <el-button size="small" type="warning" @click="accountsStore.stop(row.id)">停止</el-button>
                </el-space>
              </template>
            </el-table-column>
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
          <el-table :data="recentTasks" size="small" style="width: 100%">
            <el-table-column prop="goodsTitle" label="商品" min-width="220" show-overflow-tooltip />
            <el-table-column label="状态" width="110">
              <template #default="{ row }">
                <StatusTag kind="task" :status="row.status" />
              </template>
            </el-table-column>
            <el-table-column label="账号数" width="90">
              <template #default="{ row }">{{ row.accountIds.length }}</template>
            </el-table-column>
            <el-table-column label="创建时间" width="170">
              <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
            </el-table-column>
            <el-table-column prop="lastMessage" label="最新信息" min-width="200" show-overflow-tooltip />
          </el-table>
          <div v-if="recentTasks.length === 0" style="padding: 8px 0; color: #909399">
            还没有任务，可到「抢购工作台」创建。
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>
