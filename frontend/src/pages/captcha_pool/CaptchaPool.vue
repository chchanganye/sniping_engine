<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import dayjs from 'dayjs'
import {
  beCaptchaManualConfig,
  beCaptchaManualSubmit,
  beCaptchaPagesRefresh,
  beCaptchaPagesStatus,
  beCaptchaPagesStop,
  beCaptchaPoolFill,
  beCaptchaPoolStatus,
  type CaptchaManualConfig,
  type CaptchaPageInfo,
  type CaptchaPagesStatus,
  type CaptchaPoolItemView,
  type CaptchaPoolStatus,
} from '@/services/backend'

declare global {
  interface Window {
    AliyunCaptchaConfig?: { region: string; prefix: string }
    initAliyunCaptcha?: (options: any) => void
  }
}

const loading = ref(false)
const filling = ref(false)
const refreshingPages = ref(false)
const stoppingPages = ref(false)
const manualDialogVisible = ref(false)
const manualLoading = ref(false)
const manualSubmitting = ref(false)
const manualStatus = ref('')
const manualBatchCount = ref(1)
const manualTargetCount = ref(1)
const manualCompleted = ref(0)
const manualRunning = ref(false)
const manualConfig = ref<CaptchaManualConfig | null>(null)
const status = ref<CaptchaPoolStatus | null>(null)
const pages = ref<CaptchaPagesStatus | null>(null)
const nowMs = ref(Date.now())
const addCount = ref(2)

let pollTimer: number | undefined
let clockTimer: number | undefined
let captchaScriptPromise: Promise<void> | null = null
let manualCaptchaInstance: { destroy?: () => void } | null = null

const items = computed<CaptchaPoolItemView[]>(() => status.value?.items ?? [])
const pageList = computed<CaptchaPageInfo[]>(() => pages.value?.pages ?? [])
const manualProgressLabel = computed(() => {
  const target = manualRunning.value ? manualTargetCount.value : normalizeManualCount(manualBatchCount.value)
  if (manualRunning.value) {
    return `已完成 ${manualCompleted.value} / ${target}`
  }
  return `目标 ${target} 条`
})

function formatMs(ms?: number): string {
  if (!ms) return '-'
  return dayjs(ms).format('YYYY-MM-DD HH:mm:ss')
}

function pageStateText(s: string): string {
  if (s === 'busy') return '获取中'
  if (s === 'idle') return '待机'
  if (s === 'refreshing') return '刷新中'
  return '未知'
}

function pageStateTagType(s: string): 'success' | 'warning' | 'info' | 'danger' {
  if (s === 'busy') return 'warning'
  if (s === 'idle') return 'success'
  if (s === 'refreshing') return 'info'
  return 'danger'
}

function leftSeconds(expiresAtMs: number): number {
  const left = expiresAtMs - nowMs.value
  if (left <= 0) return 0
  return Math.ceil(left / 1000)
}

function normalizeManualCount(value: number): number {
  const raw = Math.floor(Number(value || 1))
  if (!Number.isFinite(raw) || raw <= 0) return 1
  if (raw > 50) return 50
  return raw
}

async function load() {
  loading.value = true
  try {
    const [pool, pageStatus] = await Promise.all([beCaptchaPoolStatus(), beCaptchaPagesStatus()])
    status.value = pool
    pages.value = pageStatus
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '加载失败')
  } finally {
    loading.value = false
  }
}

async function refreshPagePool() {
  refreshingPages.value = true
  try {
    const res = await beCaptchaPagesRefresh()
    ElMessage.success(`已刷新：${res.refreshed}，重建：${res.recreated}，失败：${res.failed}`)
    await load()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '刷新失败')
  } finally {
    refreshingPages.value = false
  }
}

async function stopAllFetching() {
  stoppingPages.value = true
  try {
    const res = await beCaptchaPagesStop()
    ElMessage.success(`已发送停止指令（当前获取中：${res.busy}）`)
    await load()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '停止失败')
  } finally {
    stoppingPages.value = false
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

async function ensureCaptchaScript(): Promise<void> {
  if (typeof window.initAliyunCaptcha === 'function') return
  if (!captchaScriptPromise) {
    captchaScriptPromise = new Promise<void>((resolve, reject) => {
      const script = document.createElement('script')
      script.src = 'https://o.alicdn.com/captcha-frontend/aliyunCaptcha/AliyunCaptcha.js'
      script.async = true
      script.onload = () => resolve()
      script.onerror = () => reject(new Error('验证码脚本加载失败'))
      document.head.appendChild(script)
    })
  }
  try {
    await captchaScriptPromise
  } catch (e) {
    captchaScriptPromise = null
    throw e
  }
}

function destroyManualCaptcha() {
  if (manualCaptchaInstance && typeof manualCaptchaInstance.destroy === 'function') {
    manualCaptchaInstance.destroy()
  }
  manualCaptchaInstance = null
  const container = document.getElementById('manual-captcha-container')
  if (container) container.innerHTML = ''
}

function triggerManualCaptcha() {
  const btn = document.getElementById('manual-captcha-button') as HTMLButtonElement | null
  if (!btn || btn.disabled) return
  btn.click()
}

async function submitManualCaptcha(verifyParam: string) {
  if (!verifyParam) {
    manualStatus.value = '未获取到验证码结果'
    return
  }
  if (!manualRunning.value) {
    manualTargetCount.value = normalizeManualCount(manualBatchCount.value)
    manualCompleted.value = 0
    manualRunning.value = true
  }
  const nextIndex = manualCompleted.value + 1
  manualSubmitting.value = true
  manualStatus.value = `验证成功，正在提交（${nextIndex} / ${manualTargetCount.value}）...`
  try {
    await beCaptchaManualSubmit(verifyParam)
    manualCompleted.value = nextIndex
    if (manualCompleted.value >= manualTargetCount.value) {
      manualStatus.value = `已完成 ${manualCompleted.value} / ${manualTargetCount.value}`
      ElMessage.success(`人工补充完成：已新增 ${manualCompleted.value} 条`)
      manualDialogVisible.value = false
      manualRunning.value = false
      await load()
      return
    }
    manualStatus.value = `已完成 ${manualCompleted.value} / ${manualTargetCount.value}，准备下一条...`
  } catch (e) {
    const msg = e instanceof Error ? e.message : '提交失败'
    manualStatus.value = `提交失败：${msg}`
    ElMessage.error(msg)
    return
  } finally {
    manualSubmitting.value = false
  }
  if (manualDialogVisible.value) {
    await prepareManualCaptcha(true, `已完成 ${manualCompleted.value} / ${manualTargetCount.value}，请继续验证`)
  }
}

function initManualCaptcha(cfg: CaptchaManualConfig, statusText?: string) {
  destroyManualCaptcha()
  if (typeof window.initAliyunCaptcha !== 'function') {
    manualStatus.value = '验证码脚本加载失败'
    return
  }
  manualStatus.value = statusText || '请点击按钮开始验证'
  window.initAliyunCaptcha({
    SceneId: cfg.sceneId,
    mode: 'popup',
    element: '#manual-captcha-container',
    button: '#manual-captcha-button',
    success: (captchaVerifyParam: string) => {
      void submitManualCaptcha(captchaVerifyParam)
    },
    fail: () => {
      manualStatus.value = '验证失败，请重试'
    },
    getInstance: (instance: { destroy?: () => void }) => {
      manualCaptchaInstance = instance
    },
    rem: 1,
  })
}

async function prepareManualCaptcha(autoOpen: boolean, statusText?: string) {
  if (!manualConfig.value) return
  await nextTick()
  initManualCaptcha(manualConfig.value, statusText)
  if (autoOpen) {
    window.setTimeout(() => {
      triggerManualCaptcha()
    }, 150)
  }
}

function onManualStartClick() {
  if (manualRunning.value) return
  manualTargetCount.value = normalizeManualCount(manualBatchCount.value)
  manualCompleted.value = 0
  manualRunning.value = true
  manualStatus.value = `请完成验证（0 / ${manualTargetCount.value}）`
}

async function fillHuman() {
  manualDialogVisible.value = true
  manualLoading.value = true
  manualSubmitting.value = false
  manualRunning.value = false
  manualCompleted.value = 0
  manualBatchCount.value = normalizeManualCount(addCount.value)
  manualTargetCount.value = normalizeManualCount(manualBatchCount.value)
  manualStatus.value = '加载验证码配置中...'
  try {
    const cfg = await beCaptchaManualConfig()
    manualConfig.value = cfg
    window.AliyunCaptchaConfig = { region: cfg.region, prefix: cfg.prefix }
    await ensureCaptchaScript()
    await prepareManualCaptcha(false)
  } catch (e) {
    const msg = e instanceof Error ? e.message : '加载失败'
    manualStatus.value = msg
    ElMessage.error(msg)
  } finally {
    manualLoading.value = false
  }
}

function onManualDialogClosed() {
  destroyManualCaptcha()
  manualStatus.value = ''
  manualSubmitting.value = false
  manualRunning.value = false
  manualCompleted.value = 0
  manualTargetCount.value = normalizeManualCount(manualBatchCount.value)
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
        <el-form-item>
          <el-button type="warning" plain @click="fillHuman">人工补充</el-button>
        </el-form-item>
      </el-form>
      <div style="color: #909399">提示：补充会调用验证码引擎生成 verifyParam，并按“单条有效期”自动过期清理。</div>
      <div style="color: #909399; margin-top: 6px">人工补充会在当前页面弹出验证码滑块，完成验证后自动入池。</div>
    </el-card>

    <el-card shadow="never" header="验证码页面池" style="margin-top: 12px">
      <div v-loading="loading">
        <el-space :size="10" wrap>
          <el-tag type="info" effect="light">总页数：{{ pages?.total ?? 0 }}</el-tag>
          <el-tag type="success" effect="light">待机：{{ pages?.idle ?? 0 }}</el-tag>
          <el-tag type="warning" effect="light">获取中：{{ pages?.busy ?? 0 }}</el-tag>
          <el-tag type="info" effect="light">刷新中：{{ pages?.refreshing ?? 0 }}</el-tag>
          <el-tag type="info" effect="light">Pool：{{ pages?.pagePool ?? 0 }}</el-tag>
          <el-button size="small" :loading="refreshingPages" @click="refreshPagePool">刷新全部页面</el-button>
          <el-button size="small" type="danger" plain :loading="stoppingPages" @click="stopAllFetching">停止全部获取</el-button>
        </el-space>

        <el-table :data="pageList" height="320" style="width: 100%; margin-top: 10px">
          <el-table-column label="#" type="index" width="60" />
          <el-table-column label="PageID" prop="id" min-width="140" />
          <el-table-column label="状态" width="110">
            <template #default="{ row }">
              <el-tag :type="pageStateTagType(row.state)" effect="light">{{ pageStateText(row.state) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="创建时间" width="190">
            <template #default="{ row }">
              {{ formatMs(row.createdAtMs) }}
            </template>
          </el-table-column>
          <el-table-column label="上次打开" width="190">
            <template #default="{ row }">
              {{ formatMs(row.lastOpenedAtMs) }}
            </template>
          </el-table-column>
          <el-table-column label="上次使用" width="190">
            <template #default="{ row }">
              {{ formatMs(row.lastUsedAtMs) }}
            </template>
          </el-table-column>
          <el-table-column label="错误" min-width="240">
            <template #default="{ row }">
              <span style="color: #909399">{{ row.lastError || '-' }}</span>
            </template>
          </el-table-column>
        </el-table>
      </div>
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

    <el-dialog v-model="manualDialogVisible" title="人工补充验证码" width="420px" class="manual-dialog" @closed="onManualDialogClosed">
      <div v-loading="manualLoading">
        <div class="manual-toolbar">
          <span class="manual-label">补充条数</span>
          <el-input-number
            v-model="manualBatchCount"
            :min="1"
            :max="50"
            :step="1"
            size="small"
            :disabled="manualRunning || manualLoading || manualSubmitting"
          />
          <span class="manual-progress">{{ manualProgressLabel }}</span>
        </div>
        <div id="manual-captcha-container" class="manual-captcha" />
        <el-button
          id="manual-captcha-button"
          type="primary"
          class="manual-button"
          :disabled="manualLoading || manualSubmitting"
          @click="onManualStartClick"
        >
          安全验证
        </el-button>
        <div class="manual-status">
          {{ manualStatus || ' ' }}
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.summary {
  min-height: 36px;
  display: flex;
  align-items: center;
}
.manual-dialog :deep(.el-dialog__body) {
  padding-top: 12px;
}
.manual-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
}
.manual-label {
  color: #606266;
  font-size: 12px;
}
.manual-progress {
  margin-left: auto;
  color: #909399;
  font-size: 12px;
}
.manual-captcha {
  min-height: 80px;
}
.manual-button {
  width: 100%;
  margin-top: 8px;
}
.manual-status {
  color: #909399;
  margin-top: 6px;
  min-height: 18px;
}
</style>
