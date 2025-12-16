<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage, ElMessageBox } from 'element-plus'
import dayjs from 'dayjs'
import { storeToRefs } from 'pinia'
import StatusTag from '@/components/StatusTag.vue'
import { useAccountsStore } from '@/stores/accounts'
import { apiGetCaptcha, apiSendSmsCode } from '@/services/api'

const accountsStore = useAccountsStore()
const { accounts } = storeToRefs(accountsStore)

const selectionIds = ref<string[]>([])

const addDialogVisible = ref(false)
const addFormRef = ref<FormInstance>()
const addForm = reactive({
  username: '',
  captchaCode: '',
  smsCode: '',
})
const addRules: FormRules = {
  username: [
    { required: true, message: '请输入手机号', trigger: 'blur' },
    { pattern: /^1\d{10}$/, message: '手机号格式不正确', trigger: 'blur' },
  ],
  captchaCode: [{ required: true, message: '请输入图形验证码', trigger: 'blur' }],
  smsCode: [{ required: true, message: '请输入短信验证码', trigger: 'blur' }],
}
const addCaptcha = reactive({
  token: '',
  imageUrl: '',
})
const addCaptchaLoading = ref(false)
const addSmsCountdown = ref(0)
const addSmsSending = ref(false)
const addSubmitting = ref(false)
let addSmsTimer: number | undefined

const dialogVisible = ref(false)
const editingId = ref<string | null>(null)
const formRef = ref<FormInstance>()
const formModel = reactive({
  nickname: '',
  username: '',
  remark: '',
})

const rules: FormRules = {
  nickname: [{ required: true, message: '请输入昵称', trigger: 'blur' }],
  username: [{ required: true, message: '请输入手机号', trigger: 'blur' }],
}

const loginDialogVisible = ref(false)
const loginAccountId = ref<string | null>(null)
const loginForm = reactive({
  captchaCode: '',
  smsCode: '',
})
const captcha = reactive({
  token: '',
  imageUrl: '',
})
const captchaLoading = ref(false)
const smsCountdown = ref(0)
const smsSending = ref(false)
let smsTimer: number | undefined

const loginAccount = computed(() => {
  if (!loginAccountId.value) return null
  return accounts.value.find((a) => a.id === loginAccountId.value) ?? null
})

function formatTime(value?: string) {
  if (!value) return '-'
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss')
}

function stopAddSmsTimer() {
  if (addSmsTimer) window.clearInterval(addSmsTimer)
  addSmsTimer = undefined
  addSmsCountdown.value = 0
}

async function fetchAddCaptcha() {
  addCaptchaLoading.value = true
  try {
    const data = await apiGetCaptcha()
    addCaptcha.token = data.token
    addCaptcha.imageUrl = data.imageUrl
    addForm.captchaCode = ''
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '获取图形验证码失败')
  } finally {
    addCaptchaLoading.value = false
  }
}

async function openAdd() {
  addForm.username = ''
  addForm.captchaCode = ''
  addForm.smsCode = ''
  addCaptcha.token = ''
  addCaptcha.imageUrl = ''
  addDialogVisible.value = true
  stopAddSmsTimer()
  await fetchAddCaptcha()
}

function closeAdd() {
  addDialogVisible.value = false
  addForm.username = ''
  addForm.captchaCode = ''
  addForm.smsCode = ''
  addCaptcha.token = ''
  addCaptcha.imageUrl = ''
  addSmsSending.value = false
  stopAddSmsTimer()
}

async function sendAddSmsCode() {
  if (addSmsCountdown.value > 0 || addSmsSending.value) return

  const phone = addForm.username.trim()
  if (!/^1\d{10}$/.test(phone)) {
    ElMessage.warning('请输入正确的手机号')
    return
  }
  if (!addCaptcha.token) {
    ElMessage.warning('请先获取图形验证码')
    return
  }
  if (!addForm.captchaCode.trim()) {
    ElMessage.warning('请输入图形验证码')
    return
  }

  addSmsSending.value = true
  try {
    const ok = await apiSendSmsCode({
      mobile: phone,
      captcha: addForm.captchaCode.trim(),
      token: addCaptcha.token,
    })
    if (!ok) throw new Error('发送短信验证码失败')
    ElMessage.success('短信验证码已发送')

    addSmsCountdown.value = 60
    addSmsTimer = window.setInterval(() => {
      addSmsCountdown.value -= 1
      if (addSmsCountdown.value <= 0) stopAddSmsTimer()
    }, 1000)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '发送短信验证码失败')
  } finally {
    addSmsSending.value = false
  }
}

function nicknameFromPhone(phone: string) {
  const p = phone.trim()
  const tail = p.slice(-4)
  return `账号${tail || p}`
}

async function submitAdd() {
  const ok = await addFormRef.value?.validate().catch(() => false)
  if (!ok) return
  if (!addCaptcha.token) {
    ElMessage.warning('请先获取图形验证码')
    return
  }

  const phone = addForm.username.trim()
  if (accounts.value.some((a) => a.username === phone)) {
    ElMessage.warning('该手机号已存在')
    return
  }

  addSubmitting.value = true
  try {
    const created = accountsStore.addAccount({
      nickname: nicknameFromPhone(phone),
      username: phone,
      remark: '',
    })
    const result = await accountsStore.login(created.id, { smsCode: addForm.smsCode.trim() })
    if (result.ok) {
      ElMessage.success('已新增并登录')
      closeAdd()
    } else {
      accountsStore.removeAccount(created.id)
      ElMessage.error(result.message || '登录失败')
    }
  } finally {
    addSubmitting.value = false
  }
}

function openEdit(id: string) {
  const target = accounts.value.find((a) => a.id === id)
  if (!target) return
  editingId.value = id
  formModel.nickname = target.nickname
  formModel.username = target.username
  formModel.remark = target.remark ?? ''
  dialogVisible.value = true
}

async function submitEdit() {
  const ok = await formRef.value?.validate().catch(() => false)
  if (!ok) return
  if (!editingId.value) return

  accountsStore.updateAccount(editingId.value, {
    nickname: formModel.nickname,
    username: formModel.username,
    remark: formModel.remark,
  })
  ElMessage.success('已更新账号')
  dialogVisible.value = false
}

async function removeAccount(id: string) {
  const target = accounts.value.find((a) => a.id === id)
  if (!target) return
  await ElMessageBox.confirm(`确认删除账号「${target.nickname}」？`, '提示', { type: 'warning' }).catch(() => null)
  accountsStore.removeAccount(id)
  ElMessage.success('已删除')
}

function onSelectionChange(rows: Array<{ id: string }>) {
  selectionIds.value = rows.map((r) => r.id)
}

function batchStart() {
  if (selectionIds.value.length === 0) {
    ElMessage.warning('请先选择账号')
    return
  }
  for (const id of selectionIds.value) accountsStore.start(id)
}

function batchStop() {
  if (selectionIds.value.length === 0) {
    ElMessage.warning('请先选择账号')
    return
  }
  for (const id of selectionIds.value) accountsStore.stop(id)
}

function stopSmsTimer() {
  if (smsTimer) window.clearInterval(smsTimer)
  smsTimer = undefined
  smsCountdown.value = 0
}

async function fetchCaptcha() {
  captchaLoading.value = true
  try {
    const data = await apiGetCaptcha()
    captcha.token = data.token
    captcha.imageUrl = data.imageUrl
    loginForm.captchaCode = ''
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '获取图形验证码失败')
  } finally {
    captchaLoading.value = false
  }
}

async function openLogin(id: string) {
  loginAccountId.value = id
  loginForm.captchaCode = ''
  loginForm.smsCode = ''
  loginDialogVisible.value = true
  stopSmsTimer()
  await fetchCaptcha()
}

function closeLogin() {
  loginDialogVisible.value = false
  loginAccountId.value = null
  loginForm.captchaCode = ''
  loginForm.smsCode = ''
  captcha.token = ''
  captcha.imageUrl = ''
  smsSending.value = false
  stopSmsTimer()
}

async function sendSmsCode() {
  if (!loginAccount.value) return
  if (smsCountdown.value > 0 || smsSending.value) return

  const phone = loginAccount.value.username.trim()
  if (!/^1\d{10}$/.test(phone)) {
    ElMessage.warning('手机号格式不正确')
    return
  }
  if (!captcha.token) {
    ElMessage.warning('请先获取图形验证码')
    return
  }
  if (!loginForm.captchaCode.trim()) {
    ElMessage.warning('请输入图形验证码')
    return
  }

  smsSending.value = true
  try {
    const ok = await apiSendSmsCode({
      mobile: phone,
      captcha: loginForm.captchaCode.trim(),
      token: captcha.token,
    })
    if (!ok) throw new Error('发送短信验证码失败')
    ElMessage.success('短信验证码已发送')

    smsCountdown.value = 60
    smsTimer = window.setInterval(() => {
      smsCountdown.value -= 1
      if (smsCountdown.value <= 0) stopSmsTimer()
    }, 1000)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '发送短信验证码失败')
  } finally {
    smsSending.value = false
  }
}

async function confirmLogin() {
  if (!loginAccount.value) return
  if (!loginForm.smsCode.trim()) {
    ElMessage.warning('请输入短信验证码')
    return
  }
  const result = await accountsStore.login(loginAccount.value.id, { smsCode: loginForm.smsCode.trim() })
  if (result.ok) {
    ElMessage.success('登录成功')
    closeLogin()
  } else {
    ElMessage.error(result.message || '登录失败')
  }
}
</script>

<template>
  <div class="page">
    <el-card shadow="never">
      <template #header>
        <div class="toolbar">
          <div class="left">
            <el-space :size="8">
              <el-button type="primary" @click="openAdd">新增账号</el-button>
              <el-button type="success" @click="batchStart">批量启动</el-button>
              <el-button type="warning" @click="batchStop">批量停止</el-button>
            </el-space>
          </div>
        </div>
      </template>

      <el-table :data="accounts" row-key="id" @selection-change="onSelectionChange">
        <el-table-column type="selection" width="44" />
        <el-table-column prop="nickname" label="昵称" min-width="140" />
        <el-table-column prop="username" label="手机号" min-width="160" />
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <StatusTag kind="account" :status="row.status" />
          </template>
        </el-table-column>
        <el-table-column label="最近心跳" min-width="170">
          <template #default="{ row }">{{ formatTime(row.lastActiveAt) }}</template>
        </el-table-column>
        <el-table-column prop="remark" label="备注" min-width="160" show-overflow-tooltip />
        <el-table-column label="操作" width="320">
          <template #default="{ row }">
            <el-space :size="8" wrap>
              <el-button size="small" @click="openLogin(row.id)" :disabled="row.status === 'logging_in'">
                登录
              </el-button>
              <el-button size="small" @click="accountsStore.logout(row.id)">退出</el-button>
              <el-button size="small" type="success" @click="accountsStore.start(row.id)">启动</el-button>
              <el-button size="small" type="warning" @click="accountsStore.stop(row.id)">停止</el-button>
              <el-button size="small" @click="openEdit(row.id)">编辑</el-button>
              <el-button size="small" type="danger" @click="removeAccount(row.id)">删除</el-button>
            </el-space>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog
      v-model="addDialogVisible"
      title="新增账号"
      width="520px"
      destroy-on-close
      @close="closeAdd"
    >
      <el-form ref="addFormRef" :model="addForm" :rules="addRules" label-width="110px">
        <el-form-item label="手机号" prop="username">
          <el-input v-model="addForm.username" placeholder="请输入手机号" autocomplete="off" />
        </el-form-item>
        <el-form-item label="图形验证码" prop="captchaCode">
          <div class="captcha-row">
            <el-input
              v-model="addForm.captchaCode"
              placeholder="请输入图形验证码"
              style="flex: 1"
              autocomplete="off"
            />
            <div class="captcha-img">
              <el-skeleton :loading="addCaptchaLoading" animated>
                <template #template>
                  <div style="width: 120px; height: 40px" />
                </template>
                <template #default>
                  <img v-if="addCaptcha.imageUrl" :src="addCaptcha.imageUrl" alt="captcha" />
                  <div v-else style="width: 120px; height: 40px; background: #f2f3f5" />
                </template>
              </el-skeleton>
            </div>
            <el-button :loading="addCaptchaLoading" @click="fetchAddCaptcha">刷新</el-button>
          </div>
        </el-form-item>
        <el-form-item label="短信验证码" prop="smsCode">
          <div class="sms-row">
            <el-input v-model="addForm.smsCode" placeholder="请输入短信验证码" style="flex: 1" autocomplete="off" />
            <el-button :loading="addSmsSending" :disabled="addSmsCountdown > 0 || addSmsSending" @click="sendAddSmsCode">
              {{ addSmsCountdown > 0 ? `${addSmsCountdown}s 后重试` : '获取短信验证码' }}
            </el-button>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-space :size="8">
          <el-button @click="closeAdd">取消</el-button>
          <el-button type="primary" :loading="addSubmitting" @click="submitAdd">保存</el-button>
        </el-space>
      </template>
    </el-dialog>

    <el-dialog v-model="dialogVisible" title="编辑账号" width="520px" destroy-on-close>
      <el-form ref="formRef" :model="formModel" :rules="rules" label-width="110px">
        <el-form-item label="昵称" prop="nickname">
          <el-input v-model="formModel.nickname" placeholder="例如：主号/副号" />
        </el-form-item>
        <el-form-item label="手机号" prop="username">
          <el-input v-model="formModel.username" placeholder="用于登录的手机号" />
        </el-form-item>
        <el-form-item label="备注" prop="remark">
          <el-input v-model="formModel.remark" type="textarea" :rows="2" placeholder="可选" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-space :size="8">
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" @click="submitEdit">保存</el-button>
        </el-space>
      </template>
    </el-dialog>

    <el-dialog
      v-model="loginDialogVisible"
      title="短信登录"
      width="520px"
      destroy-on-close
      @close="closeLogin"
    >
      <el-form label-width="110px">
        <el-form-item label="手机号">
          <el-input :model-value="loginAccount?.username ?? ''" disabled />
        </el-form-item>
        <el-form-item label="图形验证码">
          <div class="captcha-row">
            <el-input
              v-model="loginForm.captchaCode"
              placeholder="请输入图形验证码"
              style="flex: 1"
              autocomplete="off"
            />
            <div class="captcha-img">
              <el-skeleton :loading="captchaLoading" animated>
                <template #template>
                  <div style="width: 120px; height: 40px" />
                </template>
                <template #default>
                  <img v-if="captcha.imageUrl" :src="captcha.imageUrl" alt="captcha" />
                  <div v-else style="width: 120px; height: 40px; background: #f2f3f5" />
                </template>
              </el-skeleton>
            </div>
            <el-button :loading="captchaLoading" @click="fetchCaptcha">刷新</el-button>
          </div>
        </el-form-item>
        <el-form-item label="短信验证码">
          <div class="sms-row">
            <el-input v-model="loginForm.smsCode" placeholder="请输入短信验证码" style="flex: 1" autocomplete="off" />
            <el-button :loading="smsSending" :disabled="smsCountdown > 0 || smsSending" @click="sendSmsCode">
              {{ smsCountdown > 0 ? `${smsCountdown}s 后重试` : '获取短信验证码' }}
            </el-button>
          </div>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-space :size="8">
          <el-button @click="closeLogin">取消</el-button>
          <el-button type="primary" @click="confirmLogin" :disabled="!loginAccount">确认登录</el-button>
        </el-space>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.captcha-row,
.sms-row {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 10px;
}

.captcha-img {
  width: 120px;
  height: 40px;
  border: 1px solid #ebeef5;
  border-radius: 4px;
  overflow: hidden;
  background: #ffffff;
  display: flex;
  align-items: center;
  justify-content: center;
}

.captcha-img img {
  width: 120px;
  height: 40px;
  display: block;
}
</style>
