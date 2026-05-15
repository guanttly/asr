import { invoke } from '@tauri-apps/api/core'

export interface InputBridgeTargetView {
  targetId: string
  displayName: string
  status: string
  processName?: string
  topTitle?: string
  controlClassName?: string
  appType?: string
  lastUsedAt?: number
  useCount?: number
}

export interface InputBridgeStateView {
  supported: boolean
  state: string
  lockedTarget?: InputBridgeTargetView | null
  candidateTarget?: InputBridgeTargetView | null
  history: InputBridgeTargetView[]
  message: string
}

export interface InputBridgeCommandResult {
  success: boolean
  message: string
  targetId?: string
  displayName?: string
  state?: string
}

export function useInputBridge() {
  async function getState() {
    return await invoke<InputBridgeStateView>('input_bridge_get_state')
  }

  async function lockCurrent() {
    return await invoke<InputBridgeCommandResult>('input_bridge_lock_current')
  }

  async function unlock() {
    return await invoke<InputBridgeCommandResult>('input_bridge_unlock')
  }

  async function useHistory(targetId: string) {
    return await invoke<InputBridgeCommandResult>('input_bridge_use_history', { targetId })
  }

  async function deleteHistory(targetId: string) {
    return await invoke<InputBridgeCommandResult>('input_bridge_delete_history', { targetId })
  }

  async function flashOverlay(durationMs = 2000) {
    return await invoke<InputBridgeCommandResult>('input_bridge_flash_overlay', { durationMs })
  }

  async function pasteText(text: string, source = 'asr-final', segmentId?: string) {
    return await invoke<InputBridgeCommandResult>('input_bridge_paste_text', {
      text,
      source,
      segmentId,
    })
  }

  return {
    getState,
    lockCurrent,
    unlock,
    useHistory,
    deleteHistory,
    flashOverlay,
    pasteText,
  }
}
