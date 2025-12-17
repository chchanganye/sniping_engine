<script setup lang="ts">
import { computed } from 'vue'
import type { AccountStatus, TaskStatus } from '@/types/core'

const props = defineProps<{
  kind: 'account' | 'task'
  status: AccountStatus | TaskStatus
}>()

const view = computed(() => {
  if (props.kind === 'account') {
    const map: Record<
      AccountStatus,
      { label: string; type: '' | 'success' | 'warning' | 'danger' | 'info' }
    > = {
      idle: { label: '未登录', type: 'info' },
      logging_in: { label: '登录中', type: 'warning' },
      logged_in: { label: '已登录', type: 'success' },
      running: { label: '运行中', type: 'success' },
      error: { label: '异常', type: 'danger' },
    }
    return map[props.status as AccountStatus]
  }

  const map: Record<
    TaskStatus,
    { label: string; type: '' | 'success' | 'warning' | 'danger' | 'info' }
  > = {
    idle: { label: '未开始', type: 'info' },
    scheduled: { label: '已排队', type: 'warning' },
    running: { label: '运行中', type: 'success' },
    success: { label: '成功', type: 'success' },
    failed: { label: '失败', type: 'danger' },
    stopped: { label: '已停止', type: 'info' },
  }
  return map[props.status as TaskStatus]
})
</script>

<template>
  <el-tag :type="view.type" size="small" effect="light">{{ view.label }}</el-tag>
</template>
