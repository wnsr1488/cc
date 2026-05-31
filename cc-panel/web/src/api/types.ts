import type { CurrentUser } from '@/stores/auth'

export interface LoginResponse {
  token: string
  user: CurrentUser
}

export type WhitelistMode = 'off' | 'strict_whitelist' | 'connection_count'

export interface ServerAsset {
  id: number
  name: string
  host: string
  port: number
  username: string
  auth_type: 'password' | 'private_key'
  group_name?: string
  os_info?: string
  kernel_version?: string
  status: string
  whitelist_mode?: WhitelistMode
  strict_whitelist?: boolean
  last_seen_at?: string
  created_at: string
  updated_at: string
}

export interface ServerPayload {
  name: string
  host: string
  port: number
  username: string
  auth_type: 'password' | 'private_key'
  password?: string
  private_key?: string
  group_name?: string
  whitelist_mode?: WhitelistMode
  strict_whitelist?: boolean
}

export interface FirewallStatus {
  server_id: number
  ipsets: Array<{
    name: string
    exists: boolean
    entries: number
  }>
  iptables_rules: string[]
  iptables_counts: string[]
  mounted: boolean
  whitelist_mode?: WhitelistMode
  strict_whitelist?: boolean
  strict_whitelist_active?: boolean
}

export interface FirewallPayload {
  server_ids: number[]
  ip: string
  timeout: number
  reason?: string
}

export interface FirewallBulkPayload {
  server_ids: number[]
  ips: string[]
  timeout: number
  reason?: string
}

export interface FirewallBulkResult {
  added: number
  skipped: number
  entries: FirewallEntry[]
}

export interface FirewallEntry {
  id: number
  server_id: number
  server_name?: string
  set_name: string
  ip: string
  timeout_seconds: number
  reason?: string
  created_by?: number
  created_at: string
}

export interface AuditLog {
  id: number
  actor_user_id?: number
  action: string
  target_type: string
  target_id?: number
  detail: Record<string, unknown>
  remote_addr: string
  created_at: string
}

export interface AuditLogPage {
  items: AuditLog[]
  total: number
  page: number
  page_size: number
}

export interface GeoCIDR {
  id: number
  country: string
  province?: string
  city?: string
  cidr: string
  created_at: string
}

export interface GeoCIDRSummary {
  country: string
  province?: string
  city?: string
  cidr_count: number
  whitelist_rule_count: number
  block_rule_count: number
  latest_cidr_at?: string
}

export interface GeoCIDRPreview {
  cidr: string
  start_ip?: string
  region?: RegionInfo
  suggested_action?: 'DROP' | 'ACCEPT'
  reason?: string
  valid: boolean
  error?: string
}

export interface GeoRule {
  id: number
  name: string
  country: string
  province?: string
  city?: string
  action: string
  enabled: boolean
  created_by?: number
  created_at: string
  updated_at: string
}

export interface GeoRulePayload {
  name: string
  country: string
  province?: string
  city?: string
  action: 'DROP' | 'ACCEPT'
  server_ids: number[]
  enabled: boolean
}

export interface DefaultGeoWhitelistPayload {
  server_ids: number[]
  enabled: boolean
  country?: string
  cleanup?: boolean
  phase?: 'cidr' | 'deploy' | 'rule'
  server_id?: number
  whitelist_mode?: WhitelistMode
  strict_whitelist?: boolean
}

export interface GeoAutoSyncConfig {
  enabled: boolean
  server_ids: number[]
  interval_hours: number
  last_pull_at?: string
  last_deploy_at?: string
  last_changed: boolean
  last_error?: string
}

export interface GeoAutoSyncUpdatePayload {
  enabled?: boolean
  server_ids?: number[]
}

export interface GeoCountrySyncResult {
  country: string
  cidr_count: number
  rule_id?: number
  error?: string
}

export interface GeoOptions {
  countries: string[]
  provinces: string[]
  cities: string[]
}

export interface RegionInfo {
  ip: string
  country: string
  region: string
  province: string
  city: string
  isp: string
  iso_code: string
  raw: string
}

export interface AutoPolicy {
  id: number
  name: string
  metric: 'request_rate' | 'connection_count' | 'backend_path'
  threshold: number
  window_seconds: number
  block_seconds: number
  target_set: 'cc_rate_block' | 'cc_temp_block'
  enabled: boolean
  created_by?: number
  created_at: string
  updated_at: string
}

export type AutoPolicyPayload = Omit<AutoPolicy, 'id' | 'created_by' | 'created_at' | 'updated_at'>

export interface ServerMetric {
  id: number
  server_id: number
  cpu_usage: number
  memory_usage: number
  disk_usage: number
  load1: number
  load5: number
  load15: number
  net_in_bytes: number
  net_out_bytes: number
  tcp_established: number
  tcp_time_wait: number
  blocked_ip_count: number
  iptables_drop_hits: number
  created_at: string
}

export interface ServerMetricOverview {
  server_id: number
  server_name: string
  server_host: string
  server_status: string
  metric_id?: number
  cpu_usage?: number
  memory_usage?: number
  disk_usage?: number
  load1?: number
  load5?: number
  load15?: number
  net_in_bytes?: number
  net_out_bytes?: number
  tcp_established?: number
  tcp_time_wait?: number
  blocked_ip_count?: number
  iptables_drop_hits?: number
  collected_at?: string
}

export interface CollectAllMetricsResult {
  collected: number
  failed: number
  errors?: Record<string, string>
}

export interface LiveBlockedIP {
  ip: string
  set_name: string
  timeout_seconds?: number
  country?: string
  province?: string
  city?: string
  isp?: string
}

export interface LiveIPConnection {
  ip: string
  count: number
  country?: string
  province?: string
  city?: string
  isp?: string
}

export interface ServerLiveInsights {
  server_id: number
  tcp_established: number
  blocked_ips: LiveBlockedIP[]
  connections: LiveIPConnection[]
  collected_at: string
}

export interface AutoPolicyEvent {
  id: number
  policy_id: number
  server_id: number
  metric: string
  observed_value: number
  threshold: number
  action: string
  detail: Record<string, unknown>
  created_at: string
}

export interface AutoPolicyEventPage {
  items: AutoPolicyEvent[]
  total: number
  page: number
  page_size: number
}
