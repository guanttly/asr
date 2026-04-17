import { invoke } from '@tauri-apps/api/core'
import { useAppStore } from '@/stores/app'
import { buildServerCandidates, describeNetworkError, normalizeServerUrl } from './server'

interface ApiEnvelope<T> {
  code: number
  message: string
  data?: T
}

export interface AuthUser {
  id: number
  username: string
  display_name: string
  role: string
}

export interface MachineIdentity {
  machine_code: string
  hostname: string
  platform: string
  ip_addresses: string[]
  mac_addresses: string[]
}

interface AnonymousLoginPayload {
  token: string
  expires_in: number
  user?: AuthUser
}

let anonymousLoginPromise: Promise<AuthUser> | null = null

function mergeHeaders(headers?: HeadersInit, extra?: Record<string, string>) {
  const merged = new Headers(headers)
  if (extra) {
    Object.entries(extra).forEach(([key, value]) => {
      merged.set(key, value)
    })
  }
  return merged
}

async function fetchWithServerFallback(path: string, init?: RequestInit) {
  const appStore = useAppStore()
  const candidates = buildServerCandidates(appStore.serverUrl)
  let lastError: unknown = null

  for (const candidate of candidates) {
    try {
      const response = await fetch(`${candidate}${path}`, init)
      if (normalizeServerUrl(appStore.serverUrl) !== candidate) {
        appStore.serverUrl = candidate
        appStore.persist()
      }
      return response
    }
    catch (error) {
      lastError = error
    }
  }

  throw new Error(describeNetworkError(lastError, candidates))
}

export async function readResponseEnvelope<T>(response: Response) {
  return await response.json().catch(() => ({
    code: response.ok ? 0 : -1,
    message: response.ok ? 'ok' : '服务器返回了无法解析的响应',
  })) as ApiEnvelope<T>
}

export async function getMachineIdentity() {
  const appStore = useAppStore()
  const identity = await invoke<MachineIdentity>('get_machine_identity')
  appStore.machineCode = identity.machine_code
  return identity
}

function applyUser(user?: AuthUser | null) {
  const appStore = useAppStore()
  appStore.applyAuthenticatedUser(user)
  if (user?.display_name?.trim())
    appStore.deviceAlias = user.display_name.trim()
  appStore.persist()
}

function snapshotUser(appStore = useAppStore()): AuthUser {
  return {
    id: 0,
    username: appStore.username,
    display_name: appStore.displayName,
    role: appStore.role,
  }
}

export async function ensureAnonymousLogin(force = false): Promise<AuthUser> {
  const appStore = useAppStore()
  if (!force && appStore.token.trim() && appStore.username.trim())
    return snapshotUser(appStore)

  if (anonymousLoginPromise && !force)
    return anonymousLoginPromise

  anonymousLoginPromise = (async () => {
    appStore.serverUrl = normalizeServerUrl(appStore.serverUrl)
    const identity = await getMachineIdentity()
    const response = await fetchWithServerFallback('/api/admin/auth/anonymous-login', {
      method: 'POST',
      headers: mergeHeaders(undefined, { 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        machine_code: identity.machine_code,
        display_name: appStore.deviceAlias.trim(),
        hostname: identity.hostname,
        platform: identity.platform,
        ip_addresses: identity.ip_addresses,
        mac_addresses: identity.mac_addresses,
      }),
    })

    const payload = await readResponseEnvelope<AnonymousLoginPayload>(response)
    if (!response.ok || !payload.data?.token)
      throw new Error(payload.message || '匿名登录失败')

    appStore.token = payload.data.token
    applyUser(payload.data.user)
    appStore.persist()
    return payload.data.user || snapshotUser(appStore)
  })()

  try {
    return await anonymousLoginPromise
  }
  finally {
    anonymousLoginPromise = null
  }
}

export async function authedFetch(path: string, init?: RequestInit, retry = true) {
  const appStore = useAppStore()
  if (!appStore.token.trim())
    await ensureAnonymousLogin()

  const response = await fetchWithServerFallback(path, {
    ...init,
    headers: mergeHeaders(init?.headers, {
      Authorization: `Bearer ${appStore.token.trim()}`,
    }),
  })

  if (response.status === 401 && retry) {
    appStore.clearAuth()
    appStore.persist()
    await ensureAnonymousLogin(true)
    return authedFetch(path, init, false)
  }

  return response
}

export async function getCurrentUser() {
  const response = await authedFetch('/api/admin/me')
  const payload = await readResponseEnvelope<AuthUser>(response)
  if (!response.ok || !payload.data)
    throw new Error(payload.message || '获取当前用户失败')
  applyUser(payload.data)
  return payload.data
}

export async function updateProfile(displayName: string) {
  const response = await authedFetch('/api/admin/me/profile', {
    method: 'PUT',
    headers: mergeHeaders(undefined, { 'Content-Type': 'application/json' }),
    body: JSON.stringify({ display_name: displayName.trim() }),
  })
  const payload = await readResponseEnvelope<AuthUser>(response)
  if (!response.ok || !payload.data)
    throw new Error(payload.message || '保存别名失败')
  applyUser(payload.data)
  return payload.data
}

export async function pingServer() {
  const response = await fetchWithServerFallback('/healthz')
  if (!response.ok)
    throw new Error(`服务健康检查失败: ${response.status}`)
  return response
}