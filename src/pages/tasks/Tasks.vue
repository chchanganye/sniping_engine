<script setup lang="ts">
import { computed, reactive } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import dayjs from 'dayjs'
import { storeToRefs } from 'pinia'
import StatusTag from '@/components/StatusTag.vue'
import { useAccountsStore } from '@/stores/accounts'
import { useGoodsStore } from '@/stores/goods'
import { useTasksStore } from '@/stores/tasks'

const accountsStore = useAccountsStore()
const goodsStore = useGoodsStore()
const tasksStore = useTasksStore()

const { accounts } = storeToRefs(accountsStore)
const { goods, selectedGoodsId } = storeToRefs(goodsStore)
const { tasks } = storeToRefs(tasksStore)

const goodsOptions = computed(() => goods.value.map((g) => ({ label: g.title, value: g.id })))
const accountOptions = computed(() =>
  accounts.value.map((a) => ({ label: `${a.nickname}（${a.username}）`, value: a.id })),
)

const formModel = reactive({
  goodsId: selectedGoodsId.value ?? '',
  accountIds: [] as string[],
  quantity: 1,
  scheduleAt: '' as string | '',
})

function formatTime(value?: string) {
  if (!value) return '-'
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss')
}

function resolveGoodsTitle(id: string) {
  return goods.value.find((g) => g.id === id)?.title ?? id
}

function create(startNow: boolean) {
  if (!formModel.goodsId) {
    ElMessage.warning('请选择商品')
    return
  }
  if (formModel.accountIds.length === 0) {
    ElMessage.warning('请选择账号')
    return
  }
  const task = tasksStore.createTask({
    goodsId: formModel.goodsId,
    goodsTitle: resolveGoodsTitle(formModel.goodsId),
    accountIds: formModel.accountIds,
    quantity: formModel.quantity,
    scheduleAt: formModel.scheduleAt || undefined,
  })
  ElMessage.success('任务已创建')
  if (startNow) tasksStore.startTask(task.id)
}

function startTask(id: string) {
  tasksStore.startTask(id)
  ElMessage.success('已启动任务（mock）')
}

function stopTask(id: string) {
  tasksStore.stopTask(id)
  ElMessage.warning('已停止任务')
}

async function removeTask(id: string) {
  await ElMessageBox.confirm('确认删除该任务？', '提示', { type: 'warning' }).catch(() => null)
  tasksStore.removeTask(id)
  ElMessage.success('已删除')
}
</script>

<template>
  <div class="page">
    <el-row :gutter="12">
      <el-col :xs="24" :lg="10">
        <el-card shadow="never" header="创建抢购任务">
          <el-form label-width="110px">
            <el-form-item label="商品">
              <el-select v-model="formModel.goodsId" placeholder="请选择商品" filterable style="width: 100%">
                <el-option v-for="opt in goodsOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
              </el-select>
              <div style="margin-top: 6px; color: #909399">
                提示：可先到「商品列表」设置目标商品，再回来创建任务。
              </div>
            </el-form-item>
            <el-form-item label="账号">
              <el-select
                v-model="formModel.accountIds"
                placeholder="选择要参与抢购的账号（可多选）"
                multiple
                filterable
                collapse-tags
                collapse-tags-tooltip
                style="width: 100%"
              >
                <el-option v-for="opt in accountOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
              </el-select>
            </el-form-item>
            <el-form-item label="数量">
              <el-input-number v-model="formModel.quantity" :min="1" :max="10" />
            </el-form-item>
            <el-form-item label="定时开抢">
              <el-date-picker
                v-model="formModel.scheduleAt"
                type="datetime"
                placeholder="可选：不填则手动启动"
                value-format="YYYY-MM-DDTHH:mm:ss.SSSZ"
                style="width: 100%"
              />
            </el-form-item>
          </el-form>

          <el-space :size="8">
            <el-button type="primary" @click="create(false)">创建任务</el-button>
            <el-button type="success" @click="create(true)">创建并立即开始</el-button>
          </el-space>

          <el-divider />
          <div style="color: #909399">
            这里先把“任务编排/启动/停止/状态展示”搭好；下一步对接 API 后，会在任务运行时为每个账号执行：查询商品 → 校验资格/库存 → 下单 → 支付/确认。
          </div>
        </el-card>
      </el-col>

      <el-col :xs="24" :lg="14">
        <el-card shadow="never" header="任务列表">
          <el-table :data="tasks" row-key="id" size="small" style="width: 100%">
            <el-table-column prop="goodsTitle" label="商品" min-width="220" show-overflow-tooltip />
            <el-table-column label="状态" width="110">
              <template #default="{ row }">
                <StatusTag kind="task" :status="row.status" />
              </template>
            </el-table-column>
            <el-table-column label="账号数" width="90">
              <template #default="{ row }">{{ row.accountIds.length }}</template>
            </el-table-column>
            <el-table-column prop="quantity" label="数量" width="70" />
            <el-table-column label="定时" width="170">
              <template #default="{ row }">{{ row.scheduleAt ? formatTime(row.scheduleAt) : '-' }}</template>
            </el-table-column>
            <el-table-column label="创建时间" width="170">
              <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="260">
              <template #default="{ row }">
                <el-space :size="8" wrap>
                  <el-button size="small" type="success" @click="startTask(row.id)">启动</el-button>
                  <el-button size="small" type="warning" @click="stopTask(row.id)">停止</el-button>
                  <el-button size="small" type="danger" @click="removeTask(row.id)">删除</el-button>
                </el-space>
              </template>
            </el-table-column>
          </el-table>

          <div v-if="tasks.length === 0" style="padding: 8px 0; color: #909399">暂无任务</div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>
