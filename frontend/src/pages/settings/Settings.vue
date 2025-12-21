<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import {
  beGetEmailSettings,
  beGetLimitsSettings,
  beSaveEmailSettings,
  beSaveLimitsSettings,
  beTestEmail,
  type EmailSettings,
  type LimitsSettings,
} from '@/services/backend'

const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const formRef = ref<FormInstance>()

const limitsLoading = ref(false)
const limitsSaving = ref(false)

const form = reactive<EmailSettings>({
  enabled: false,
  email: '',
  authCode: '',
})

const limits = reactive<LimitsSettings>({
  maxPerTargetInFlight: 1,
  captchaMaxInFlight: 1,
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

onMounted(() => {
  void load()
  void loadLimits()
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
  </div>
</template>
