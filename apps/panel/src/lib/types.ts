export interface Session {
  authenticated: boolean
  role: string
  local: boolean
  label: string
}

export interface WpSettings {
  adminUrl: string
  adminUser: string
  adminPassword: string
  consumerKey: string
  consumerSecret: string
  exportId: string
  exportCronKey: string
}

export interface Settings {
  domain: string
  tunnelHostname: string
  tunnelToken?: string
  catalogPath: string
  panelPort: number
  debug: boolean
  syncMinutes?: number | null
  syncCreatedDays?: number | null
  autoUpdate?: boolean | null
  wp: WpSettings
  webhookSecret: string
  access: { enabled: boolean }
}

export interface Diagnostics {
  version: string
  uptime: string
  dataDir: string
  catalog: string
  panelPort: number
  debug: boolean
  logs: string
  logPath?: string
}

export type UpdateStatus = 'idle' | 'checking' | 'up-to-date' | 'available' | 'downloading' | 'applying' | 'updated' | 'error'

export interface UpdateState {
  current: string
  latest?: string
  available: boolean
  status: UpdateStatus
  error?: string
  checkedAt?: string
  updatedAt?: string
}

export interface TunnelInfo {
  hostname: string
  url: string
  service: string
  status: string
  detail: string
  certPresent: boolean
  token?: boolean
}

export interface Invite {
  id: string
  label: string
  createdAt: string
  expiresAt: string
  usedAt?: string
}

export interface SessionRow {
  id: string
  label: string
  role: string
  createdAt: string
  lastSeen: string
  expiresAt: string
}

export interface PanelEntrySource {
  orderNumber?: string
  productText: string
  source?: string
  quantity: number
  /** Delivery day, set only by the combined several-days list. */
  date?: string
}

export interface PanelEntry {
  unitId: string
  name: string
  measure: string
  category: string
  note: string
  quantity: number
  references?: string[]
  sources?: PanelEntrySource[]
}

export interface PanelUnresolved {
  productText: string
  quantity: number
  count: number
  reason: string
  detail?: string
  references?: string[]
}

export interface PanelList {
  entries: PanelEntry[]
  unresolved: number
  total: number
  unresolvedItems?: PanelUnresolved[]
}

export interface OrderFacet {
  kind: string
  value: string
}

export interface PanelOrder {
  number: string
  text: string
  units: number
  status: string
  note?: string
  color?: string
  reason?: string
  facets?: OrderFacet[]
}

export interface LineUnit {
  unitId: string
  name: string
  quantity: number
}

export interface PanelOrderLine {
  productText: string
  quantity: number
  options: string[]
  source: string
  status: string
  detail?: string
  units: LineUnit[]
}

export interface PanelOrderDetail {
  number: string
  status: string
  note?: string
  color?: string
  reason?: string
  units: number
  lines: PanelOrderLine[]
  entries: PanelEntry[]
}

export interface Activity {
  time: string
  kind: string
  text: string
}

export interface NextSend {
  time: string
  targets: number
}

export interface PanelDay {
  date: string
  orders: PanelOrder[]
  list: PanelList
}

export interface WhatsAppState {
  status: string
  label: string
  linked: boolean
  phone: string
  allowlist: string[]
}

export interface Notifications {
  enabled: boolean
  new: boolean
  updated: boolean
  today: boolean
  tomorrow: boolean
}

export interface Snapshot {
  whatsapp: WhatsAppState
  notifications: Notifications
  days: PanelDay[]
  activity: Activity[]
}

export interface PanelState {
  ok: boolean
  role: string
  local: boolean
  tunnel: TunnelInfo
  snapshot: Snapshot
  nextSend?: NextSend
}

export interface TargetSchedule {
  mode: 'immediate' | 'times'
  times: string[]
  timezone?: string
  quietHours?: string[]
}

export interface Target {
  id: string
  label: string
  phone: string
  botId?: string
  kinds: string[]
  schedule: TargetSchedule
  enabled: boolean
}

export interface Bot {
  id: string
  name: string
  phone?: string
  enabled: boolean
  loopback?: boolean
  allowlist?: string[]
  status: string
}

export type CheckStatus = 'ok' | 'warn' | 'fail' | 'skipped' | ''

export interface CheckResult {
  id: string
  label: string
  status: CheckStatus
  detail: string
  durationMs: number
  checkedAt: string
}
