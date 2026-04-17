import { invoke } from '@tauri-apps/api/core'
import { SETTINGS_STORAGE_KEY } from '@/stores/app'

interface PersistedDebugState {
  debugLoggingEnabled?: boolean
}

function safeSerialize(payload: unknown) {
  if (payload == null)
    return ''
  if (typeof payload === 'string')
    return payload

  try {
    return JSON.stringify(payload)
  }
  catch {
    return String(payload)
  }
}

function isDebugEnabled() {
  try {
    const raw = localStorage.getItem(SETTINGS_STORAGE_KEY)
    if (!raw)
      return false
    return (JSON.parse(raw) as PersistedDebugState).debugLoggingEnabled === true
  }
  catch {
    return false
  }
}

export async function appendRuntimeLog(scope: string, message: string) {
  await invoke('append_runtime_log', { scope, message }).catch(() => undefined)
}

export async function debugLog(scope: string, message: string, payload?: unknown) {
  const detail = safeSerialize(payload)
  const line = detail ? `${message} | ${detail}` : message
  console.info(`[${scope}] ${line}`)

  if (!isDebugEnabled())
    return

  await appendRuntimeLog(scope, line)
}

export async function readRuntimeLogTail(lines = 120) {
  return await invoke<string>('read_runtime_log_tail', { lines })
}

export async function getRuntimeLogPath() {
  return await invoke<string>('get_runtime_log_path')
}