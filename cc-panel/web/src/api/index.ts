import { jsonBody, request } from './client'
import type {
  AuditLog,
  AuditLogPage,
  AutoPolicy,
  AutoPolicyEvent,
  AutoPolicyEventPage,
  AutoPolicyPayload,
  FirewallEntry,
  FirewallPayload,
  FirewallBulkPayload,
  FirewallBulkResult,
  FirewallStatus,
  DefaultGeoWhitelistPayload,
  GeoCIDR,
  GeoCIDRPreview,
  GeoCIDRSummary,
  GeoCountrySyncResult,
  GeoOptions,
  GeoRule,
  GeoRulePayload,
  LoginResponse,
  RegionInfo,
  ServerAsset,
  ServerMetric,
  ServerMetricOverview,
  ServerLiveInsights,
  CollectAllMetricsResult,
  ServerPayload,
} from './types'

export function login(username: string, password: string) {
  return request<LoginResponse>('/api/v1/auth/login', {
    method: 'POST',
    ...jsonBody({ username, password }),
  })
}

export function getMe() {
  return request('/api/v1/me')
}

export function listServers() {
  return request<ServerAsset[]>('/api/v1/servers')
}

export function getServer(id: number) {
  return request<ServerAsset>(`/api/v1/servers/${id}`)
}

export function createServer(payload: ServerPayload) {
  return request<ServerAsset>('/api/v1/servers', {
    method: 'POST',
    ...jsonBody(payload),
  })
}

export function updateServer(id: number, payload: ServerPayload) {
  return request<ServerAsset>(`/api/v1/servers/${id}`, {
    method: 'PUT',
    ...jsonBody(payload),
  })
}

export function deleteServer(id: number) {
  return request<void>(`/api/v1/servers/${id}`, { method: 'DELETE' })
}

export function testSSH(id: number) {
  return request<{ status: string }>(`/api/v1/servers/${id}/test-ssh`, { method: 'POST' })
}

export function deployServer(id: number) {
  return request<{ status: string }>(`/api/v1/servers/${id}/deploy`, { method: 'POST' })
}

export function stopServerRules(id: number) {
  return request<{ status: string }>(`/api/v1/servers/${id}/stop-rules`, { method: 'POST' })
}

export function getFirewallStatus(id: number) {
  return request<FirewallStatus>(`/api/v1/servers/${id}/firewall/status`)
}

export function rollbackServer(id: number) {
  return request<{ status: string; snapshot_id: number }>(`/api/v1/servers/${id}/rollback`, { method: 'POST' })
}

export function addBlacklist(payload: FirewallPayload) {
  return request<FirewallEntry[]>('/api/v1/firewall/blacklist', {
    method: 'POST',
    ...jsonBody(payload),
  })
}

export function listBlacklist(limit = 5000) {
  return request<FirewallEntry[]>(`/api/v1/firewall/blacklist?limit=${limit}`)
}

export function addWhitelist(payload: FirewallPayload) {
  return request<FirewallEntry[]>('/api/v1/firewall/whitelist', {
    method: 'POST',
    ...jsonBody(payload),
  })
}

export function bulkAddWhitelist(payload: FirewallBulkPayload) {
  return request<FirewallBulkResult>('/api/v1/firewall/whitelist/bulk', {
    method: 'POST',
    ...jsonBody(payload),
  })
}

export function bulkAddBlacklist(payload: FirewallBulkPayload) {
  return request<FirewallBulkResult>('/api/v1/firewall/blacklist/bulk', {
    method: 'POST',
    ...jsonBody(payload),
  })
}

export function listWhitelist(limit = 5000) {
  return request<FirewallEntry[]>(`/api/v1/firewall/whitelist?limit=${limit}`)
}

export function deleteBlacklist(id: number) {
  return request<void>(`/api/v1/firewall/blacklist/${id}`, { method: 'DELETE' })
}

export function deleteWhitelist(id: number) {
  return request<void>(`/api/v1/firewall/whitelist/${id}`, { method: 'DELETE' })
}

export function listAuditLogs(page = 1, pageSize = 20) {
  return request<AuditLogPage>(`/api/v1/audit/logs?page=${page}&page_size=${pageSize}`)
}

export function listGeoCIDRs(limit = 200, country = '', province = '', city = '') {
  const params = new URLSearchParams({ limit: String(limit) })
  if (country) params.set('country', country)
  if (province) params.set('province', province)
  if (city) params.set('city', city)
  return request<GeoCIDR[]>(`/api/v1/geo/cidrs?${params.toString()}`)
}

export function listGeoCIDRSummaries() {
  return request<GeoCIDRSummary[]>('/api/v1/geo/cidrs/summary')
}

export function addGeoCIDR(payload: Omit<GeoCIDR, 'id' | 'created_at'>) {
  return request<GeoCIDR>('/api/v1/geo/cidrs', {
    method: 'POST',
    ...jsonBody(payload),
  })
}

export function previewGeoCIDRs(cidrs: string[]) {
  return request<GeoCIDRPreview[]>('/api/v1/geo/cidrs/preview', {
    method: 'POST',
    ...jsonBody({ cidrs }),
  })
}

export function bulkAddGeoCIDRs(cidrs: string[]) {
  return request<GeoCIDR[]>('/api/v1/geo/cidrs/bulk', {
    method: 'POST',
    ...jsonBody({ cidrs }),
  })
}

export function listGeoRules(limit = 100) {
  return request<GeoRule[]>(`/api/v1/geo/block?limit=${limit}`)
}

export function getGeoOptions(country = '', province = '') {
  const params = new URLSearchParams()
  if (country) params.set('country', country)
  if (province) params.set('province', province)
  const query = params.toString()
  return request<GeoOptions>(`/api/v1/geo/options${query ? `?${query}` : ''}`)
}

export function searchGeoIP(ip: string) {
  return request<RegionInfo>(`/api/v1/geo/search?ip=${encodeURIComponent(ip)}`)
}

export function getDefaultGeoWhitelist() {
  return request<{ countries: string[] }>('/api/v1/geo/default-whitelist')
}

export function createDefaultGeoWhitelist(payload: DefaultGeoWhitelistPayload) {
  return request<GeoRule[]>('/api/v1/geo/default-whitelist', {
    method: 'POST',
    ...jsonBody(payload),
  })
}

export function syncDefaultGeoWhitelist(payload: DefaultGeoWhitelistPayload) {
  return request<GeoCountrySyncResult[]>('/api/v1/geo/default-whitelist/sync', {
    method: 'POST',
    ...jsonBody(payload),
  })
}

export function getDefaultGeoWhitelistAutoSync() {
  return request<GeoAutoSyncConfig>('/api/v1/geo/default-whitelist/auto-sync')
}

export function updateDefaultGeoWhitelistAutoSync(payload: GeoAutoSyncUpdatePayload) {
  return request<GeoAutoSyncConfig>('/api/v1/geo/default-whitelist/auto-sync', {
    method: 'PUT',
    ...jsonBody(payload),
  })
}

export function createGeoRule(payload: GeoRulePayload) {
  return request<GeoRule>('/api/v1/geo/block', {
    method: 'POST',
    ...jsonBody(payload),
  })
}

export function listPolicies(limit = 100) {
  return request<AutoPolicy[]>(`/api/v1/policies?limit=${limit}`)
}

export function createPolicy(payload: AutoPolicyPayload) {
  return request<AutoPolicy>('/api/v1/policies', {
    method: 'POST',
    ...jsonBody(payload),
  })
}

export function updatePolicy(id: number, payload: AutoPolicyPayload) {
  return request<AutoPolicy>(`/api/v1/policies/${id}`, {
    method: 'PUT',
    ...jsonBody(payload),
  })
}

export function deletePolicy(id: number) {
  return request<void>(`/api/v1/policies/${id}`, { method: 'DELETE' })
}

export function listPolicyEvents(page = 1, pageSize = 20) {
  return request<AutoPolicyEventPage>(`/api/v1/policy-events?page=${page}&page_size=${pageSize}`)
}

export function executePolicies(serverID: number) {
  return request<AutoPolicyEvent[]>(`/api/v1/servers/${serverID}/policies/execute`, { method: 'POST' })
}

export function executeAllPolicies() {
  return request<{ events: number }>('/api/v1/policies/execute-all', { method: 'POST' })
}

export function listServerMetrics(serverID: number, limit = 100) {
  return request<ServerMetric[]>(`/api/v1/servers/${serverID}/metrics?limit=${limit}`)
}

export function collectServerMetrics(serverID: number) {
  return request<ServerMetric>(`/api/v1/servers/${serverID}/metrics/collect`, { method: 'POST' })
}

export function listMetricsOverview() {
  return request<ServerMetricOverview[]>('/api/v1/metrics/overview')
}

export function collectAllMetrics() {
  return request<CollectAllMetricsResult>('/api/v1/metrics/collect-all', { method: 'POST' })
}

export function getServerLiveInsights(serverID: number, limit = 100) {
  return request<ServerLiveInsights>(`/api/v1/servers/${serverID}/metrics/live?limit=${limit}`)
}
