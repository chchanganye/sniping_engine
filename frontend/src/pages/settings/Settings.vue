<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import {
  beGetEmailSettings,
  beGetLimitsSettings,
  beGetCaptchaPoolSettings,
  beGetNotifySettings,
  beSaveEmailSettings,
  beSaveLimitsSettings,
  beSaveCaptchaPoolSettings,
  beSaveNotifySettings,
  beTestEmail,
  type EmailSettings,
  type LimitsSettings,
  type CaptchaPoolSettings,
  type NotifySettings,
} from '@/services/backend'

const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const formRef = ref<FormInstance>()

const limitsLoading = ref(false)
const limitsSaving = ref(false)

const captchaPoolLoading = ref(false)
const captchaPoolSaving = ref(false)

const notifyLoading = ref(false)
const notifySaving = ref(false)

const form = reactive<EmailSettings>({
  enabled: false,
  email: '',
  authCode: '',
})

const limits = reactive<LimitsSettings>({
  maxPerTargetInFlight: 1,
  captchaMaxInFlight: 1,
})

const captchaPool = reactive<CaptchaPoolSettings>({
  warmupSeconds: 30,
  poolSize: 2,
  itemTtlSeconds: 120,
})

const notify = reactive<NotifySettings>({
  rushExpireDisableMinutes: 10,
})

function isValidEmailLike(value: string): boolean {
  const v = String(value ?? '').trim()
  if (!v) return false
  if (/\s/.test(v)) return false
  const at = v.indexOf('@')
  if (at <= 0) return false
  if (at !== v.lastIndexOf('@')) return false
  if (at >= v.length - 1) return false
  return true
}

const rules: FormRules = {
  email: [
    {
      validator: (_rule, value, callback) => {
        if (!form.enabled) return callback()
        const v = String(value ?? '').trim()
        if (!v) return callback(new Error('请输入收件邮箱'))
        if (!isValidEmailLike(v)) return callback(new Error('邮箱格式不正确'))
        return callback()
      },
      trigger: 'blur',
    },
  ],
  authCode: [
    {
      validator: (_rule, value, callback) => {
        if (!form.enabled) return callback()
        const v = String(value ?? '').trim()
        if (!v) return callback(new Error('请输入授权码'))
        return callback()
      },
      trigger: 'blur',
    },
  ],
}

async function load() {
  loading.value = true
  try {
    const data = await beGetEmailSettings()
    form.enabled = Boolean(data.enabled)
    form.email = data.email || ''
    form.authCode = data.authCode || ''
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '加载失败')
  } finally {
    loading.value = false
  }
}

async function loadLimits() {
  limitsLoading.value = true
  try {
    const data = await beGetLimitsSettings()
    limits.maxPerTargetInFlight = Number(data.maxPerTargetInFlight || 1)
    limits.captchaMaxInFlight = Number(data.captchaMaxInFlight || 1)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '加载失败')
  } finally {
    limitsLoading.value = false
  }
}

async function save(silent = false): Promise<boolean> {
  const ok = await formRef.value?.validate().catch(() => false)
  if (!ok) return false

  saving.value = true
  try {
    const payload: Partial<EmailSettings> = {
      enabled: form.enabled,
      email: form.email.trim(),
      authCode: (form.authCode || '').trim(),
    }
    const saved = await beSaveEmailSettings(payload)
    form.enabled = Boolean(saved.enabled)
    form.email = saved.email || ''
    form.authCode = saved.authCode || ''

    if (!silent) ElMessage.success('已保存')
    return true
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '保存失败')
    return false
  } finally {
    saving.value = false
  }
}

async function testEmail() {
  testing.value = true
  try {
    const savedOk = await save(true)
    if (!savedOk) return
    await beTestEmail({ email: form.email.trim(), authCode: (form.authCode || '').trim() })
    ElMessage.success('已触发测试邮件（请查收收件箱）')
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '测试失败')
  } finally {
    testing.value = false
  }
}

async function saveLimits() {
  limitsSaving.value = true
  try {
    const payload: Partial<LimitsSettings> = {
      maxPerTargetInFlight: Math.max(1, Math.floor(Number(limits.maxPerTargetInFlight || 1))),
      captchaMaxInFlight: Math.max(1, Math.floor(Number(limits.captchaMaxInFlight || 1))),
    }
    const saved = await beSaveLimitsSettings(payload)
    limits.maxPerTargetInFlight = Number(saved.maxPerTargetInFlight || 1)
    limits.captchaMaxInFlight = Number(saved.captchaMaxInFlight || 1)
    ElMessage.success('已保存')
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '保存失败')
  } finally {
    limitsSaving.value = false
  }
}

async function loadCaptchaPool() {
  captchaPoolLoading.value = true
  try {
    const data = await beGetCaptchaPoolSettings()
    captchaPool.warmupSeconds = Number(data.warmupSeconds || 30)
    captchaPool.poolSize = Number(data.poolSize || 2)
    captchaPool.itemTtlSeconds = Number(data.itemTtlSeconds || 120)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '加载失败')
  } finally {
    captchaPoolLoading.value = false
  }
}

async function saveCaptchaPool() {
  captchaPoolSaving.value = true
  try {
    const payload: Partial<CaptchaPoolSettings> = {
      warmupSeconds: Math.max(1, Math.floor(Number(captchaPool.warmupSeconds || 30))),
      poolSize: Math.max(1, Math.floor(Number(captchaPool.poolSize || 2))),
      itemTtlSeconds: Math.max(1, Math.floor(Number(captchaPool.itemTtlSeconds || 120))),
    }
    const saved = await beSaveCaptchaPoolSettings(payload)
    captchaPool.warmupSeconds = Number(saved.warmupSeconds || 30)
    captchaPool.poolSize = Number(saved.poolSize || 2)
    captchaPool.itemTtlSeconds = Number(saved.itemTtlSeconds || 120)
    ElMessage.success('已保存')
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '保存失败')
  } finally {
    captchaPoolSaving.value = false
  }
}

async function loadNotify() {
  notifyLoading.value = true
  try {
    const data = await beGetNotifySettings()
    notify.rushExpireDisableMinutes = Number(data.rushExpireDisableMinutes || 10)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '加载失败')
  } finally {
    notifyLoading.value = false
  }
}

async function saveNotify() {
  notifySaving.value = true
  try {
    const payload: Partial<NotifySettings> = {
      rushExpireDisableMinutes: Math.max(1, Math.floor(Number(notify.rushExpireDisableMinutes || 10))),
    }
    const saved = await beSaveNotifySettings(payload)
    notify.rushExpireDisableMinutes = Number(saved.rushExpireDisableMinutes || 10)
    ElMessage.success('已保存')
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '保存失败')
  } finally {
    notifySaving.value = false
  }
}

onMounted(() => {
  void load()
  void loadLimits()
  void loadCaptchaPool()
  void loadNotify()
})
</script>

<template>
  <div class="page">
    <el-card shadow="never" header="邮件通知">
      <el-form
        ref="formRef"
        v-loading="loading"
        :model="form"
        :rules="rules"
        label-width="110px"
        style="max-width: 720px"
      >
        <el-form-item label="开启通知">
          <el-switch v-model="form.enabled" />
        </el-form-item>

        <el-form-item label="收件邮箱" prop="email">
          <el-input v-model="form.email" placeholder="例如 123456@qq.com" autocomplete="off" />
        </el-form-item>

        <el-form-item label="授权码" prop="authCode">
          <el-input
            v-model="form.authCode"
            placeholder="邮箱 SMTP 授权码 / App Password"
            show-password
            autocomplete="off"
          />
        </el-form-item>

        <el-form-item>
          <el-space :size="8">
            <el-button type="primary" :loading="saving" @click="save">保存</el-button>
            <el-button :loading="testing" @click="testEmail">发送测试邮件</el-button>
          </el-space>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" header="并发设置" style="margin-top: 12px">
      <el-form v-loading="limitsLoading" :model="limits" label-width="160px" style="max-width: 720px">
        <el-form-item label="同一任务并发账号数">
          <el-input-number v-model="limits.maxPerTargetInFlight" :min="1" :max="200" :step="1" />
          <div style="margin-left: 10px; color: #909399">
            数值越大越快，但更容易触发风控/占用更多资源
          </div>
        </el-form-item>

        <el-form-item label="验证码并发（无头浏览器）">
          <el-input-number v-model="limits.captchaMaxInFlight" :min="1" :max="50" :step="1" />
          <div style="margin-left: 10px; color: #909399">机器配置不高建议保持 1</div>
        </el-form-item>

        <el-form-item>
          <el-button type="primary" :loading="limitsSaving" @click="saveLimits">保存</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" header="验证码池" style="margin-top: 12px">
      <el-form v-loading="captchaPoolLoading" :model="captchaPool" label-width="160px" style="max-width: 720px">
        <el-form-item label="开抢前预热（秒）">
          <el-input-number v-model="captchaPool.warmupSeconds" :min="1" :max="3600" :step="1" />
          <div style="margin-left: 10px; color: #909399">默认 30：开抢前 30 秒开始维护验证码池</div>
        </el-form-item>

        <el-form-item label="验证码池数量">
          <el-input-number v-model="captchaPool.poolSize" :min="1" :max="200" :step="1" />
          <div style="margin-left: 10px; color: #909399">默认 2：后台会尽量维持池内数量达到该值</div>
        </el-form-item>

        <el-form-item label="单条有效期（秒）">
          <el-input-number v-model="captchaPool.itemTtlSeconds" :min="1" :max="3600" :step="1" />
          <div style="margin-left: 10px; color: #909399">倒计时从“拿到验证码返回值”的时刻开始计算</div>
        </el-form-item>

        <el-form-item>
          <el-button type="primary" :loading="captchaPoolSaving" @click="saveCaptchaPool">保存</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" header="过时自动关闭" style="margin-top: 12px">
      <el-form v-loading="notifyLoading" :model="notify" label-width="160px" style="max-width: 720px">
        <el-form-item label="抢购过时关闭（分钟）">
          <el-input-number v-model="notify.rushExpireDisableMinutes" :min="1" :max="1440" :step="1" />
          <div style="margin-left: 10px; color: #909399">默认 10：超过抢购时间 N 分钟自动关闭该任务监控</div>
        </el-form-item>

        <el-form-item>
          <el-button type="primary" :loading="notifySaving" @click="saveNotify">保存</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>
