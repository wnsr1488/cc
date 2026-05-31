<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { deployServer, getFirewallStatus, getServer, rollbackServer, stopServerRules, testSSH, updateServer } from '@/api'
import type { FirewallStatus, ServerAsset, WhitelistMode } from '@/api/types'
import { formatDateTime } from '@/utils/format'

const route = useRoute()
const router = useRouter()
const loading = ref(false)
const checking = ref(false)
const stopping = ref(false)
const savingMode = ref(false)
const server = ref<ServerAsset | null>(null)
const firewallStatus = ref<FirewallStatus>()
const serverID = computed(() => Number(route.params.id))

const whitelistModeLabel: Record<WhitelistMode, string> = {
  off: '关闭',
  strict_whitelist: '严格白名单',
  connection_count: '连接数封禁',
}

async function load() {
  loading.value = true
  try {
    server.value = await getServer(serverID.value)
  } finally {
    loading.value = false
  }
}

async function runTest() {
  await testSSH(serverID.value)
  ElMessage.success('SSH 连接正常')
  await load()
}

async function runDeploy() {
  await deployServer(serverID.value)
  ElMessage.success('防护规则已部署')
  await checkFirewall()
  await load()
}

async function checkFirewall() {
  checking.value = true
  try {
    firewallStatus.value = await getFirewallStatus(serverID.value)
    ElMessage.success(firewallStatus.value.mounted ? '防护规则已挂载' : '未检测到 iptables 引用')
  } finally {
    checking.value = false
  }
}

async function runRollback() {
  const result = await rollbackServer(serverID.value)
  ElMessage.success(`已回滚到快照 #${result.snapshot_id}`)
  await load()
}

async function runStopRules() {
  await ElMessageBox.confirm(
    '将移除 cc-panel iptables 规则（保留 ipset 数据）。停止后流量不再受防护规则约束，确认继续？',
    '停止防护规则',
    { type: 'warning', confirmButtonText: '确认停止', cancelButtonText: '取消' },
  )
  stopping.value = true
  try {
    await stopServerRules(serverID.value)
    ElMessage.success('防护 iptables 规则已停止')
    await checkFirewall()
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      ElMessage.error(error instanceof Error ? error.message : '停止防护规则失败')
    }
  } finally {
    stopping.value = false
  }
}

async function changeWhitelistMode(mode: WhitelistMode) {
  if (!server.value || server.value.whitelist_mode === mode) return
  if (mode === 'strict_whitelist') {
    await ElMessageBox.confirm(
      '开启后仅白名单 IP 可访问，其余全部 DROP。请确认你的管理 IP 已在 cc_whitelist 或 cc_geo_whitelist 中。',
      '切换为严格白名单',
      { type: 'warning', confirmButtonText: '确认切换', cancelButtonText: '取消' },
    )
  }
  savingMode.value = true
  try {
    server.value = await updateServer(serverID.value, {
      name: server.value.name,
      host: server.value.host,
      port: server.value.port,
      username: server.value.username,
      auth_type: server.value.auth_type,
      group_name: server.value.group_name,
      whitelist_mode: mode,
    })
    await deployServer(serverID.value)
    ElMessage.success(`已切换为${whitelistModeLabel[mode]}并重新部署`)
    await checkFirewall()
  } catch (error) {
    await load()
    if (!(error === 'cancel' || error === 'close')) {
      ElMessage.error(error instanceof Error ? error.message : '更新防护模式失败')
    }
  } finally {
    savingMode.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <div class="toolbar">
      <div>
        <h2>服务器详情</h2>
        <p class="muted">查看服务器连接信息和防护部署状态</p>
      </div>
      <div>
        <el-button @click="router.push('/servers')">返回列表</el-button>
        <el-button @click="load">刷新</el-button>
        <el-button type="primary" @click="runTest">测试 SSH</el-button>
        <el-button type="success" @click="runDeploy">部署防护</el-button>
        <el-button type="danger" :loading="stopping" @click="runStopRules">停止规则</el-button>
        <el-button type="info" :loading="checking" @click="checkFirewall">检测规则</el-button>
        <el-button type="warning" @click="runRollback">回滚规则</el-button>
      </div>
    </div>

    <el-card v-loading="loading" class="page-card" v-if="server">
      <el-descriptions :column="2" border>
        <el-descriptions-item label="名称">{{ server.name }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="server.status === 'online' ? 'success' : server.status === 'offline' ? 'danger' : 'info'">
            {{ server.status }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="主机">{{ server.host }}</el-descriptions-item>
        <el-descriptions-item label="端口">{{ server.port }}</el-descriptions-item>
        <el-descriptions-item label="用户名">{{ server.username }}</el-descriptions-item>
        <el-descriptions-item label="认证方式">{{ server.auth_type }}</el-descriptions-item>
        <el-descriptions-item label="分组">{{ server.group_name || '-' }}</el-descriptions-item>
        <el-descriptions-item label="最后在线">{{ formatDateTime(server.last_seen_at) }}</el-descriptions-item>
        <el-descriptions-item label="系统信息">{{ server.os_info || '-' }}</el-descriptions-item>
        <el-descriptions-item label="内核版本">{{ server.kernel_version || '-' }}</el-descriptions-item>
        <el-descriptions-item label="防护模式" :span="2">
          <el-radio-group
            :model-value="server.whitelist_mode || 'off'"
            :disabled="savingMode"
            @change="changeWhitelistMode"
          >
            <el-radio value="strict_whitelist">严格白名单（推荐）</el-radio>
            <el-radio value="connection_count">连接数封禁</el-radio>
            <el-radio value="off">关闭</el-radio>
          </el-radio-group>
          <div class="form-tip">严格白名单与连接数封禁互斥，切换后会自动重新部署 iptables</div>
        </el-descriptions-item>
        <el-descriptions-item label="创建时间">{{ formatDateTime(server.created_at) }}</el-descriptions-item>
        <el-descriptions-item label="更新时间">{{ formatDateTime(server.updated_at) }}</el-descriptions-item>
      </el-descriptions>
    </el-card>

    <el-card v-if="firewallStatus" class="page-card status-card">
      <template #header>规则生效检测</template>
      <el-alert
        class="status-alert"
        :title="firewallStatus.mounted ? 'iptables 已引用防护 ipset，规则已挂载' : '未检测到 iptables 引用防护 ipset'"
        :type="firewallStatus.mounted ? 'success' : 'warning'"
        :closable="false"
      />
      <el-alert
        v-if="firewallStatus.whitelist_mode === 'strict_whitelist'"
        class="status-alert"
        :title="firewallStatus.strict_whitelist_active ? '严格白名单已生效：非白名单 IP 将被 DROP' : '已配置严格白名单，但目标机器尚未检测到默认 DROP 规则，请重新部署'"
        :type="firewallStatus.strict_whitelist_active ? 'warning' : 'info'"
        :closable="false"
      />
      <el-alert
        v-else-if="firewallStatus.whitelist_mode === 'connection_count'"
        class="status-alert"
        title="连接数封禁模式：非白名单 IP 连接数超阈值时写入 ipset，不会默认 DROP 全部非白名单流量"
        type="success"
        :closable="false"
      />
      <el-table :data="firewallStatus.ipsets" empty-text="未检测到 ipset 集合">
        <el-table-column prop="name" label="ipset 集合" />
        <el-table-column label="存在" width="100">
          <template #default="{ row }">
            <el-tag :type="row.exists ? 'success' : 'danger'">{{ row.exists ? '是' : '否' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="entries" label="条目数量" width="120" />
      </el-table>
      <el-divider>iptables 引用</el-divider>
      <pre class="rule-output">{{ firewallStatus.iptables_rules.length ? firewallStatus.iptables_rules.join('\n') : '未检测到 cc_* 规则引用' }}</pre>
      <el-divider>命中计数</el-divider>
      <pre class="rule-output">{{ firewallStatus.iptables_counts.length ? firewallStatus.iptables_counts.join('\n') : '暂无 cc_* 命中计数' }}</pre>
    </el-card>
  </div>
</template>

<style scoped>
h2 {
  margin: 0;
}

.form-tip {
  color: #6b7280;
  font-size: 12px;
  margin-top: 6px;
}

.status-card {
  margin-top: 18px;
}

.status-alert {
  margin-bottom: 16px;
}

.rule-output {
  background: #0f172a;
  border-radius: 6px;
  color: #d1d5db;
  overflow: auto;
  padding: 12px;
  white-space: pre-wrap;
}
</style>
