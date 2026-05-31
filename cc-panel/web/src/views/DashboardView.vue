<script setup lang="ts">
import { Connection, DataLine, Lock, Monitor } from '@element-plus/icons-vue'
import { computed, onMounted, ref } from 'vue'

import { listAuditLogs, listMetricsOverview, listServers } from '@/api'
import type { AuditLog, ServerAsset, ServerMetricOverview } from '@/api/types'
import { formatDateTime } from '@/utils/format'

const servers = ref<ServerAsset[]>([])
const logs = ref<AuditLog[]>([])
const overview = ref<ServerMetricOverview[]>([])
const loading = ref(false)

const onlineCount = computed(() => servers.value.filter((item) => item.status === 'online').length)
const offlineCount = computed(() => servers.value.filter((item) => item.status === 'offline').length)
const monitoredCount = computed(() => overview.value.filter((item) => item.collected_at).length)
const highLoadServers = computed(() =>
  overview.value
    .filter((item) => (item.load1 ?? 0) >= 2 || (item.tcp_established ?? 0) >= 3000)
    .slice(0, 5),
)

async function load() {
  loading.value = true
  try {
    const [serverItems, auditPage, overviewItems] = await Promise.all([
      listServers(),
      listAuditLogs(1, 6),
      listMetricsOverview(),
    ])
    servers.value = serverItems
    logs.value = auditPage.items
    overview.value = overviewItems
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div v-loading="loading">
    <div class="toolbar">
      <div>
        <h2>总览</h2>
        <p class="muted">查看服务器状态、监控摘要与最近操作</p>
      </div>
      <el-button type="primary" @click="load">刷新</el-button>
    </div>

    <div class="stats-grid">
      <el-card class="page-card stat-card">
        <el-icon><Monitor /></el-icon>
        <span>服务器总数</span>
        <strong>{{ servers.length }}</strong>
      </el-card>
      <el-card class="page-card stat-card">
        <el-icon><Connection /></el-icon>
        <span>在线服务器</span>
        <strong>{{ onlineCount }}</strong>
      </el-card>
      <el-card class="page-card stat-card">
        <el-icon><DataLine /></el-icon>
        <span>已采集监控</span>
        <strong>{{ monitoredCount }}</strong>
      </el-card>
      <el-card class="page-card stat-card">
        <el-icon><Lock /></el-icon>
        <span>离线服务器</span>
        <strong>{{ offlineCount }}</strong>
      </el-card>
    </div>

    <el-row :gutter="18">
      <el-col :span="14">
        <el-card class="page-card recent-card">
          <template #header>监控摘要</template>
          <el-table :data="overview" empty-text="暂无监控数据，请到系统监控页采集">
            <el-table-column label="服务器" min-width="160">
              <template #default="{ row }">{{ row.server_name }}</template>
            </el-table-column>
            <el-table-column label="CPU" width="80">
              <template #default="{ row }">{{ row.cpu_usage != null ? `${row.cpu_usage.toFixed(0)}%` : '-' }}</template>
            </el-table-column>
            <el-table-column label="内存" width="80">
              <template #default="{ row }">{{ row.memory_usage != null ? `${row.memory_usage.toFixed(0)}%` : '-' }}</template>
            </el-table-column>
            <el-table-column label="Load1" width="80">
              <template #default="{ row }">{{ row.load1?.toFixed(2) ?? '-' }}</template>
            </el-table-column>
            <el-table-column label="ESTAB" width="90">
              <template #default="{ row }">{{ row.tcp_established ?? '-' }}</template>
            </el-table-column>
            <el-table-column label="封禁 IP" width="90">
              <template #default="{ row }">{{ row.blocked_ip_count ?? '-' }}</template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>
      <el-col :span="10">
        <el-card class="page-card recent-card">
          <template #header>需关注</template>
          <el-empty v-if="highLoadServers.length === 0" description="当前无高负载告警" />
          <el-table v-else :data="highLoadServers">
            <el-table-column prop="server_name" label="服务器" />
            <el-table-column label="Load1" width="80">
              <template #default="{ row }">{{ row.load1?.toFixed(2) ?? '-' }}</template>
            </el-table-column>
            <el-table-column label="ESTAB" width="90">
              <template #default="{ row }">{{ row.tcp_established ?? '-' }}</template>
            </el-table-column>
          </el-table>
        </el-card>

        <el-card class="page-card recent-card">
          <template #header>最近操作</template>
          <el-table :data="logs" empty-text="暂无审计日志">
            <el-table-column prop="action" label="动作" width="190" />
            <el-table-column label="时间" width="180">
              <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<style scoped>
h2 {
  margin: 0;
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

.stat-card .el-icon {
  color: #2563eb;
  font-size: 24px;
}

.stat-card span {
  color: #6b7280;
}

.stat-card strong {
  font-size: 30px;
}

.recent-card {
  margin-bottom: 18px;
}
</style>
