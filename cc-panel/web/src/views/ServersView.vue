<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'

import { createServer, deleteServer, deployServer, listServers, testSSH, updateServer } from '@/api'
import type { ServerAsset, ServerPayload } from '@/api/types'

const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)
const editingID = ref<number | null>(null)
const servers = ref<ServerAsset[]>([])
const router = useRouter()

const form = reactive<ServerPayload>({
  name: '',
  host: '',
  port: 22,
  username: 'root',
  auth_type: 'password',
  password: '',
  private_key: '',
  group_name: '',
})

async function load() {
  loading.value = true
  try {
    servers.value = await listServers()
  } finally {
    loading.value = false
  }
}

function resetForm() {
  editingID.value = null
  Object.assign(form, {
    name: '',
    host: '',
    port: 22,
    username: 'root',
    auth_type: 'password',
    password: '',
    private_key: '',
    group_name: '',
  })
}

function openCreate() {
  resetForm()
  dialogVisible.value = true
}

function openEdit(row: ServerAsset) {
  editingID.value = row.id
  Object.assign(form, {
    name: row.name,
    host: row.host,
    port: row.port,
    username: row.username,
    auth_type: row.auth_type,
    password: '',
    private_key: '',
    group_name: row.group_name || '',
  })
  dialogVisible.value = true
}

async function save() {
  saving.value = true
  try {
    const payload = { ...form }
    if (payload.auth_type === 'password') delete payload.private_key
    if (payload.auth_type === 'private_key') delete payload.password

    if (editingID.value) {
      await updateServer(editingID.value, payload)
      ElMessage.success('服务器已更新')
    } else {
      await createServer(payload)
      ElMessage.success('服务器已添加')
    }
    dialogVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

async function remove(row: ServerAsset) {
  await ElMessageBox.confirm(`确认删除服务器 ${row.name}？`, '删除确认', { type: 'warning' })
  await deleteServer(row.id)
  ElMessage.success('服务器已删除')
  await load()
}

async function runTest(row: ServerAsset) {
  await testSSH(row.id)
  ElMessage.success('SSH 连接正常')
  await load()
}

async function runDeploy(row: ServerAsset) {
  await deployServer(row.id)
  ElMessage.success('防护规则已部署')
  await load()
}

onMounted(load)
</script>

<template>
  <div>
    <div class="toolbar">
      <div>
        <h2>服务器管理</h2>
        <p class="muted">添加服务器、测试 SSH，并部署 ipset/iptables 基础防护规则</p>
      </div>
      <div>
        <el-button @click="load">刷新</el-button>
        <el-button type="primary" @click="openCreate">添加服务器</el-button>
      </div>
    </div>

    <el-card class="page-card">
      <el-table v-loading="loading" :data="servers" empty-text="暂无服务器">
        <el-table-column prop="name" label="名称" min-width="140" />
        <el-table-column prop="host" label="地址" min-width="160" />
        <el-table-column prop="port" label="端口" width="90" />
        <el-table-column prop="username" label="用户" width="120" />
        <el-table-column prop="auth_type" label="认证" width="120" />
        <el-table-column prop="status" label="状态" width="110">
          <template #default="{ row }">
            <el-tag :type="row.status === 'online' ? 'success' : row.status === 'offline' ? 'danger' : 'info'">
              {{ row.status }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="390" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="router.push(`/servers/${row.id}`)">详情</el-button>
            <el-button size="small" @click="runTest(row)">测试 SSH</el-button>
            <el-button size="small" type="primary" @click="runDeploy(row)">部署</el-button>
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="editingID ? '编辑服务器' : '添加服务器'" width="620px">
      <el-form label-width="100px">
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="web-01" />
        </el-form-item>
        <el-form-item label="主机">
          <el-input v-model="form.host" placeholder="1.2.3.4 或 example.com" />
        </el-form-item>
        <el-form-item label="端口">
          <el-input-number v-model="form.port" :min="1" :max="65535" />
        </el-form-item>
        <el-form-item label="用户名">
          <el-input v-model="form.username" />
        </el-form-item>
        <el-form-item label="认证方式">
          <el-radio-group v-model="form.auth_type">
            <el-radio-button label="password">密码</el-radio-button>
            <el-radio-button label="private_key">私钥</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="form.auth_type === 'password'" label="密码">
          <el-input v-model="form.password" type="password" show-password />
        </el-form-item>
        <el-form-item v-else label="私钥">
          <el-input v-model="form.private_key" type="textarea" :rows="6" />
        </el-form-item>
        <el-form-item label="分组">
          <el-input v-model="form.group_name" placeholder="可选" />
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
</style>
