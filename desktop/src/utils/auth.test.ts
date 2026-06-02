import { invoke } from '@tauri-apps/api/core'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useAppStore } from '@/stores/app'
import { authedFetch, ensureAnonymousLogin, ensureMeetingWorkflowBinding } from './auth'

vi.mock('@tauri-apps/api/core', () => ({
  invoke: vi.fn(),
}))

const invokeMock = vi.mocked(invoke)

const machineIdentity = {
  machine_code: 'machine-001',
  hostname: 'desktop-host',
  platform: 'windows',
  ip_addresses: ['192.168.1.2'],
  mac_addresses: ['00:11:22:33:44:55'],
}

function envelope(data: unknown, status = 200, message = 'ok') {
  return new Response(JSON.stringify({ code: status < 400 ? 0 : -1, message, data }), { status })
}

function loginPayload(token: string, displayName = '云端设备') {
  return {
    token,
    expires_in: 3600,
    user: {
      id: 1,
      username: 'anon-machine-001',
      display_name: displayName,
      role: 'user',
    },
  }
}

describe('desktop anonymous auth flow', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    invokeMock.mockResolvedValue(machineIdentity)
    vi.stubGlobal('fetch', vi.fn())
  })

  it('logs in anonymously with the local machine identity and stores JWT state', async () => {
    const fetchMock = vi.mocked(fetch)
    fetchMock.mockResolvedValue(envelope(loginPayload('jwt-1')))

    const appStore = useAppStore()
    appStore.serverUrl = 'localhost:8080'
    appStore.deviceAlias = '测试设备-001'

    const user = await ensureAnonymousLogin(true)

    expect(user.username).toBe('anon-machine-001')
    expect(appStore.token).toBe('jwt-1')
    expect(appStore.machineCode).toBe('machine-001')
    expect(appStore.deviceAlias).toBe('云端设备')

    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('http://localhost:8080/api/admin/auth/anonymous-login')
    expect(JSON.parse(String(init?.body))).toMatchObject({
      machine_code: 'machine-001',
      display_name: '测试设备-001',
      hostname: 'desktop-host',
    })
  })

  it('clears an expired token, relogs in, and retries the original request with the new JWT', async () => {
    const fetchMock = vi.mocked(fetch)
    fetchMock
      .mockResolvedValueOnce(envelope(null, 401, 'token expired'))
      .mockResolvedValueOnce(envelope(loginPayload('jwt-2', '重登设备')))
      .mockResolvedValueOnce(envelope({ ok: true }))

    const appStore = useAppStore()
    appStore.serverUrl = 'http://127.0.0.1:10010'
    appStore.token = 'jwt-old'
    appStore.username = 'anon-old'
    appStore.deviceAlias = '本地设备'

    const response = await authedFetch('/api/admin/me')

    expect(response.status).toBe(200)
    expect(fetchMock).toHaveBeenCalledTimes(3)
    expect(new Headers(fetchMock.mock.calls[0][1]?.headers).get('Authorization')).toBe('Bearer jwt-old')
    expect(fetchMock.mock.calls[1][0]).toBe('http://127.0.0.1:10010/api/admin/auth/anonymous-login')
    expect(new Headers(fetchMock.mock.calls[2][1]?.headers).get('Authorization')).toBe('Bearer jwt-2')
    expect(appStore.token).toBe('jwt-2')
    expect(appStore.username).toBe('anon-machine-001')
  })

  it('loads the meeting workflow binding after product features are known', async () => {
    const fetchMock = vi.mocked(fetch)
    fetchMock
      .mockResolvedValueOnce(envelope({
        edition: 'advanced',
        capabilities: {
          realtime: true,
          batch: true,
          meeting: true,
          voiceprint: false,
          voice_control: false,
        },
      }))
      .mockResolvedValueOnce(envelope({
        realtime: 11,
        meeting: 33,
        voice_control: null,
      }))

    const appStore = useAppStore()
    appStore.serverUrl = 'http://127.0.0.1:10010'
    appStore.token = 'jwt-current'
    appStore.username = 'anon-current'

    const workflowId = await ensureMeetingWorkflowBinding(true)

    expect(workflowId).toBe(33)
    expect(appStore.meetingWorkflowId).toBe(33)
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      'http://127.0.0.1:10010/api/admin/app-settings/product-features',
      'http://127.0.0.1:10010/api/admin/me/workflow-bindings',
    ])
  })
})
