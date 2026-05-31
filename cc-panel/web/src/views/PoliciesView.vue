<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import { onMounted, reactive, ref, watch } from 'vue'

import { createPolicy, deletePolicy, executeAllPolicies, executePolicies, listPolicies, listPolicyEvents, listServers, updatePolicy } from '@/api'
import type { AutoPolicy, AutoPolicyEvent, AutoPolicyPayload, ServerAsset } from '@/api/types'
import { formatDateTime } from '@/utils/format'

const loading = ref(false)
const eventsLoading = ref(false)
const saving = ref(false)
const executing = ref(false)
const executingAll = ref(false)
const dialogVisible = ref(false)
const editingID = ref<number | null>(null)
const policies = ref<AutoPolicy[]>([])
const events = ref<AutoPolicyEvent[]>([])
const servers = ref<ServerAsset[]>([])
const selectedServerID = ref<number>()
const eventPage = ref(1)
const eventPageSize = 20
const eventTotal = ref(0)

const form = reactive<AutoPolicyPayload>({
  name: '',
  metric: 'connection_count',
  threshold: 80,
  window_seconds: 10,
  block_seconds: 600,
  target_set: 'cc_rate_block',
  enabled: true,
})

const metricOptions = [
  { label: '单 IP 连接数 (connection_count)', value: 'connection_count' },
]

async function loadEvents() {
  eventsLoading.value = true
  try {
    const result = await listPolicyEvents(eventPage.value, eventPageSize)
    events.value = result.items
    eventTotal.value = result.total
  } finally {
    eventsLoading.value = false
  }
}

async function load() {
  loading.value = true
  try {
    const [policyItems, serverItems] = await Promise.all([listPolicies(), listServers()])
    policies.value = policyItems
    servers.value = serverItems
    if (!selectedServerID.value && serverItems.length > 0) {
      selectedServerID.value = serverItems[0].id
    }
  } finally {
    loading.value = false
  }
}

watch(eventPage, loadEvents)

function resetForm() {
  editingID.value = null
  Object.assign(form, {
    name: '',
    metric: 'connection_count',
    threshold: 80,
    window_seconds: 10,
    block_seconds: 600,
    target_set: 'cc_rate_block',
    enabled: true,
  })
}

function openCreate() {
  resetForm()
  dialogVisible.value = true
}

function openEdit(row: AutoPolicy) {
  editingID.value = row.id
  Object.assign(form, {
    name: row.name,
    metric: row.metric,
    threshold: row.threshold,
    window_seconds: row.window_seconds,
    block_seconds: row.block_seconds,
    target_set: row.target_set,
    enabled: row.enabled,
  })
  dialogVisible.value = true
}

async function save() {
  saving.value = true
  try {
    if (editingID.value) {
      await updatePolicy(editingID.value, form)
      ElMessage.success('策略已更新')
    } else {
      await createPolicy(form)
      ElMessage.success('策略已创建')
    }
    dialogVisible.value = false
    await load()
    await loadEvents()
  } finally {
    saving.value = false
  }
}

async function remove(row: AutoPolicy) {
  await ElMessageBox.confirm(`确认删除策略 ${row.name}？`, '删除确认', { type: 'warning' })
  await deletePolicy(row.id)
  ElMessage.success('策略已删除')
  await load()
  await loadEvents()
}

async function runExecutor() {
  if (!selectedServerID.value) return
  executing.value = true
  try {
    const result = await executePolicies(selectedServerID.value)
    ElMessage.success(`策略执行完成，生成 ${result.length} 条事件`)
    eventPage.value = 1
    await loadEvents()
  } finally {
    executing.value = false
  }
}

async function runExecutorAll() {
  executingAll.value = true
  try {
    const result = await executeAllPolicies()
    ElMessage.success(`全部服务器策略执行完成，生成 ${result.events} 条事件`)
    eventPage.value = 1
    await loadEvents()
  } finally {
    executingAll.value = false
  }
}

onMounted(async () => {
  await load()
  await loadEvents()
})
</script>

<template>
  <div>
    <div class="toolbar">
      <div>
        <h2>自动封禁策略</h2>
        <p class="muted">基于 ss 按源 IP 统计 ESTABLISHED 连接，跳过白名单后超阈值写入 ipset（cc_rate_block / cc_temp_block）</p>
      </div>
      <div>
        <el-select v-model="selectedServerID" placeholder="选择服务器" filterable class="server-select">
          <el-option
            v-for="server in servers"
            :key="server.id"
            :label="`${server.name} (${server.host})`"
            :value="server.id"
          />
        </el-select>
        <el-button type="success" :loading="executing" @click="runExecutor">执行当前服务器</el-button>
        <el-button type="warning" :loading="executingAll" @click="runExecutorAll">执行全部服务器</el-button>
        <el-button @click="load(); loadEvents()">刷新</el-button>
        <el-button type="primary" @click="openCreate">新增策略</el-button>
      </div>
    </div>

    <el-card class="page-card">
      <el-table v-loading="loading" :data="policies" empty-text="暂无策略">
        <el-table-column prop="name" label="名称" />
        <el-table-column prop="metric" label="指标" width="160" />
        <el-table-column prop="threshold" label="阈值" width="100" />
        <el-table-column prop="window_seconds" label="窗口秒数" width="120" />
        <el-table-column prop="block_seconds" label="封禁秒数" width="120" />
        <el-table-column prop="target_set" label="目标集合" width="150" />
        <el-table-column prop="enabled" label="启用" width="90">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '是' : '否' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card class="page-card event-card">
      <template #header>策略执行事件</template>
      <el-table v-loading="eventsLoading" :data="events" empty-text="暂无执行事件">
        <el-table-column prop="policy_id" label="策略 ID" width="100" />
        <el-table-column prop="server_id" label="服务器 ID" width="110" />
        <el-table-column prop="metric" label="指标" width="150" />
        <el-table-column prop="observed_value" label="观测值" width="110" />
        <el-table-column prop="threshold" label="阈值" width="90" />
        <el-table-column prop="action" label="动作" width="130">
          <template #default="{ row }">
            <el-tag :type="row.action === 'blocked' || row.action === 'triggered' ? 'danger' : row.action === 'skipped' ? 'warning' : 'success'">
              {{ row.action === 'blocked' ? '已封禁' : row.action === 'not_triggered' ? '未触发' : row.action === 'skipped' ? '跳过' : row.action }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="详情" min-width="220">
          <template #default="{ row }">
            <code>{{ JSON.stringify(row.detail) }}</code>
          </template>
        </el-table-column>
        <el-table-column label="时间" width="180">
          <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
        </el-table-column>
      </el-table>
      <el-pagination
        v-if="eventTotal > eventPageSize"
        v-model:current-page="eventPage"
        class="event-pagination"
        layout="total, prev, pager, next, jumper"
        :page-size="eventPageSize"
        :total="eventTotal"
      />
    </el-card>

    <el-dialog v-model="dialogVisible" :title="editingID ? '编辑策略' : '新增策略'" width="620px">
      <el-form label-width="120px">
        <el-form-item label="名称">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="指标">
          <el-select v-model="form.metric" class="wide">
            <el-option v-for="item in metricOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="阈值">
          <el-input-number v-model="form.threshold" :min="1" />
          <span class="hint">单 IP 当前 ESTABLISHED 连接数上限</span>
        </el-form-item>
        <el-form-item label="统计窗口">
          <el-input-number v-model="form.window_seconds" :min="1" />
          <span class="hint">秒（预留字段，当前按瞬时连接数判定）</span>
        </el-form-item>
        <el-form-item label="封禁时间">
          <el-input-number v-model="form.block_seconds" :min="1" />
          <span class="hint">秒</span>
        </el-form-item>
        <el-form-item label="目标集合">
          <el-select v-model="form.target_set" class="wide">
            <el-option label="cc_rate_block" value="cc_rate_block" />
            <el-option label="cc_temp_block" value="cc_temp_block" />
          </el-select>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
h2 {
  margin: 0;
}

.wide {
  width: 360px;
}

.hint {
  margin-left: 10px;
  color: #6b7280;
}

.server-select {
  width: 260px;
  margin-right: 8px;
}

.event-card {
  margin-top: 18px;
}

.event-pagination {
  margin-top: 16px;
  justify-content: flex-end;
}
</style>
