<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { reactive } from 'vue'

const settings = reactive({
  apiBaseURL: import.meta.env.VITE_API_BASE_URL || '同源代理 /api',
  sshTimeout: '10 秒',
  tokenTTL: '60 分钟',
  deploySnapshot: true,
  autoRollback: true,
  httpsEnabled: false,
})

const features = [
  { name: '部署前规则快照', status: '已完成', note: '部署防护前自动保存 iptables/ipset 快照' },
  { name: '失败自动回滚', status: '已完成', note: '部署失败时自动恢复最近快照' },
  { name: 'HTTPS', status: '部署层配置', note: '需要 Nginx/Caddy 或证书服务接入' },
  { name: '地区封禁 ip2region', status: '已完成', note: '已下载 xdb 数据库，支持 IP 查询、CIDR 导入和地区封禁部署' },
  { name: '自动封禁策略', status: '已接入', note: '已支持策略 CRUD、基于监控指标的手动执行器和事件记录' },
  { name: '系统监控', status: '已完成', note: '已支持 SSH 采集内存、负载和 TCP 状态' },
  { name: 'Agent 模式', status: '待开发', note: '后续做常驻 Agent 和中心通信' },
  { name: 'nftables 兼容', status: '待开发', note: '后续增加 nft 命令模板和规则适配' },
]

function saveLocal() {
  localStorage.setItem('cc_panel_ui_settings', JSON.stringify(settings))
  ElMessage.success('前端显示设置已保存')
}
</script>

<template>
  <div>
    <div class="toolbar">
      <div>
        <h2>系统设置</h2>
        <p class="muted">展示当前运行配置、安全状态和功能完成进度</p>
      </div>
      <el-button type="primary" @click="saveLocal">保存显示设置</el-button>
    </div>

    <el-row :gutter="18">
      <el-col :span="14">
        <el-card class="page-card">
          <template #header>运行配置</template>
          <el-form label-width="150px">
            <el-form-item label="API 地址">
              <el-input v-model="settings.apiBaseURL" disabled />
            </el-form-item>
            <el-form-item label="SSH 超时">
              <el-input v-model="settings.sshTimeout" disabled />
            </el-form-item>
            <el-form-item label="Token 有效期">
              <el-input v-model="settings.tokenTTL" disabled />
            </el-form-item>
          </el-form>
        </el-card>
      </el-col>

      <el-col :span="10">
        <el-card class="page-card">
          <template #header>安全状态</template>
          <div class="security-list">
            <el-alert title="请修改默认管理员密码" type="warning" :closable="false" />
            <el-alert title="生产环境应启用 HTTPS" type="warning" :closable="false" />
            <el-alert title="规则快照和失败自动回滚已启用" type="success" :closable="false" />
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-card class="page-card feature-card">
      <template #header>功能进度</template>
      <el-table :data="features">
        <el-table-column prop="name" label="能力" />
        <el-table-column prop="status" label="状态" width="140">
          <template #default="{ row }">
            <el-tag
              :type="
                row.status === '已完成'
                  ? 'success'
                  : row.status === '已接入'
                    ? 'success'
                  : row.status === '部分完成'
                    ? 'warning'
                    : row.status === '部署层配置'
                      ? 'info'
                      : 'danger'
              "
            >
              {{ row.status }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="note" label="说明" />
      </el-table>
    </el-card>
  </div>
</template>

<style scoped>
h2 {
  margin: 0;
}

.security-list {
  display: grid;
  gap: 12px;
}

.feature-card {
  margin-top: 18px;
}
</style>
