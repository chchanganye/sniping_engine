<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage, ElMessageBox } from 'element-plus'
import { storeToRefs } from 'pinia'
import { Delete, Edit, Plus, Refresh } from '@element-plus/icons-vue'
import StatusTag from '@/components/StatusTag.vue'
import type { Account } from '@/types/core'
import { useAccountsStore } from '@/stores/accounts'

const accountsStore = useAccountsStore()
const { accounts, loading } = storeToRefs(accountsStore)

onMounted(() => {
  void accountsStore.refresh().catch(() => null)
})

const dialogVisible = ref(false)
const formRef = ref<FormInstance>()
const form = reactive({
  id: '',
  mobile: '',
  token: '',
  proxy: '',
  userAgent: '',
  deviceId: '',
  uuid: '',
})

const rules: FormRules = {
  mobile: [
    { required: true, message: '请输入手机号', trigger: 'blur' },
    { pattern: /^1\\d{10}$/, message: '手机号格式不正确', trigger: 'blur' },
  ],
}

const isEditing = computed(() => Boolean(form.id))

function resetForm() {
  form.id = ''
  form.mobile = ''
  form.token = ''
  form.proxy = ''
  form.userAgent = ''
  form.deviceId = ''
  form.uuid = ''
}

function openAdd() {
  resetForm()
  dialogVisible.value = true
}

function openEdit(row: Account) {
  form.id = row.id
  form.mobile = row.mobile
  form.token = row.token ?? ''
  form.proxy = row.proxy ?? ''
  form.userAgent = row.userAgent ?? ''
  form.deviceId = row.deviceId ?? ''
  form.uuid = row.uuid ?? ''
  dialogVisible.value = true
}

async function submit() {
  const ok = await formRef.value?.validate().catch(() => false)
  if (!ok) return

  await accountsStore.upsert({
    id: form.id || undefined,
    mobile: form.mobile.trim(),
    token: form.token.trim() || undefined,
    proxy: form.proxy.trim() || undefined,
    userAgent: form.userAgent.trim() || undefined,
    deviceId: form.deviceId.trim() || undefined,
    uuid: form.uuid.trim() || undefined,
  })

  ElMessage.success(isEditing.value ? '已更新' : '已新增')
  dialogVisible.value = false
}

async function remove(row: Account) {
  await ElMessageBox.confirm(`确认删除账号 ${row.mobile}？`, '提示', { type: 'warning' }).catch(() => null)
  await accountsStore.remove(row.id)
  ElMessage.success('已删除')
}
</script>

<template>
  <div class="page">
    <el-card shadow="never" header="账号管理">
      <div class="toolbar">
        <div style="color: #909399">
          账号信息已持久化到后端 SQLite；前端只负责配置与监控。
        </div>
        <el-space :size="8" wrap>
          <el-button :icon="Refresh" :loading="loading" @click="accountsStore.refresh()">刷新</el-button>
          <el-button type="primary" :icon="Plus" @click="openAdd">新增账号</el-button>
        </el-space>
      </div>

      <el-divider />

      <el-table :data="accounts" row-key="id" size="small" style="width: 100%">
        <el-table-column prop="mobile" label="手机号" min-width="160" />
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <StatusTag kind="account" :status="row.status" />
          </template>
        </el-table-column>
        <el-table-column label="Token" width="100">
          <template #default="{ row }">
            <el-tag :type="row.token ? 'success' : 'info'" size="small" effect="light">
              {{ row.token ? '已配置' : '未配置' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="proxy" label="独立代理" min-width="200" show-overflow-tooltip />
        <el-table-column prop="updatedAt" label="更新时间" width="200" show-overflow-tooltip />
        <el-table-column label="操作" width="200">
          <template #default="{ row }">
            <el-space :size="8">
              <el-button size="small" :icon="Edit" @click="openEdit(row)">编辑</el-button>
              <el-button size="small" type="danger" plain :icon="Delete" @click="remove(row)">删除</el-button>
            </el-space>
          </template>
        </el-table-column>
      </el-table>

      <div v-if="accounts.length === 0" style="padding: 8px 0; color: #909399">暂无账号</div>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="isEditing ? '编辑账号' : '新增账号'" width="560px" destroy-on-close>
      <el-form ref="formRef" :model="form" :rules="rules" label-width="110px">
        <el-form-item label="手机号" prop="mobile">
          <el-input v-model="form.mobile" placeholder="请输入手机号" autocomplete="off" />
        </el-form-item>
        <el-form-item label="Token">
          <el-input v-model="form.token" placeholder="可选：用于后续真实登录态" autocomplete="off" />
        </el-form-item>
        <el-form-item label="独立代理">
          <el-input v-model="form.proxy" placeholder="可选：如 http://1.2.3.4:7897" autocomplete="off" />
        </el-form-item>
        <el-form-item label="UserAgent">
          <el-input v-model="form.userAgent" placeholder="可选" autocomplete="off" />
        </el-form-item>
        <el-form-item label="DeviceId">
          <el-input v-model="form.deviceId" placeholder="可选" autocomplete="off" />
        </el-form-item>
        <el-form-item label="UUID">
          <el-input v-model="form.uuid" placeholder="可选" autocomplete="off" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-space :size="8">
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" @click="submit">保存</el-button>
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
</style>

