<script setup lang="ts">
import { computed } from 'vue'
import type { LogLevel } from '@/types/core'

const props = defineProps<{ level: LogLevel }>()

const view = computed(() => {
  const map: Record<LogLevel, { label: string; type: '' | 'success' | 'warning' | 'danger' | 'info' }> = {
    info: { label: '信息', type: 'info' },
    success: { label: '成功', type: 'success' },
    warning: { label: '警告', type: 'warning' },
    error: { label: '错误', type: 'danger' },
  }
  return map[props.level]
})
</script>

<template>
  <el-tag :type="view.type" size="small" effect="light">{{ view.label }}</el-tag>
</template>
