<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage, ElMessageBox } from 'element-plus'
import dayjs from 'dayjs'
import { storeToRefs } from 'pinia'
import StatusTag from '@/components/StatusTag.vue'
import { useAccountsStore } from '@/stores/accounts'
import { apiGetCaptcha } from '@/services/api'

const accountsStore = useAccountsStore()
const { accounts } = storeToRefs(accountsStore)

const selectionIds = ref<string[]>([])

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

const dialogTitle = computed(() => (editingId.value ? '编辑账号' : '新增账号'))

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
let smsTimer: number | undefined

const loginAccount = computed(() => {
  if (!loginAccountId.value) return null
  return accounts.value.find((a) => a.id === loginAccountId.value) ?? null
})

function formatTime(value?: string) {
  if (!value) return '-'
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss')
}

function openAdd() {
  editingId.value = null
  formModel.nickname = ''
  formModel.username = ''
  formModel.remark = ''
  dialogVisible.value = true
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

async function submit() {
  const ok = await formRef.value?.validate().catch(() => false)
  if (!ok) return

  if (editingId.value) {
    accountsStore.updateAccount(editingId.value, {
      nickname: formModel.nickname,
      username: formModel.username,
      remark: formModel.remark,
    })
    ElMessage.success('已更新账号')
  } else {
    accountsStore.addAccount({
      nickname: formModel.nickname,
      username: formModel.username,
      remark: formModel.remark,
    })
    ElMessage.success('已新增账号')
  }
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

async function batchLogin() {
  void selectionIds.value
  ElMessage.warning('短信登录不支持批量，请逐个账号登录')
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
  stopSmsTimer()
}

function sendSmsCode() {
  if (!loginAccount.value) return
  if (!captcha.token) {
    ElMessage.warning('请先获取图形验证码')
    return
  }
  if (!loginForm.captchaCode.trim()) {
    ElMessage.warning('请输入图形验证码')
    return
  }
  ElMessage.info('短信发送接口尚未对接：请提供发送短信验证码的 API（请求/响应示例）')

  smsCountdown.value = 60
  smsTimer = window.setInterval(() => {
    smsCountdown.value -= 1
    if (smsCountdown.value <= 0) stopSmsTimer()
  }, 1000)
}

async function confirmLogin() {
  if (!loginAccount.value) return
  if (!loginForm.smsCode.trim()) {
    ElMessage.warning('请输入短信验证码')
    return
  }
  await accountsStore.login(loginAccount.value.id, {
    captchaToken: captcha.token,
    captchaCode: loginForm.captchaCode.trim(),
    smsCode: loginForm.smsCode.trim(),
  })
  ElMessage.success('已登录（mock）')
  closeLogin()
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
              <el-button @click="batchLogin">批量登录</el-button>
              <el-button type="success" @click="batchStart">批量启动</el-button>
              <el-button type="warning" @click="batchStop">批量停止</el-button>
            </el-space>
          </div>
          <div class="right" style="color: #909399">提示：这里先用 mock 登录/运行状态，后续会接入真实 API。</div>
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

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="520px" destroy-on-close>
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
          <el-button type="primary" @click="submit">保存</el-button>
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
            <el-button :disabled="smsCountdown > 0" @click="sendSmsCode">
              {{ smsCountdown > 0 ? `${smsCountdown}s 后重试` : '获取短信验证码' }}
            </el-button>
          </div>
          <div style="margin-top: 6px; color: #909399">
            说明：已联通“获取图形验证码”接口；短信发送/短信登录接口待你提供抓包信息后继续对接。
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
