<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import { computed, onMounted, reactive, ref, watch } from 'vue'

import {
  addBlacklist,
  addWhitelist,
  bulkAddBlacklist,
  bulkAddWhitelist,
  deleteBlacklist,
  deleteWhitelist,
  listBlacklist,
  listServers,
  listWhitelist,
} from '@/api'
import type { FirewallEntry, ServerAsset } from '@/api/types'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{
  mode: 'blacklist' | 'whitelist'
}>()

const loading = ref(false)
const submitting = ref(false)
const addMode = ref<'single' | 'bulk'>('single')
const servers = ref<ServerAsset[]>([])
const entries = ref<FirewallEntry[]>([])
const keyword = ref('')
const entryPage = ref(1)
const entryPageSize = 10
const activeServerID = ref<number | null>(null)

interface ServerEntryGroup {
  server_id: number
  server_name: string
  server_host: string
  entries: FirewallEntry[]
}

const form = reactive({
  server_ids: [] as number[],
  ip: '',
  bulk_ips: '',
  timeout: 0,
  reason: '',
})

const title = computed(() => (props.mode === 'blacklist' ? '黑名单' : '白名单'))
const targetSetName = computed(() => (props.mode === 'blacklist' ? 'cc_blacklist' : 'cc_whitelist'))
const description = computed(() =>
  props.mode === 'blacklist'
    ? '将恶意 IP 写入远程服务器 cc_blacklist'
    : '将可信 IP 写入远程服务器 cc_whitelist，白名单规则优先级最高',
)
const parsedBulkIPs = computed(() => parseBulkIPs(form.bulk_ips))

const serverGroups = computed(() => {
  const groupMap = new Map<number, ServerEntryGroup>()
  for (const entry of entries.value) {
    let group = groupMap.get(entry.server_id)
    if (!group) {
      const server = servers.value.find((item) => item.id === entry.server_id)
      group = {
        server_id: entry.server_id,
        server_name: entry.server_name || server?.name || `#${entry.server_id}`,
        server_host: server?.host || '',
        entries: [],
      }
      groupMap.set(entry.server_id, group)
    }
    group.entries.push(entry)
  }
  return Array.from(groupMap.values()).sort((a, b) => a.server_name.localeCompare(b.server_name, 'zh-CN'))
})

const activeGroup = computed(() => serverGroups.value.find((group) => group.server_id === activeServerID.value) ?? null)

const filteredEntries = computed(() => {
  const list = activeGroup.value?.entries ?? []
  const value = keyword.value.trim().toLowerCase()
  if (!value) return list
  return list.filter((entry) =>
    [entry.ip, entry.reason].some((item) => item?.toLowerCase().includes(value)),
  )
})
const paginatedEntries = computed(() => {
  const start = (entryPage.value - 1) * entryPageSize
  return filteredEntries.value.slice(start, start + entryPageSize)
})

function entryRowIndex(index: number) {
  return (entryPage.value - 1) * entryPageSize + index + 1
}

function parseBulkIPs(text: string) {
  const seen = new Set<string>()
  const items: string[] = []
  for (const part of text.split(/[\n,，;\s]+/)) {
    const ip = part.trim()
    if (!ip || seen.has(ip)) continue
    seen.add(ip)
    items.push(ip)
  }
  return items
}

async function load() {
  loading.value = true
  try {
    const [serverItems, entryItems] = await Promise.all([
      listServers(),
      props.mode === 'blacklist' ? listBlacklist(5000) : listWhitelist(5000),
    ])
    servers.value = serverItems
    entries.value = entryItems
    entryPage.value = 1
    if (!serverGroups.value.some((group) => group.server_id === activeServerID.value)) {
      activeServerID.value = serverGroups.value[0]?.server_id ?? null
    }
  } finally {
    loading.value = false
  }
}

async function submitSingle() {
  if (form.server_ids.length === 0) {
    ElMessage.warning('请先选择目标服务器')
    return
  }
  if (!form.ip.trim()) {
    ElMessage.warning('请输入 IP 地址')
    return
  }
  submitting.value = true
  try {
    const payload = {
      server_ids: form.server_ids,
      ip: form.ip.trim(),
      timeout: form.timeout,
      reason: form.reason || undefined,
    }
    if (props.mode === 'blacklist') {
      await addBlacklist(payload)
    } else {
      await addWhitelist(payload)
    }
    ElMessage.success(`${title.value}已同步到服务器`)
    form.ip = ''
    form.reason = ''
    await load()
  } finally {
    submitting.value = false
  }
}

async function submitBulk() {
  if (form.server_ids.length === 0) {
    ElMessage.warning('请先选择目标服务器')
    return
  }
  const ips = parsedBulkIPs.value
  if (ips.length === 0) {
    ElMessage.warning('请粘贴至少一个 IP 地址')
    return
  }
  if (ips.length > 500) {
    ElMessage.warning('单次最多添加 500 个 IP')
    return
  }
  await ElMessageBox.confirm(
    `确认将 ${ips.length} 个 IP 写入 ${form.server_ids.length} 台服务器的 ${title.value}（${targetSetName.value}）？`,
    `批量添加${title.value}确认`,
    { type: props.mode === 'blacklist' ? 'warning' : 'info' },
  )
  submitting.value = true
  try {
    const payload = {
      server_ids: form.server_ids,
      ips,
      timeout: form.timeout,
      reason: form.reason || undefined,
    }
    const result =
      props.mode === 'blacklist' ? await bulkAddBlacklist(payload) : await bulkAddWhitelist(payload)
    const skippedText = result.skipped > 0 ? `，跳过 ${result.skipped} 个已存在` : ''
    ElMessage.success(`已新增 ${result.added} 条${title.value}记录${skippedText}`)
    form.bulk_ips = ''
    form.reason = ''
    await load()
  } finally {
    submitting.value = false
  }
}

async function submit() {
  if (addMode.value === 'bulk') {
    await submitBulk()
  } else {
    await submitSingle()
  }
}

async function remove(entry: FirewallEntry) {
  await ElMessageBox.confirm(`确认从 ${entry.server_name || entry.server_id} 删除 ${entry.ip}？`, '删除确认', {
    type: 'warning',
  })
  if (props.mode === 'blacklist') {
    await deleteBlacklist(entry.id)
  } else {
    await deleteWhitelist(entry.id)
  }
  ElMessage.success('记录已删除并同步到服务器')
  await load()
}

onMounted(load)

watch(
  () => props.mode,
  () => {
    form.ip = ''
    form.bulk_ips = ''
    form.reason = ''
    addMode.value = 'single'
    entryPage.value = 1
    activeServerID.value = null
    load()
  },
)

watch(keyword, () => {
  entryPage.value = 1
})

watch(activeServerID, () => {
  entryPage.value = 1
})
</script>

<template>
  <div>
    <div class="toolbar">
      <div>
        <div class="title-row">
          <h2>{{ title }}</h2>
          <el-tag :type="mode === 'blacklist' ? 'danger' : 'success'" effect="dark">{{ targetSetName }}</el-tag>
        </div>
        <p class="muted">{{ description }}</p>
      </div>
      <el-button @click="load">刷新</el-button>
    </div>

    <el-alert
      v-if="mode === 'blacklist'"
      class="mode-tip"
      title="当前是黑名单页面，IP 会被封禁（写入 cc_blacklist）。若要放行可信 IP，请点击左侧菜单「白名单」。"
      type="error"
      :closable="false"
      show-icon
    />
    <el-alert
      v-else
      class="mode-tip"
      title="当前是白名单页面，IP 会被放行（写入 cc_whitelist）。"
      type="success"
      :closable="false"
      show-icon
    />

    <el-card class="page-card form-card" v-loading="loading">
      <el-form label-width="120px">
        <el-form-item label="添加方式">
          <el-radio-group v-model="addMode">
            <el-radio value="single">单个添加</el-radio>
            <el-radio value="bulk">批量添加</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="目标服务器">
          <el-select v-model="form.server_ids" multiple filterable placeholder="选择服务器" class="wide">
            <el-option
              v-for="server in servers"
              :key="server.id"
              :label="`${server.name} (${server.host})`"
              :value="server.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item v-if="addMode === 'single'" label="IP 地址">
          <el-input v-model="form.ip" placeholder="例如 8.8.8.8" class="wide" />
        </el-form-item>
        <el-form-item v-else label="IP 列表">
          <el-input
            v-model="form.bulk_ips"
            type="textarea"
            :rows="12"
            placeholder="每行一个 IP，也支持逗号、空格分隔"
            class="wide"
          />
          <div class="hint block-hint">已识别 {{ parsedBulkIPs.length }} 个 IP（自动去重，单次最多 500 个）</div>
        </el-form-item>
        <el-form-item :label="mode === 'blacklist' ? '封禁时间' : '有效期'">
          <el-input-number v-model="form.timeout" :min="0" />
          <span class="hint">秒，0 表示永久</span>
        </el-form-item>
        <el-form-item label="原因">
          <el-input v-model="form.reason" type="textarea" :rows="3" class="wide" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="submitting" @click="submit">
            {{ addMode === 'bulk' ? `批量同步（${parsedBulkIPs.length} 个 IP）` : '同步到服务器' }}
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card class="page-card list-card">
      <template #header>
        <div class="list-header">
          <span>{{ title }}记录</span>
          <el-input v-model="keyword" placeholder="搜索 IP / 原因" clearable class="search-input" />
        </div>
      </template>

      <el-empty v-if="!loading && serverGroups.length === 0" description="暂无记录" />

      <el-tabs v-else v-model="activeServerID" class="server-tabs">
        <el-tab-pane
          v-for="group in serverGroups"
          :key="group.server_id"
          :name="group.server_id"
          :label="`${group.server_name}${group.server_host ? ` (${group.server_host})` : ''} · ${group.entries.length}`"
        />
      </el-tabs>

      <template v-if="activeGroup">
        <div class="group-summary">
          当前服务器：<strong>{{ activeGroup.server_name }}</strong>
          <span v-if="activeGroup.server_host" class="muted">（{{ activeGroup.server_host }}）</span>
          · 共 {{ filteredEntries.length }} 条，每页 {{ entryPageSize }} 条
        </div>
        <el-table v-loading="loading" :data="paginatedEntries" empty-text="该服务器暂无匹配记录">
          <el-table-column type="index" label="#" width="60" :index="entryRowIndex" />
          <el-table-column prop="ip" label="IP" min-width="150" />
          <el-table-column prop="timeout_seconds" label="超时秒数" width="120" />
          <el-table-column prop="reason" label="原因" min-width="180" />
          <el-table-column label="创建时间" width="180">
            <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="110" fixed="right">
            <template #default="{ row }">
              <el-button size="small" type="danger" @click="remove(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-pagination
          v-if="filteredEntries.length > entryPageSize"
          v-model:current-page="entryPage"
          class="entry-pagination"
          layout="total, prev, pager, next, jumper"
          :page-size="entryPageSize"
          :total="filteredEntries.length"
        />
      </template>
    </el-card>
  </div>
</template>

<style scoped>
h2 {
  margin: 0;
}

.title-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.mode-tip {
  margin-bottom: 16px;
}

.form-card {
  max-width: 760px;
}

.wide {
  width: 520px;
}

.hint {
  margin-left: 12px;
  color: #6b7280;
}

.block-hint {
  display: block;
  margin-left: 0;
  margin-top: 8px;
}

.list-card {
  margin-top: 18px;
}

.list-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.search-input {
  width: 280px;
}

.entry-pagination {
  justify-content: flex-end;
  margin-top: 16px;
}

.server-tabs {
  margin-bottom: 12px;
}

.group-summary {
  margin-bottom: 12px;
  color: #374151;
}

.group-summary strong {
  color: #111827;
}
</style>
