<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage, ElMessageBox } from 'element-plus'
import { storeToRefs } from 'pinia'
import dayjs from 'dayjs'
import { CopyDocument, Delete, Edit, Key, Plus, Refresh, SwitchButton } from '@element-plus/icons-vue'
import StatusTag from '@/components/StatusTag.vue'
import type { Account } from '@/types/core'
import { useAccountsStore } from '@/stores/accounts'
import { apiGetCaptcha, apiSendSmsCode } from '@/services/api'

const accountsStore = useAccountsStore()
const { accounts, loading } = storeToRefs(accountsStore)

onMounted(() => {
  void accountsStore.refresh().catch(() => null)
})

// Add/Login (SMS) dialog
const addDialogVisible = ref(false)
const addFormRef = ref<FormInstance>()
const addForm = reactive({
  mobile: '',
  captchaCode: '',
  smsCode: '',
})
const addRules: FormRules = {
  mobile: [
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
  addForm.mobile = ''
  addForm.captchaCode = ''
  addForm.smsCode = ''
  addCaptcha.token = ''
  addCaptcha.imageUrl = ''
  addDialogVisible.value = true
  stopAddSmsTimer()
  await fetchAddCaptcha()
}

async function openLogin(row: Account) {
  addForm.mobile = row.mobile
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
  addForm.mobile = ''
  addForm.captchaCode = ''
  addForm.smsCode = ''
  addCaptcha.token = ''
  addCaptcha.imageUrl = ''
  addSmsSending.value = false
  stopAddSmsTimer()
}

async function sendAddSmsCode() {
  if (addSmsCountdown.value > 0 || addSmsSending.value) return

  const mobile = addForm.mobile.trim()
  if (!/^1\d{10}$/.test(mobile)) {
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
      mobile,
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

async function submitAdd() {
  const ok = await addFormRef.value?.validate().catch(() => false)
  if (!ok) return

  addSubmitting.value = true
  try {
    const existed = accounts.value.some((a) => a.mobile === addForm.mobile.trim())
    await accountsStore.loginBySms({
      mobile: addForm.mobile,
      smsCode: addForm.smsCode,
    })
    ElMessage.success(existed ? '登录成功' : '已新增并登录')
    closeAdd()
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '登录失败')
  } finally {
    addSubmitting.value = false
  }
}

async function remove(row: Account) {
  await ElMessageBox.confirm(`确认删除账号 ${row.mobile}？`, '提示', { type: 'warning' }).catch(() => null)
  await accountsStore.remove(row.id)
  ElMessage.success('已删除')
}

async function logout(row: Account) {
  await accountsStore.logout(row.id).catch((e) => {
    ElMessage.error(e instanceof Error ? e.message : '退出失败')
  })
  ElMessage.success('已退出')
}

function copyCookies(row: Account) {
  if (!row.cookies || row.cookies.length === 0) {
    ElMessage.warning('该账号没有cookie信息')
    return
  }
  
  try {
    // 查找draco_local cookie
    let dracoValue = ''
    for (const cookieEntry of row.cookies) {
      if (cookieEntry.cookies) {
        for (const cookie of cookieEntry.cookies) {
          if (cookie.name === 'draco_local') {
            dracoValue = cookie.value
            break
          }
        }
      }
      if (dracoValue) {
        break
      }
    }
    
    if (!dracoValue) {
      ElMessage.warning('该账号没有draco_local cookie')
      return
    }
    
    // 复制draco_local的value到剪贴板
    navigator.clipboard.writeText(dracoValue)
    ElMessage.success('draco_local cookie值已复制到剪贴板')
  } catch (e) {
    ElMessage.error('复制cookie失败')
  }
}

// Edit (proxy only)
const editDialogVisible = ref(false)
const editFormRef = ref<FormInstance>()
const editForm = reactive({
  id: '',
  mobile: '',
  proxy: '',
})
const editRules: FormRules = {
  mobile: [
    { required: true, message: '请输入手机号', trigger: 'blur' },
    { pattern: /^1\d{10}$/, message: '手机号格式不正确', trigger: 'blur' },
  ],
}

function openEdit(row: Account) {
  editForm.id = row.id
  editForm.mobile = row.mobile
  editForm.proxy = row.proxy ?? ''
  editDialogVisible.value = true
}

async function submitEdit() {
  const ok = await editFormRef.value?.validate().catch(() => false)
  if (!ok) return

  await accountsStore.upsert({
    id: editForm.id,
    mobile: editForm.mobile.trim(),
    proxy: editForm.proxy.trim() || '',
  })
  ElMessage.success('已保存')
  editDialogVisible.value = false
}

function formatTime(value?: string) {
  if (!value) return '-'
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss')
}
</script>

<template>
  <div class="page">
    <el-card shadow="never">
      <template #header>
        <div class="toolbar">
          <div class="title">账号管理</div>
          <el-space :size="8" wrap>
            <el-button :icon="Refresh" :loading="loading" @click="accountsStore.refresh()">刷新</el-button>
            <el-button type="primary" :icon="Plus" @click="openAdd">新增账号</el-button>
          </el-space>
        </div>
      </template>

      <el-divider />

      <el-table :data="accounts" row-key="id" size="small" style="width: 100%">
        <el-table-column label="用户ID" min-width="140" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ row.username || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="mobile" label="手机号" min-width="160" />
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <StatusTag kind="account" :status="row.status" />
          </template>
        </el-table-column>
        <el-table-column label="更新时间" width="180">
          <template #default="{ row }">
            <span>{{ formatTime(row.updatedAt) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="240" fixed="right">
          <template #default="{ row }">
            <el-space :size="8" wrap>
              <el-tooltip content="短信登录" placement="top">
                <el-button
                  circle
                  size="small"
                  type="primary"
                  :icon="Key"
                  :disabled="row.status === 'logging_in' || Boolean(row.token)"
                  @click="openLogin(row)"
                />
              </el-tooltip>
              <el-tooltip content="退出登录" placement="top">
                <el-button
                  circle
                  size="small"
                  :icon="SwitchButton"
                  :disabled="row.status === 'logging_in' || !row.token"
                  @click="logout(row)"
                />
              </el-tooltip>
              <el-tooltip content="复制Cookie" placement="top">
                <el-button
                  circle
                  size="small"
                  :icon="CopyDocument"
                  :disabled="row.status === 'logging_in' || !row.token"
                  @click="copyCookies(row)"
                />
              </el-tooltip>
              <el-tooltip content="编辑代理" placement="top">
                <el-button circle size="small" :icon="Edit" @click="openEdit(row)" />
              </el-tooltip>
              <el-tooltip content="删除账号" placement="top">
                <el-button circle size="small" type="danger" :icon="Delete" @click="remove(row)" />
              </el-tooltip>
            </el-space>
          </template>
        </el-table-column>
      </el-table>

      <div v-if="accounts.length === 0" style="padding: 8px 0; color: #909399">暂无账号</div>
    </el-card>

    <el-dialog v-model="addDialogVisible" title="短信登录" width="520px" destroy-on-close @close="closeAdd">
      <el-form ref="addFormRef" :model="addForm" :rules="addRules" label-width="110px">
        <el-form-item label="手机号" prop="mobile">
          <el-input v-model="addForm.mobile" placeholder="请输入手机号" autocomplete="off" />
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
          <el-button type="primary" :loading="addSubmitting" @click="submitAdd">确认登录</el-button>
        </el-space>
      </template>
    </el-dialog>

    <el-dialog v-model="editDialogVisible" title="编辑账号" width="520px" destroy-on-close>
      <el-form ref="editFormRef" :model="editForm" :rules="editRules" label-width="110px">
        <el-form-item label="手机号" prop="mobile">
          <el-input v-model="editForm.mobile" disabled autocomplete="off" />
        </el-form-item>
        <el-form-item label="独立代理">
          <el-input v-model="editForm.proxy" placeholder="可选，如 http://1.2.3.4:7897" autocomplete="off" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-space :size="8">
          <el-button @click="editDialogVisible = false">取消</el-button>
          <el-button type="primary" @click="submitEdit">保存</el-button>
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
  flex-wrap: wrap;
}

.title {
  font-weight: 600;
  color: #303133;
}

.captcha-row {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
}

.captcha-img {
  width: 120px;
  height: 40px;
  border-radius: 6px;
  overflow: hidden;
  flex: 0 0 auto;
  border: 1px solid #ebeef5;
  display: flex;
  align-items: center;
  justify-content: center;
}

.captcha-img img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.sms-row {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
}
</style>
