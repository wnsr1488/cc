<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { computed, onMounted, onUnmounted, ref } from 'vue'

import { collectAllMetrics, collectServerMetrics, getServerLiveInsights, listMetricsOverview, listServerMetrics } from '@/api'
import type { ServerLiveInsights, ServerMetric, ServerMetricOverview } from '@/api/types'
import { formatDateTime } from '@/utils/format'

const loading = ref(false)
const collecting = ref(false)
const collectingAll = ref(false)
const loadingLive = ref(false)
const autoRefresh = ref(true)
const overview = ref<ServerMetricOverview[]>([])
const metrics = ref<ServerMetric[]>([])
const liveInsights = ref<ServerLiveInsights | null>(null)
const connPage = ref(1)
const connPageSize = 10
const selectedServerID = ref<number>()
let refreshTimer: number | undefined

const latest = computed(() => metrics.value[0])
const selectedOverview = computed(() => overview.value.find((item) => item.server_id === selectedServerID.value))
const paginatedConnections = computed(() => {
  const items = liveInsights.value?.connections ?? []
  const start = (connPage.value - 1) * connPageSize
  return items.slice(start, start + connPageSize)
})

function formatBytes(value?: number) {
  if (value == null) return '-'
  if (value < 1024) return `${value} B`
  if (value < 1024 ** 2) return `${(value / 1024).toFixed(1)} KB`
  if (value < 1024 ** 3) return `${(value / 1024 ** 2).toFixed(1)} MB`
  return `${(value / 1024 ** 3).toFixed(2)} GB`
}

function formatPercent(value?: number) {
  return value == null ? '-' : `${value.toFixed(1)}%`
}

function formatLoad(value?: number) {
  return value == null ? '-' : value.toFixed(2)
}

function metricClass(value: number | undefined, warn: number, danger: number) {
  if (value == null) return ''
  if (value >= danger) return 'metric-danger'
  if (value >= warn) return 'metric-warn'
  return ''
}

async function loadOverview() {
  overview.value = await listMetricsOverview()
  if (!selectedServerID.value && overview.value.length > 0) {
    selectedServerID.value = overview.value[0].server_id
  }
}

function formatRegion(row: { country?: string; province?: string; city?: string }) {
  const parts = [row.country, row.province, row.city].filter(Boolean)
  return parts.length ? parts.join(' / ') : '-'
}

function formatSetName(name: string) {
  switch (name) {
    case 'cc_blacklist':
      return '黑名单'
    case 'cc_temp_block':
      return '临时封禁'
    case 'cc_rate_block':
      return '连接数封禁'
    default:
      return name
  }
}

function connRowIndex(index: number) {
  return (connPage.value - 1) * connPageSize + index + 1
}

async function loadLiveInsights() {
  if (!selectedServerID.value) return
  loadingLive.value = true
  try {
    liveInsights.value = await getServerLiveInsights(selectedServerID.value, 100)
    connPage.value = 1
  } finally {
    loadingLive.value = false
  }
}

async function loadMetrics() {
  if (!selectedServerID.value) return
  loading.value = true
  try {
    metrics.value = await listServerMetrics(selectedServerID.value)
  } finally {
    loading.value = false
  }
}

async function refreshAll() {
  await loadOverview()
  await Promise.all([loadMetrics(), loadLiveInsights()])
}

async function collectCurrent() {
  if (!selectedServerID.value) return
  collecting.value = true
  try {
    await collectServerMetrics(selectedServerID.value)
    ElMessage.success('指标采集完成')
    await refreshAll()
  } finally {
    collecting.value = false
  }
}

async function collectAll() {
  collectingAll.value = true
  try {
    const result = await collectAllMetrics()
    if (result.failed > 0) {
      ElMessage.warning(`采集完成：成功 ${result.collected} 台，失败 ${result.failed} 台`)
    } else {
      ElMessage.success(`已采集 ${result.collected} 台服务器`)
    }
    await refreshAll()
  } finally {
    collectingAll.value = false
  }
}

function selectServer(serverID: number) {
  selectedServerID.value = serverID
  liveInsights.value = null
  refreshAll()
}

function setupAutoRefresh() {
  if (refreshTimer) {
    window.clearInterval(refreshTimer)
    refreshTimer = undefined
  }
  if (autoRefresh.value) {
    refreshTimer = window.setInterval(() => {
      refreshAll().catch(() => undefined)
    }, 60_000)
  }
}

async function init() {
  await refreshAll()
  setupAutoRefresh()
}

onMounted(init)
onUnmounted(() => {
  if (refreshTimer) window.clearInterval(refreshTimer)
})
</script>

<template>
  <div>
    <div class="toolbar">
      <div>
        <h2>系统监控</h2>
        <p class="muted">SSH 采集 CPU、内存、磁盘、网络、TCP 连接与防火墙命中（默认每 5 分钟自动采集）</p>
      </div>
      <div class="actions">
        <el-switch v-model="autoRefresh" active-text="自动刷新" @change="setupAutoRefresh" />
        <el-button @click="refreshAll">刷新</el-button>
        <el-button type="primary" :loading="collectingAll" @click="collectAll">采集全部服务器</el-button>
      </div>
    </div>

    <el-card class="page-card">
      <template #header>全部服务器概览</template>
      <el-table :data="overview" empty-text="暂无服务器">
        <el-table-column label="服务器" min-width="180">
          <template #default="{ row }">
            <el-button link type="primary" @click="selectServer(row.server_id)">
              {{ row.server_name }} ({{ row.server_host }})
            </el-button>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.server_status === 'online' ? 'success' : 'info'">{{ row.server_status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="CPU" width="90">
          <template #default="{ row }">
            <span :class="metricClass(row.cpu_usage, 70, 90)">{{ formatPercent(row.cpu_usage) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="内存" width="90">
          <template #default="{ row }">
            <span :class="metricClass(row.memory_usage, 80, 95)">{{ formatPercent(row.memory_usage) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="磁盘" width="90">
          <template #default="{ row }">{{ formatPercent(row.disk_usage) }}</template>
        </el-table-column>
        <el-table-column label="Load1" width="80">
          <template #default="{ row }">{{ formatLoad(row.load1) }}</template>
        </el-table-column>
        <el-table-column label="ESTAB" width="90">
          <template #default="{ row }">
            <span :class="metricClass(row.tcp_established, 5000, 10000)">{{ row.tcp_established ?? '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="封禁 IP" width="90">
          <template #default="{ row }">{{ row.blocked_ip_count ?? '-' }}</template>
        </el-table-column>
        <el-table-column label="DROP 命中" width="100">
          <template #default="{ row }">{{ row.iptables_drop_hits ?? '-' }}</template>
        </el-table-column>
        <el-table-column label="采集时间" min-width="170">
          <template #default="{ row }">{{ row.collected_at ? formatDateTime(row.collected_at) : '未采集' }}</template>
        </el-table-column>
      </el-table>
    </el-card>

    <div v-if="selectedOverview" class="detail-section">
      <div class="detail-header">
        <h3>{{ selectedOverview.server_name }} 详情</h3>
        <div class="detail-actions">
          <el-button :loading="loadingLive" @click="loadLiveInsights">刷新连接/封禁明细</el-button>
          <el-button type="primary" :loading="collecting" @click="collectCurrent">立即采集</el-button>
        </div>
      </div>

      <div class="stats-grid" v-if="latest">
        <el-card class="page-card stat-card">
          <span>CPU</span>
          <strong>{{ latest.cpu_usage.toFixed(1) }}%</strong>
        </el-card>
        <el-card class="page-card stat-card">
          <span>内存</span>
          <strong>{{ latest.memory_usage.toFixed(1) }}%</strong>
        </el-card>
        <el-card class="page-card stat-card">
          <span>磁盘 /</span>
          <strong>{{ latest.disk_usage.toFixed(1) }}%</strong>
        </el-card>
        <el-card class="page-card stat-card">
          <span>Load 1/5/15</span>
          <strong>{{ latest.load1 }} / {{ latest.load5 }} / {{ latest.load15 }}</strong>
        </el-card>
        <el-card class="page-card stat-card">
          <span>TCP ESTAB / TW</span>
          <strong>{{ latest.tcp_established }} / {{ latest.tcp_time_wait }}</strong>
        </el-card>
        <el-card class="page-card stat-card">
          <span>网络入/出</span>
          <strong>{{ formatBytes(latest.net_in_bytes) }} / {{ formatBytes(latest.net_out_bytes) }}</strong>
        </el-card>
        <el-card class="page-card stat-card">
          <span>封禁 IP 数</span>
          <strong>{{ latest.blocked_ip_count }}</strong>
        </el-card>
        <el-card class="page-card stat-card">
          <span>iptables DROP</span>
          <strong>{{ latest.iptables_drop_hits }}</strong>
        </el-card>
      </div>

      <el-card v-loading="loadingLive" class="page-card live-card">
        <template #header>
          <div class="live-header">
            <span>实时连接与封禁明细</span>
            <span v-if="liveInsights" class="muted live-time">
              采集于 {{ formatDateTime(liveInsights.collected_at) }}，ESTAB 总数 {{ liveInsights.tcp_established }}
            </span>
          </div>
        </template>
        <el-alert
          class="live-tip"
          title="下方为 SSH 实时拉取：封禁 IP 来自 cc_blacklist / cc_temp_block / cc_rate_block；连接数为非白名单 IP 的 ESTABLISHED 连接，地区由 ip2region 识别。"
          type="info"
          :closable="false"
        />
        <h4>当前封禁 IP（{{ liveInsights?.blocked_ips.length ?? 0 }}）</h4>
        <el-table :data="liveInsights?.blocked_ips ?? []" empty-text="暂无封禁 IP，点击上方按钮刷新">
          <el-table-column prop="ip" label="IP" min-width="140" />
          <el-table-column label="集合" width="120">
            <template #default="{ row }">{{ formatSetName(row.set_name) }}</template>
          </el-table-column>
          <el-table-column label="剩余秒数" width="100">
            <template #default="{ row }">{{ row.timeout_seconds ? row.timeout_seconds : '永久' }}</template>
          </el-table-column>
          <el-table-column label="地区" min-width="180">
            <template #default="{ row }">{{ formatRegion(row) }}</template>
          </el-table-column>
          <el-table-column prop="isp" label="运营商" min-width="120" />
        </el-table>

        <h4 class="conn-title">非白名单 IP 连接数 TOP（共 {{ liveInsights?.connections.length ?? 0 }}，每页 {{ connPageSize }}）</h4>
        <el-table :data="paginatedConnections" empty-text="暂无连接数据，点击上方按钮刷新">
          <el-table-column type="index" label="#" width="60" :index="connRowIndex" />
          <el-table-column prop="ip" label="IP" min-width="140" />
          <el-table-column prop="count" label="ESTAB 连接" width="110" />
          <el-table-column label="地区" min-width="180">
            <template #default="{ row }">{{ formatRegion(row) }}</template>
          </el-table-column>
          <el-table-column prop="isp" label="运营商" min-width="120" />
        </el-table>
        <el-pagination
          v-if="(liveInsights?.connections.length ?? 0) > connPageSize"
          v-model:current-page="connPage"
          class="conn-pagination"
          layout="total, prev, pager, next"
          :page-size="connPageSize"
          :total="liveInsights?.connections.length ?? 0"
        />
      </el-card>

      <el-card class="page-card">
        <el-table v-loading="loading" :data="metrics" empty-text="暂无历史数据，请先采集">
          <el-table-column prop="cpu_usage" label="CPU %" width="90" />
          <el-table-column prop="memory_usage" label="内存 %" width="90" />
          <el-table-column prop="disk_usage" label="磁盘 %" width="90" />
          <el-table-column prop="load1" label="Load1" width="80" />
          <el-table-column prop="tcp_established" label="ESTAB" width="90" />
          <el-table-column prop="tcp_time_wait" label="TW" width="80" />
          <el-table-column label="网络入" width="110">
            <template #default="{ row }">{{ formatBytes(row.net_in_bytes) }}</template>
          </el-table-column>
          <el-table-column label="网络出" width="110">
            <template #default="{ row }">{{ formatBytes(row.net_out_bytes) }}</template>
          </el-table-column>
          <el-table-column prop="blocked_ip_count" label="封禁 IP" width="90" />
          <el-table-column prop="iptables_drop_hits" label="DROP" width="90" />
          <el-table-column label="采集时间" min-width="170">
            <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
          </el-table-column>
        </el-table>
      </el-card>
    </div>
  </div>
</template>

<style scoped>
h2,
h3 {
  margin: 0;
}

.actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 18px;
}

.stat-card :deep(.el-card__body) {
  display: grid;
  gap: 8px;
}

.stat-card span {
  color: #6b7280;
}

.stat-card strong {
  font-size: 20px;
}

.detail-section {
  margin-top: 18px;
}

.detail-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.detail-actions {
  display: flex;
  gap: 10px;
}

.live-card {
  margin-bottom: 18px;
}

.live-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.live-time {
  font-size: 13px;
}

.live-tip {
  margin-bottom: 16px;
}

.live-card h4 {
  margin: 0 0 12px;
}

.conn-title {
  margin-top: 20px;
}

.conn-pagination {
  justify-content: flex-end;
  margin-top: 12px;
}

.metric-warn {
  color: #d97706;
  font-weight: 600;
}

.metric-danger {
  color: #dc2626;
  font-weight: 600;
}
</style>
