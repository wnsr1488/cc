<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'

import { listAuditLogs } from '@/api'
import type { AuditLog } from '@/api/types'
import { formatDateTime } from '@/utils/format'

const loading = ref(false)
const logs = ref<AuditLog[]>([])
const page = ref(1)
const pageSize = 20
const total = ref(0)

async function load() {
  loading.value = true
  try {
    const result = await listAuditLogs(page.value, pageSize)
    logs.value = result.items
    total.value = result.total
  } finally {
    loading.value = false
  }
}

watch(page, load)

onMounted(load)
</script>

<template>
  <div>
    <div class="toolbar">
      <div>
        <h2>审计日志</h2>
        <p class="muted">记录登录、服务器变更和防火墙操作</p>
      </div>
      <el-button type="primary" @click="load">刷新</el-button>
    </div>

    <el-card class="page-card">
      <el-table v-loading="loading" :data="logs" empty-text="暂无审计日志">
        <el-table-column prop="id" label="ID" width="90" />
        <el-table-column prop="action" label="动作" width="200" />
        <el-table-column prop="target_type" label="目标类型" width="120" />
        <el-table-column prop="target_id" label="目标 ID" width="110" />
        <el-table-column prop="remote_addr" label="来源地址" width="180" />
        <el-table-column label="详情" min-width="260">
          <template #default="{ row }">
            <code>{{ JSON.stringify(row.detail) }}</code>
          </template>
        </el-table-column>
        <el-table-column label="时间" width="230">
          <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
        </el-table-column>
      </el-table>
      <el-pagination
        v-if="total > pageSize"
        v-model:current-page="page"
        class="audit-pagination"
        layout="total, prev, pager, next, jumper"
        :page-size="pageSize"
        :total="total"
      />
    </el-card>
  </div>
</template>

<style scoped>
h2 {
  margin: 0;
}

code {
  color: #475569;
  white-space: normal;
}

.audit-pagination {
  margin-top: 16px;
  justify-content: flex-end;
}
</style>
