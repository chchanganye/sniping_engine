<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Document, Fold, Goods, Monitor, Operation, User, Expand, Setting } from '@element-plus/icons-vue'
import { storeToRefs } from 'pinia'
import { useAccountsStore } from '@/stores/accounts'
import { useTasksStore } from '@/stores/tasks'
import { useLogsStore } from '@/stores/logs'

const isCollapsed = ref(false)
const route = useRoute()

const accountsStore = useAccountsStore()
const tasksStore = useTasksStore()
const logsStore = useLogsStore()

const { summary: accountSummary } = storeToRefs(accountsStore)
const { summary: taskSummary, engineRunning } = storeToRefs(tasksStore)

onMounted(() => {
  void accountsStore.ensureLoaded()
  void tasksStore.ensureLoaded()
  logsStore.connect()
})

const pageTitle = computed(() => route.meta.title ?? '控制台')
const activeMenu = computed(() => route.path)

const menuItems = [
  { path: '/dashboard', title: '监控中心', icon: Monitor },
  { path: '/accounts', title: '账号管理', icon: User },
  { path: '/goods', title: '商品列表', icon: Goods },
  { path: '/tasks', title: '抢购工作台', icon: Operation },
  { path: '/logs', title: '运行日志', icon: Document },
  { path: '/settings', title: '通知设置', icon: Setting },
]
</script>

<template>
  <el-container class="shell">
    <el-aside :width="isCollapsed ? '64px' : '240px'" class="shell-aside">
      <div class="brand">
        <div class="brand-title">
          <span v-if="!isCollapsed">sniping_engine</span>
          <span v-else>SE</span>
        </div>
        <el-button text class="brand-toggle" @click="isCollapsed = !isCollapsed">
          <el-icon>
            <component :is="isCollapsed ? Expand : Fold" />
          </el-icon>
        </el-button>
      </div>

      <el-scrollbar class="menu-scroll">
        <el-menu :collapse="isCollapsed" :default-active="activeMenu" router class="menu">
          <el-menu-item v-for="item in menuItems" :key="item.path" :index="item.path">
            <el-icon><component :is="item.icon" /></el-icon>
            <span>{{ item.title }}</span>
          </el-menu-item>
        </el-menu>
      </el-scrollbar>
    </el-aside>

    <el-container>
      <el-header class="shell-header">
        <div class="header-left">
          <div class="header-title">{{ pageTitle }}</div>
        </div>
        <div class="header-right">
          <el-space :size="12" wrap>
            <el-tag type="info" effect="light">账号：{{ accountSummary.total }}</el-tag>
            <el-tag type="success" effect="light">Token：{{ accountSummary.loggedIn }}</el-tag>
            <el-tag :type="engineRunning ? 'success' : 'info'" effect="light">
              引擎：{{ engineRunning ? '运行中' : '未运行' }}
            </el-tag>
            <el-tag type="info" effect="light">任务：{{ taskSummary.total }}</el-tag>
            <el-tag type="warning" effect="light">运行/排队：{{ taskSummary.running }}</el-tag>
          </el-space>
        </div>
      </el-header>

      <el-main class="shell-main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<style scoped>
.shell {
  height: 100%;
}

.shell-aside {
  background: #ffffff;
  border-right: 1px solid #ebeef5;
  display: flex;
  flex-direction: column;
}

.brand {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 10px 0 14px;
  border-bottom: 1px solid #ebeef5;
}

.brand-title {
  font-weight: 700;
  color: #303133;
  user-select: none;
}

.brand-toggle {
  padding: 6px;
}

.menu-scroll {
  flex: 1;
}

.menu {
  border-right: none;
}

.shell-header {
  height: 56px;
  background: #ffffff;
  border-bottom: 1px solid #ebeef5;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.header-title {
  font-size: 16px;
  font-weight: 600;
}

.shell-main {
  padding: 14px;
}
</style>
