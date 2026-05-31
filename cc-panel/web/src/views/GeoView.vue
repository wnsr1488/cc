<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import { computed, onMounted, reactive, ref, watch } from 'vue'

import {
  addGeoCIDR,
  bulkAddGeoCIDRs,
  createDefaultGeoWhitelist,
  createGeoRule,
  deployServer,
  getDefaultGeoWhitelist,
  getDefaultGeoWhitelistAutoSync,
  getFirewallStatus,
  getGeoOptions,
  listGeoCIDRs,
  listGeoCIDRSummaries,
  listGeoRules,
  listServers,
  previewGeoCIDRs,
  searchGeoIP,
  stopServerRules,
  syncDefaultGeoWhitelist,
  updateDefaultGeoWhitelistAutoSync,
} from '@/api'
import type { FirewallStatus, GeoAutoSyncConfig, GeoCIDR, GeoCIDRPreview, GeoCIDRSummary, GeoOptions, GeoRule, RegionInfo, ServerAsset, WhitelistMode } from '@/api/types'
import { formatDateTime } from '@/utils/format'

const loading = ref(false)
const submittingCIDR = ref(false)
const submittingRule = ref(false)
const submittingDefaultWhitelist = ref(false)
const syncingDefaultWhitelist = ref(false)
const searching = ref(false)
const smartPreviewing = ref(false)
const smartImporting = ref(false)
const cidrSummaries = ref<GeoCIDRSummary[]>([])
const cidrDetailVisible = ref(false)
const cidrDetailTitle = ref('')
const cidrDetailLoading = ref(false)
const cidrDetails = ref<GeoCIDR[]>([])
const previews = ref<GeoCIDRPreview[]>([])
const rules = ref<GeoRule[]>([])
const servers = ref<ServerAsset[]>([])
const regionInfo = ref<RegionInfo>()
const geoOptions = ref<GeoOptions>({ countries: [], provinces: [], cities: [] })
const defaultWhitelistCountries = ref<string[]>([])

const searchForm = reactive({
  ip: '',
})

const cidrForm = reactive({
  country: '',
  province: '',
  city: '',
  cidr: '',
})

const smartForm = reactive({
  cidrs: '8.8.8.0/24\n1.1.1.0/24',
})

const ruleForm = reactive({
  name: '',
  country: '',
  province: '',
  city: '',
  action: 'DROP' as 'DROP' | 'ACCEPT',
  server_ids: [] as number[],
  enabled: true,
})

const defaultWhitelistForm = reactive({
  server_ids: [] as number[],
  enabled: true,
  whitelist_mode: 'strict_whitelist' as WhitelistMode,
  auto_sync: false,
})

const autoSyncState = ref<GeoAutoSyncConfig | null>(null)
const savingAutoSync = ref(false)
const checkingServerStatus = ref(false)
const mountingServerRules = ref(false)
const stoppingServerRules = ref(false)
const targetServerStatuses = ref<Array<FirewallStatus & { server_name: string; server_host: string }>>([])

const defaultWhitelistProgress = reactive({
  visible: false,
  percent: 0,
  label: '',
  title: '部署进度',
  status: '' as '' | 'success' | 'exception',
})

function beginProgress(title: string, label: string) {
  defaultWhitelistProgress.visible = true
  defaultWhitelistProgress.title = title
  defaultWhitelistProgress.percent = 0
  defaultWhitelistProgress.label = label
  defaultWhitelistProgress.status = ''
}

function finishProgress(label: string, status: '' | 'success' | 'exception' = 'success') {
  defaultWhitelistProgress.percent = 100
  defaultWhitelistProgress.label = label
  defaultWhitelistProgress.status = status
}

function hideProgressLater() {
  window.setTimeout(() => {
    defaultWhitelistProgress.visible = false
    defaultWhitelistProgress.status = ''
  }, 1500)
}

const ruleTitle = computed(() => (ruleForm.action === 'ACCEPT' ? '创建地区白名单' : '创建地区封禁'))
const countryOnlyRule = computed(() => ruleForm.action === 'ACCEPT')
const autoSyncSummary = computed(() => {
  const state = autoSyncState.value
  if (!state) {
    return '加载中...'
  }
  const parts = [`每 ${state.interval_hours} 小时检查一次`]
  if (state.last_pull_at) {
    parts.push(`上次拉取：${formatDateTime(state.last_pull_at)}`)
  }
  if (state.last_deploy_at) {
    parts.push(`上次部署：${formatDateTime(state.last_deploy_at)}`)
  }
  if (state.last_error) {
    parts.push(`最近错误：${state.last_error}`)
  }
  return parts.join('；')
})

const unmountedTargetServers = computed(() =>
  targetServerStatuses.value.filter((item) => {
    const geoSet = item.ipsets.find((set) => set.name === 'cc_geo_whitelist')
    return geoSet?.exists && !item.mounted
  }),
)

const mountedTargetServers = computed(() => targetServerStatuses.value.filter((item) => item.mounted))

function geoWhitelistEntries(status: FirewallStatus) {
  return status.ipsets.find((set) => set.name === 'cc_geo_whitelist')?.entries ?? 0
}

async function checkTargetServerStatus() {
  if (defaultWhitelistForm.server_ids.length === 0) {
    ElMessage.warning('请先选择目标服务器')
    return
  }
  checkingServerStatus.value = true
  try {
    const results = await Promise.all(
      defaultWhitelistForm.server_ids.map(async (serverId) => {
        const status = await getFirewallStatus(serverId)
        const server = servers.value.find((item) => item.id === serverId)
        return {
          ...status,
          server_name: server?.name ?? `#${serverId}`,
          server_host: server?.host ?? '',
        }
      }),
    )
    targetServerStatuses.value = results
    const mountedCount = results.filter((item) => item.mounted).length
    ElMessage.success(`已检测 ${results.length} 台服务器，${mountedCount} 台规则已挂载`)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '检测目标服务器规则失败')
  } finally {
    checkingServerStatus.value = false
  }
}

async function mountMissingTargetRules() {
  const targets = unmountedTargetServers.value.map((item) => item.server_id)
  if (targets.length === 0) {
    ElMessage.info('没有需要补挂 iptables 的目标服务器')
    return
  }
  mountingServerRules.value = true
  try {
    for (const serverId of targets) {
      await deployServer(serverId)
    }
    ElMessage.success(`已为 ${targets.length} 台服务器补挂 iptables 规则`)
    await checkTargetServerStatus()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '补挂 iptables 失败')
  } finally {
    mountingServerRules.value = false
  }
}

async function stopTargetServerRules() {
  const targets = mountedTargetServers.value.map((item) => item.server_id)
  if (targets.length === 0) {
    ElMessage.info('没有已挂载 iptables 的目标服务器')
    return
  }
  await ElMessageBox.confirm(
    `将从 ${targets.length} 台目标服务器移除 cc-panel iptables 规则（保留 ipset 数据）。停止后流量不再受防护规则约束，确认继续？`,
    '停止目标服务器规则',
    { type: 'warning', confirmButtonText: '确认停止', cancelButtonText: '取消' },
  )
  stoppingServerRules.value = true
  try {
    for (const serverId of targets) {
      await stopServerRules(serverId)
    }
    ElMessage.success(`已停止 ${targets.length} 台服务器的 iptables 规则`)
    await checkTargetServerStatus()
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      ElMessage.error(error instanceof Error ? error.message : '停止 iptables 规则失败')
    }
  } finally {
    stoppingServerRules.value = false
  }
}

async function loadAutoSyncConfig() {
  const cfg = await getDefaultGeoWhitelistAutoSync()
  autoSyncState.value = cfg
  if (cfg.server_ids.length > 0 && defaultWhitelistForm.server_ids.length === 0) {
    defaultWhitelistForm.server_ids = [...cfg.server_ids]
  }
  defaultWhitelistForm.auto_sync = cfg.enabled
}

async function persistAutoSyncConfig() {
  savingAutoSync.value = true
  try {
    autoSyncState.value = await updateDefaultGeoWhitelistAutoSync({
      enabled: defaultWhitelistForm.auto_sync,
      server_ids: defaultWhitelistForm.server_ids,
    })
    ElMessage.success('自动同步设置已保存')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存自动同步设置失败')
  } finally {
    savingAutoSync.value = false
  }
}

async function load() {
  loading.value = true
  try {
    const [summaryItems, ruleItems, serverItems, optionItems] = await Promise.all([
      listGeoCIDRSummaries(),
      listGeoRules(),
      listServers(),
      getGeoOptions(ruleForm.country, ruleForm.province),
    ])
    cidrSummaries.value = summaryItems
    rules.value = ruleItems
    servers.value = serverItems
    geoOptions.value = optionItems
    if (defaultWhitelistCountries.value.length === 0) {
      defaultWhitelistCountries.value = (await getDefaultGeoWhitelist()).countries
    }
    await loadAutoSyncConfig()
  } finally {
    loading.value = false
  }
}

async function submitDefaultWhitelist() {
  if (defaultWhitelistForm.server_ids.length === 0) {
    ElMessage.warning('请先选择目标服务器')
    return
  }
  submittingDefaultWhitelist.value = true
  beginProgress('部署默认白名单', '正在导入 CIDR 并一次性部署到目标服务器...')
  try {
    const rules = await createDefaultGeoWhitelist({
      server_ids: defaultWhitelistForm.server_ids,
      enabled: defaultWhitelistForm.enabled,
      whitelist_mode: defaultWhitelistForm.whitelist_mode,
      cleanup: true,
    })
    finishProgress('部署默认白名单完成')
    ElMessage.success(`默认白名单已创建并部署 ${rules.length} 条规则`)
    await load()
  } catch (error) {
    finishProgress('部署默认白名单失败', 'exception')
    ElMessage.error(error instanceof Error ? error.message : '部署默认白名单失败')
  } finally {
    submittingDefaultWhitelist.value = false
    hideProgressLater()
  }
}

async function syncDefaultWhitelist() {
  if (defaultWhitelistForm.server_ids.length === 0) {
    ElMessage.warning('请先选择目标服务器')
    return
  }
  syncingDefaultWhitelist.value = true
  beginProgress('更新 CIDR 并重新部署', '正在拉取最新 CIDR 并一次性部署到目标服务器...')
  try {
    const results = await syncDefaultGeoWhitelist({
      server_ids: defaultWhitelistForm.server_ids,
      enabled: defaultWhitelistForm.enabled,
      whitelist_mode: defaultWhitelistForm.whitelist_mode,
      cleanup: true,
    })
    finishProgress('更新 CIDR 并重新部署完成')
    const total = results.reduce((sum, item) => sum + item.cidr_count, 0)
    ElMessage.success(`默认白名单已同步 ${results.length} 个地区，共 ${total} 条 CIDR`)
    await load()
  } catch (error) {
    finishProgress('更新 CIDR 并重新部署失败', 'exception')
    ElMessage.error(error instanceof Error ? error.message : '同步默认白名单失败')
  } finally {
    syncingDefaultWhitelist.value = false
    hideProgressLater()
  }
}

async function showCIDRDetails(row: GeoCIDRSummary) {
  cidrDetailVisible.value = true
  cidrDetailLoading.value = true
  cidrDetailTitle.value = `${row.country}${row.province ? ` / ${row.province}` : ''}${row.city ? ` / ${row.city}` : ''} CIDR 明细`
  try {
    cidrDetails.value = await listGeoCIDRs(1000, row.country, row.province || '', row.city || '')
  } finally {
    cidrDetailLoading.value = false
  }
}

async function submitCIDR() {
  submittingCIDR.value = true
  try {
    await addGeoCIDR({
      country: cidrForm.country,
      province: cidrForm.province || undefined,
      city: cidrForm.city || undefined,
      cidr: cidrForm.cidr,
    })
    ElMessage.success('CIDR 已保存')
    cidrForm.cidr = ''
    await load()
  } finally {
    submittingCIDR.value = false
  }
}

async function searchIP() {
  searching.value = true
  try {
    regionInfo.value = await searchGeoIP(searchForm.ip)
  } finally {
    searching.value = false
  }
}

function parseSmartCIDRs() {
  return smartForm.cidrs
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean)
}

async function smartPreview() {
  smartPreviewing.value = true
  try {
    previews.value = await previewGeoCIDRs(parseSmartCIDRs())
  } finally {
    smartPreviewing.value = false
  }
}

async function smartImport() {
  smartImporting.value = true
  try {
    const validCIDRs = previews.value.filter((item) => item.valid).map((item) => item.cidr)
    const result = await bulkAddGeoCIDRs(validCIDRs.length > 0 ? validCIDRs : parseSmartCIDRs())
    ElMessage.success(`已智能导入 ${result.length} 条 CIDR`)
    await load()
    await smartPreview()
  } finally {
    smartImporting.value = false
  }
}

async function submitRule() {
  submittingRule.value = true
  try {
    await createGeoRule({
      name: ruleForm.name || `${ruleForm.country}${ruleForm.action === 'ACCEPT' ? '白名单' : '封禁'}`,
      country: ruleForm.country,
      province: countryOnlyRule.value ? undefined : ruleForm.province || undefined,
      city: countryOnlyRule.value ? undefined : ruleForm.city || undefined,
      action: ruleForm.action,
      server_ids: ruleForm.server_ids,
      enabled: ruleForm.enabled,
    })
    ElMessage.success(`${ruleForm.action === 'ACCEPT' ? '地区白名单' : '地区封禁'}规则已创建并部署`)
    await load()
  } finally {
    submittingRule.value = false
  }
}

onMounted(load)

watch(
  () => ruleForm.country,
  async () => {
    ruleForm.province = ''
    ruleForm.city = ''
    geoOptions.value = await getGeoOptions(ruleForm.country)
  },
)

watch(
  () => ruleForm.province,
  async () => {
    ruleForm.city = ''
    geoOptions.value = await getGeoOptions(ruleForm.country, ruleForm.province)
  },
)
</script>

<template>
  <div>
    <div class="toolbar">
      <div>
        <h2>地区封禁 / 白名单</h2>
        <p class="muted">导入 CIDR 时可自动通过 ip2region 识别地区，再选择封禁或白名单规则写入远程 ipset</p>
      </div>
      <el-button @click="load">刷新</el-button>
    </div>

    <el-row :gutter="18">
      <el-col :span="8">
        <el-card class="page-card" v-loading="searching">
          <template #header>IP 归属地查询</template>
          <el-form label-width="90px">
            <el-form-item label="IP 地址">
              <el-input v-model="searchForm.ip" placeholder="8.8.8.8" @keyup.enter="searchIP" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="searching" @click="searchIP">查询</el-button>
            </el-form-item>
          </el-form>
          <el-descriptions v-if="regionInfo" :column="1" border>
            <el-descriptions-item label="国家/地区">{{ regionInfo.country || '-' }}</el-descriptions-item>
            <el-descriptions-item label="区域">{{ regionInfo.region || '-' }}</el-descriptions-item>
            <el-descriptions-item label="省份">{{ regionInfo.province || '-' }}</el-descriptions-item>
            <el-descriptions-item label="城市">{{ regionInfo.city || '-' }}</el-descriptions-item>
            <el-descriptions-item label="ISP">{{ regionInfo.isp || '-' }}</el-descriptions-item>
            <el-descriptions-item label="ISO">{{ regionInfo.iso_code || '-' }}</el-descriptions-item>
            <el-descriptions-item label="原始结果">{{ regionInfo.raw || '-' }}</el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>

      <el-col :span="8">
        <el-card class="page-card" v-loading="loading">
          <template #header>导入地区 CIDR</template>
          <el-form label-width="100px">
            <el-form-item label="国家/地区">
              <el-input v-model="cidrForm.country" placeholder="可留空，保存时自动识别" />
            </el-form-item>
            <el-form-item label="省份">
              <el-input v-model="cidrForm.province" placeholder="可留空，保存时自动识别" />
            </el-form-item>
            <el-form-item label="城市">
              <el-input v-model="cidrForm.city" placeholder="可留空，保存时自动识别" />
            </el-form-item>
            <el-form-item label="CIDR">
              <el-input v-model="cidrForm.cidr" placeholder="1.2.3.0/24" />
            </el-form-item>
            <el-alert
              class="cidr-tip"
              title="ip2region 用于根据 CIDR 起始 IP 自动识别归属地；它不能反向生成某地区的全部 CIDR 段。"
              type="info"
              :closable="false"
            />
            <el-form-item>
              <el-button type="primary" :loading="submittingCIDR" @click="submitCIDR">保存 CIDR</el-button>
            </el-form-item>
          </el-form>
        </el-card>
      </el-col>

      <el-col :span="8">
        <el-card class="page-card" v-loading="loading">
          <template #header>{{ ruleTitle }}</template>
          <el-form label-width="110px">
            <el-form-item label="规则名称">
              <el-input v-model="ruleForm.name" />
            </el-form-item>
            <el-form-item label="规则动作">
              <el-radio-group v-model="ruleForm.action">
                <el-radio-button label="DROP">地区封禁</el-radio-button>
                <el-radio-button label="ACCEPT">地区白名单</el-radio-button>
              </el-radio-group>
            </el-form-item>
            <el-form-item label="国家/地区">
              <el-select v-model="ruleForm.country" filterable clearable placeholder="选择国家名称" class="wide">
                <el-option v-for="item in geoOptions.countries" :key="item" :label="item" :value="item" />
              </el-select>
            </el-form-item>
            <el-alert
              v-if="countryOnlyRule"
              class="cidr-tip"
              title="地区白名单按国家名称生效：选择国家后系统会自动导入该国家 CIDR，并写入 cc_geo_whitelist。"
              type="success"
              :closable="false"
            />
            <el-form-item v-if="!countryOnlyRule" label="省份">
              <el-select v-model="ruleForm.province" filterable allow-create clearable placeholder="可选" class="wide">
                <el-option v-for="item in geoOptions.provinces" :key="item" :label="item" :value="item" />
              </el-select>
            </el-form-item>
            <el-form-item v-if="!countryOnlyRule" label="城市">
              <el-select v-model="ruleForm.city" filterable allow-create clearable placeholder="可选" class="wide">
                <el-option v-for="item in geoOptions.cities" :key="item" :label="item" :value="item" />
              </el-select>
            </el-form-item>
            <el-form-item label="目标服务器">
              <el-select v-model="ruleForm.server_ids" multiple filterable class="wide">
                <el-option
                  v-for="server in servers"
                  :key="server.id"
                  :label="`${server.name} (${server.host})`"
                  :value="server.id"
                />
              </el-select>
            </el-form-item>
            <el-form-item label="启用">
              <el-switch v-model="ruleForm.enabled" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="submittingRule" @click="submitRule">创建并部署</el-button>
            </el-form-item>
          </el-form>
        </el-card>
      </el-col>
    </el-row>

    <el-card class="page-card table-card">
      <template #header>
        <div class="default-whitelist-header">
          <span>默认地区白名单</span>
          <div class="default-whitelist-header-actions">
            <el-button :loading="checkingServerStatus" @click="checkTargetServerStatus">检测目标服务器规则</el-button>
            <el-button
              v-if="unmountedTargetServers.length > 0"
              type="warning"
              :loading="mountingServerRules"
              @click="mountMissingTargetRules"
            >
              补挂 iptables（{{ unmountedTargetServers.length }}）
            </el-button>
            <el-button
              v-if="mountedTargetServers.length > 0"
              type="danger"
              :loading="stoppingServerRules"
              @click="stopTargetServerRules"
            >
              停止 iptables（{{ mountedTargetServers.length }}）
            </el-button>
          </div>
        </div>
      </template>
      <el-alert
        class="cidr-tip"
        title="这些地区会默认写入 cc_geo_whitelist：柬埔寨、中国内地、中国香港、缅甸、菲律宾、中国台湾、泰国、越南、老挝。"
        type="success"
        :closable="false"
      />
      <el-alert
        v-if="defaultWhitelistForm.whitelist_mode === 'strict_whitelist'"
        class="cidr-tip"
        title="严格白名单模式：仅允许白名单内 IP 访问，其余全部 DROP。请确保你的管理 IP 已在白名单内，否则会无法连接。"
        type="warning"
        :closable="false"
      />
      <el-alert
        v-else-if="defaultWhitelistForm.whitelist_mode === 'connection_count'"
        class="cidr-tip"
        title="连接数封禁模式：非白名单 IP 连接数超阈值时写入 ipset 临时封禁，不会默认 DROP 全部非白名单流量。请在「自动封禁策略」中配置阈值。"
        type="info"
        :closable="false"
      />
      <div class="default-tags">
        <el-tag v-for="country in defaultWhitelistCountries" :key="country" type="success">{{ country }}</el-tag>
      </div>
      <el-form label-width="110px" class="default-form">
        <el-form-item label="目标服务器">
          <el-select v-model="defaultWhitelistForm.server_ids" multiple filterable class="wide">
            <el-option
              v-for="server in servers"
              :key="server.id"
              :label="`${server.name} (${server.host})`"
              :value="server.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="defaultWhitelistForm.enabled" />
        </el-form-item>
        <el-form-item label="防护模式">
          <el-radio-group v-model="defaultWhitelistForm.whitelist_mode">
            <el-radio value="strict_whitelist">严格白名单（推荐）</el-radio>
            <el-radio value="connection_count">连接数封禁</el-radio>
          </el-radio-group>
          <div class="form-tip">两种模式互斥，同一服务器仅生效一种</div>
        </el-form-item>
        <el-form-item label="每日自动同步">
          <el-switch v-model="defaultWhitelistForm.auto_sync" />
          <span class="form-tip">每天拉取 CIDR 入库，有变化才部署到下方目标服务器</span>
        </el-form-item>
        <el-form-item v-if="autoSyncState" label="同步状态">
          <div class="auto-sync-summary">{{ autoSyncSummary }}</div>
        </el-form-item>
        <el-form-item>
          <el-button :loading="savingAutoSync" @click="persistAutoSyncConfig">保存自动同步设置</el-button>
        </el-form-item>
        <el-form-item>
          <el-button type="success" :loading="submittingDefaultWhitelist" @click="submitDefaultWhitelist">部署默认白名单</el-button>
          <el-button type="primary" :loading="syncingDefaultWhitelist" @click="syncDefaultWhitelist">更新 CIDR 并重新部署</el-button>
        </el-form-item>
      </el-form>
      <el-table v-if="targetServerStatuses.length" :data="targetServerStatuses" class="target-status-table" empty-text="暂无检测结果">
        <el-table-column label="服务器" min-width="180">
          <template #default="{ row }">{{ row.server_name }} ({{ row.server_host }})</template>
        </el-table-column>
        <el-table-column label="iptables 挂载" width="120">
          <template #default="{ row }">
            <el-tag :type="row.mounted ? 'success' : 'danger'">{{ row.mounted ? '已挂载' : '未挂载' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="geo 白名单" width="120">
          <template #default="{ row }">{{ geoWhitelistEntries(row) }}</template>
        </el-table-column>
        <el-table-column label="防护模式" width="140">
          <template #default="{ row }">
            <el-tag v-if="row.whitelist_mode === 'strict_whitelist' && row.strict_whitelist_active" type="warning">严格白名单</el-tag>
            <el-tag v-else-if="row.whitelist_mode === 'strict_whitelist'" type="info">严格（未检测到）</el-tag>
            <el-tag v-else-if="row.whitelist_mode === 'connection_count'" type="success">连接数封禁</el-tag>
            <span v-else>-</span>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog
      v-model="defaultWhitelistProgress.visible"
      :title="defaultWhitelistProgress.title"
      width="520px"
      :close-on-click-modal="false"
      :close-on-press-escape="false"
      :show-close="!submittingDefaultWhitelist && !syncingDefaultWhitelist"
    >
      <div class="default-whitelist-progress">
        <div class="progress-label">{{ defaultWhitelistProgress.label }}</div>
        <el-progress
          :percentage="defaultWhitelistProgress.percent"
          :status="defaultWhitelistProgress.status || undefined"
          :stroke-width="18"
          striped
          striped-flow
        />
      </div>
    </el-dialog>

    <el-card class="page-card table-card">
      <template #header>智能批量导入</template>
      <el-row :gutter="18">
        <el-col :span="8">
          <el-input
            v-model="smartForm.cidrs"
            type="textarea"
            :rows="8"
            placeholder="每行一个 CIDR，例如：&#10;8.8.8.0/24&#10;1.1.1.0/24"
          />
          <div class="smart-actions">
            <el-button :loading="smartPreviewing" @click="smartPreview">智能预览</el-button>
            <el-button type="primary" :loading="smartImporting" @click="smartImport">确认导入有效 CIDR</el-button>
          </div>
        </el-col>
        <el-col :span="16">
          <el-table :data="previews" empty-text="暂无预览，请点击智能预览">
            <el-table-column prop="cidr" label="CIDR" width="150" />
            <el-table-column label="识别地区" min-width="220">
              <template #default="{ row }">
                <span v-if="row.region">
                  {{ row.region.country || '-' }} / {{ row.region.province || '-' }} / {{ row.region.city || '-' }}
                </span>
                <el-tag v-else type="danger">{{ row.error || '识别失败' }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="start_ip" label="起始 IP" width="140" />
            <el-table-column label="建议动作" width="120">
              <template #default="{ row }">
                <el-tag v-if="row.valid" :type="row.suggested_action === 'ACCEPT' ? 'success' : 'danger'">
                  {{ row.suggested_action === 'ACCEPT' ? '建议白名单' : '建议封禁' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="reason" label="建议原因" min-width="220" />
          </el-table>
        </el-col>
      </el-row>
    </el-card>

    <el-card class="page-card table-card">
      <template #header>地区 CIDR 汇总</template>
      <el-table :data="cidrSummaries" empty-text="暂无 CIDR 数据">
        <el-table-column prop="country" label="国家/地区" min-width="120" />
        <el-table-column prop="province" label="省份" min-width="120" />
        <el-table-column prop="city" label="城市" min-width="120" />
        <el-table-column prop="cidr_count" label="CIDR 数量" width="110" />
        <el-table-column label="白名单规则" width="110">
          <template #default="{ row }">
            <el-tag :type="row.whitelist_rule_count > 0 ? 'success' : 'info'">{{ row.whitelist_rule_count }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="封禁规则" width="100">
          <template #default="{ row }">
            <el-tag :type="row.block_rule_count > 0 ? 'danger' : 'info'">{{ row.block_rule_count }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="latest_cidr_at" label="最近导入" width="220" />
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button link type="primary" @click="showCIDRDetails(row)">查看明细</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="cidrDetailVisible" :title="cidrDetailTitle" width="760px">
      <el-table v-loading="cidrDetailLoading" :data="cidrDetails" max-height="520" empty-text="暂无明细">
        <el-table-column prop="country" label="国家/地区" width="120" />
        <el-table-column prop="province" label="省份" width="120" />
        <el-table-column prop="city" label="城市" width="120" />
        <el-table-column prop="cidr" label="CIDR" />
        <el-table-column label="创建时间" width="180">
          <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <el-card class="page-card table-card">
      <template #header>地区规则</template>
      <el-table :data="rules" empty-text="暂无规则">
        <el-table-column prop="name" label="名称" />
        <el-table-column prop="action" label="动作" width="100">
          <template #default="{ row }">
            <el-tag :type="row.action === 'ACCEPT' ? 'success' : 'danger'">
              {{ row.action === 'ACCEPT' ? '白名单' : '封禁' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="country" label="国家/地区" width="120" />
        <el-table-column prop="province" label="省份" width="120" />
        <el-table-column prop="city" label="城市" width="120" />
        <el-table-column prop="enabled" label="启用" width="100">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '是' : '否' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="180">
          <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<style scoped>
h2 {
  margin: 0;
}

.wide {
  width: 100%;
}

.table-card {
  margin-top: 18px;
}

.cidr-tip {
  margin-bottom: 16px;
}

.smart-actions {
  display: flex;
  gap: 10px;
  margin-top: 12px;
}

.default-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 16px;
}

.default-form {
  max-width: 620px;
}

.default-whitelist-progress {
  width: 100%;
}

.progress-label {
  margin-bottom: 8px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.form-tip {
  margin-left: 12px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.auto-sync-summary {
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.6;
}

.target-status-table {
  margin-top: 16px;
}

.default-whitelist-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.default-whitelist-header-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
</style>
