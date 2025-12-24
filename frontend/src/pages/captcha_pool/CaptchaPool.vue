<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import dayjs from 'dayjs'
import { beCaptchaPoolFill, beCaptchaPoolStatus, type CaptchaPoolItemView, type CaptchaPoolStatus } from '@/services/backend'

const loading = ref(false)
const filling = ref(false)
const status = ref<CaptchaPoolStatus | null>(null)
const nowMs = ref(Date.now())
const addCount = ref(2)

let pollTimer: number | undefined
let clockTimer: number | undefined

const items = computed<CaptchaPoolItemView[]>(() => status.value?.items ?? [])

function formatMs(ms?: number): string {
  if (!ms) return '-'
  return dayjs(ms).format('YYYY-MM-DD HH:mm:ss')
}

function leftSeconds(expiresAtMs: number): number {
  const left = expiresAtMs - nowMs.value
  if (left <= 0) return 0
  return Math.ceil(left / 1000)
}

async function load() {
  loading.value = true
  try {
    status.value = await beCaptchaPoolStatus()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '加载失败')
  } finally {
    loading.value = false
  }
}

async function fill() {
  const count = Math.max(1, Math.floor(Number(addCount.value || 1)))
  filling.value = true
  try {
    const res = await beCaptchaPoolFill(count)
    ElMessage.success(`已补充：${res.added}，失败：${res.failed}`)
    await load()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '补充失败')
  } finally {
    filling.value = false
  }
}

onMounted(() => {
  void load()
  clockTimer = window.setInterval(() => {
    nowMs.value = Date.now()
  }, 250)
  pollTimer = window.setInterval(() => {
    void load()
  }, 1500)
})

onUnmounted(() => {
  if (pollTimer) window.clearInterval(pollTimer)
  if (clockTimer) window.clearInterval(clockTimer)
})
</script>

<template>
  <div class="page">
    <el-card shadow="never" header="验证码池状态">
      <div v-loading="loading" class="summary">
        <el-space :size="10" wrap>
          <el-tag :type="status?.activated ? 'success' : 'info'" effect="light">
            状态：{{ status?.activated ? '维护中' : '未启动' }}
          </el-tag>
          <el-tag type="info" effect="light">
            数量：{{ status?.size ?? 0 }} / {{ status?.desiredSize ?? '-' }}
          </el-tag>
          <el-tag type="info" effect="light">预热：{{ status?.settings?.warmupSeconds ?? 30 }}s</el-tag>
          <el-tag type="info" effect="light">有效期：{{ status?.settings?.itemTtlSeconds ?? 120 }}s</el-tag>
          <el-tag type="info" effect="light">
            预计启动：{{ formatMs(status?.activateAtMs) }}
          </el-tag>
        </el-space>
      </div>
    </el-card>

    <el-card shadow="never" header="手动补充" style="margin-top: 12px">
      <el-form inline>
        <el-form-item label="补充条数">
          <el-input-number v-model="addCount" :min="1" :max="50" :step="1" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="filling" @click="fill">开始补充</el-button>
        </el-form-item>
      </el-form>
      <div style="color: #909399">提示：补充会调用验证码引擎生成 verifyParam，并按“单条有效期”自动过期清理。</div>
    </el-card>

    <el-card shadow="never" header="池内明细" style="margin-top: 12px">
      <el-table :data="items" height="520" style="width: 100%">
        <el-table-column label="#" type="index" width="60" />
        <el-table-column label="ID" prop="id" min-width="220" />
        <el-table-column label="Preview" prop="preview" width="140" />
        <el-table-column label="获取时间" width="190">
          <template #default="{ row }">
            {{ formatMs(row.createdAtMs) }}
          </template>
        </el-table-column>
        <el-table-column label="剩余（秒）" width="120">
          <template #default="{ row }">
            <el-tag :type="leftSeconds(row.expiresAtMs) > 10 ? 'success' : 'warning'" effect="light">
              {{ leftSeconds(row.expiresAtMs) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="过期时间" width="190">
          <template #default="{ row }">
            {{ formatMs(row.expiresAtMs) }}
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<style scoped>
.summary {
  min-height: 36px;
  display: flex;
  align-items: center;
}
</style>

