<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { ElIcon } from 'element-plus'
import { CircleCheckFilled, CircleCloseFilled, Loading, WarningFilled } from '@element-plus/icons-vue'
import { useProgressStore, type ProgressEvent } from '@/stores/progress'

const props = defineProps<{ opId: string }>()

const progressStore = useProgressStore()
const { sessions } = storeToRefs(progressStore)

const tick = ref(Date.now())
let timer: number | null = null

function startTicker() {
  if (timer != null) return
  timer = window.setInterval(() => {
    tick.value = Date.now()
  }, 500)
}

function stopTicker() {
  if (timer == null) return
  window.clearInterval(timer)
  timer = null
}

onMounted(() => startTicker())
onUnmounted(() => stopTicker())

const session = computed(() => sessions.value.find((s) => s.opId === props.opId) ?? null)
const lastEvent = computed<ProgressEvent | null>(() => {
  const evs = session.value?.events
  if (!evs || evs.length === 0) return null
  return evs[evs.length - 1] ?? null
})

function normalizeErrorText(raw: string) {
  const s = (raw || '').trim()
  if (!s) return ''
  if (s.includes('verification.code.failed')) return '验证码校验失败'
  if (s.includes('no logged-in accounts')) return '没有可用账号（请先登录/配置 Token）'
  if (s.includes('deviceId is required')) return '缺少设备信息（deviceId）'
  return s
}

function pickOrderId(): string {
  const evs = session.value?.events ?? []
  for (let i = evs.length - 1; i >= 0; i--) {
    const v = evs[i]?.fields?.orderId
    if (v != null && String(v).trim() !== '') return String(v).trim()
  }
  return ''
}

const ui = computed(() => {
  const s = session.value
  const ev = lastEvent.value
  const status = s?.status ?? 'running'

  if (status === 'success') {
    const orderId = pickOrderId()
    return { icon: CircleCheckFilled, text: orderId ? `抢购成功（订单号：${orderId}）` : '抢购成功' }
  }
  if (status === 'error') {
    const reason = normalizeErrorText(ev?.message ?? '') || normalizeErrorText(String(ev?.fields?.error ?? '')) || '请稍后重试'
    return { icon: CircleCloseFilled, text: `抢购失败：${reason}` }
  }
  if (status === 'warning') {
    const reason = (ev?.message ?? '').trim() || '需要处理'
    return { icon: WarningFilled, text: `提示：${reason}` }
  }

  const api = typeof ev?.fields?.api === 'string' ? ev.fields.api.trim() : ''
  const step = (ev?.step ?? '').trim()

  if (step === 'captcha') {
    const startedAt = ev?.time ?? Date.now()
    const attempt = Math.min(9, Math.max(1, Math.floor((tick.value - startedAt) / 4500) + 1))
    return { icon: Loading, loading: true, text: `正在第 ${attempt} 次滑动验证码…` }
  }
  if (api.includes('/api/trade/buy/render-order')) return { icon: Loading, loading: true, text: '正在发送预下单请求…' }
  if (api.includes('/api/trade/buy/create-order')) return { icon: Loading, loading: true, text: '正在提交订单…' }
  if (step === 'render_order') return { icon: Loading, loading: true, text: '正在预下单（确认可购买与价格）…' }
  if (step === 'create_order') return { icon: Loading, loading: true, text: '正在创建订单…' }

  return { icon: Loading, loading: true, text: '正在抢购中…' }
})
</script>

<template>
  <div style="display: flex; align-items: center; gap: 8px">
    <ElIcon :class="ui.loading ? 'is-loading' : ''" style="flex: none">
      <component :is="ui.icon" />
    </ElIcon>
    <div style="line-height: 18px; word-break: break-word">{{ ui.text }}</div>
  </div>
</template>
