<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { beGetEmailSettings, beSaveEmailSettings, beTestEmail, type EmailSettings } from '@/services/backend'

const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const formRef = ref<FormInstance>()

const form = reactive<EmailSettings>({
  enabled: false,
  email: '',
  authCode: '',
})

const rules: FormRules = {
  email: [
    {
      validator: (_rule, value, callback) => {
        if (!form.enabled) return callback()
        const v = String(value ?? '').trim()
        if (!v) return callback(new Error('请输入收件邮箱'))
        if (!/^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$/.test(v)) return callback(new Error('邮箱格式不正确'))
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

async function save() {
  const ok = await formRef.value?.validate().catch(() => false)
  if (!ok) return

  saving.value = true
  try {
    const payload: Partial<EmailSettings> = {
      enabled: form.enabled,
      email: form.email.trim(),
    }
    const authCode = (form.authCode || '').trim()
    if (authCode && authCode !== '******') payload.authCode = authCode

    const saved = await beSaveEmailSettings(payload)
    form.enabled = Boolean(saved.enabled)
    form.email = saved.email || ''
    form.authCode = saved.authCode || ''
    if (payload.authCode) form.authCode = '******'

    ElMessage.success('已保存')
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '保存失败')
  } finally {
    saving.value = false
  }
}

async function testEmail() {
  const ok = await formRef.value?.validate().catch(() => false)
  if (!ok) return

  testing.value = true
  try {
    await beTestEmail()
    ElMessage.success('已触发测试邮件（请查收收件箱）')
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '测试失败')
  } finally {
    testing.value = false
  }
}

onMounted(() => {
  void load()
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
  </div>
</template>
