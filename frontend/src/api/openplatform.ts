import request from './request'

export interface OpenPlatformCapability {
  id: string
  display_name: string
  description: string
}

export type OpenPlatformAppStatus = 'active' | 'disabled' | 'revoked'

export interface OpenPlatformApp {
  id: number
  app_id: string
  name: string
  description?: string
  secret_hint?: string
  secret_version: number
  status: OpenPlatformAppStatus
  rate_limit_per_sec: number
  allowed_caps: string[]
  default_workflows?: Record<string, number>
  callback_whitelist?: string[]
  created_at?: string
  updated_at?: string
}

export interface OpenPlatformCreateAppPayload {
  name: string
  description?: string
  allowed_caps: string[]
  default_workflows?: Record<string, number>
  callback_whitelist?: string[]
  rate_limit_per_sec?: number
  meta_json?: string
}

export interface OpenPlatformCreateAppResponse extends OpenPlatformApp {
  app_secret: string
}

export interface OpenPlatformCallLog {
  id: number
  request_id: string
  capability: string
  route: string
  http_status: number
  err_code?: string
  latency_ms: number
  ip?: string
  user_agent?: string
  body_ref?: string
  created_at?: string
}

export function getOpenPlatformCapabilities() {
  return request.get('/api/admin/openplatform/capabilities')
}

export function getOpenPlatformApps(params?: { offset?: number, limit?: number }) {
  return request.get('/api/admin/openplatform/apps', { params })
}

export function createOpenPlatformApp(payload: OpenPlatformCreateAppPayload) {
  return request.post('/api/admin/openplatform/apps', payload)
}

export function updateOpenPlatformApp(id: number | string, payload: OpenPlatformCreateAppPayload) {
  return request.put(`/api/admin/openplatform/apps/${id}`, payload)
}

export function rotateOpenPlatformAppSecret(id: number | string) {
  return request.post(`/api/admin/openplatform/apps/${id}/rotate-secret`)
}

export function disableOpenPlatformApp(id: number | string) {
  return request.post(`/api/admin/openplatform/apps/${id}/disable`)
}

export function enableOpenPlatformApp(id: number | string) {
  return request.post(`/api/admin/openplatform/apps/${id}/enable`)
}

export function revokeOpenPlatformApp(id: number | string) {
  return request.delete(`/api/admin/openplatform/apps/${id}`)
}

export function getOpenPlatformAppCalls(id: number | string, params?: { limit?: number }) {
  return request.get(`/api/admin/openplatform/apps/${id}/calls`, { params })
}