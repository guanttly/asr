import { invoke } from '@tauri-apps/api/core'
import { listen } from '@tauri-apps/api/event'
import { onBeforeUnmount } from 'vue'
import { useAppStore } from '@/stores/app'
import { debugLog } from '@/utils/debug'
import { isHotkeyActionId, toBackendHotkeyBindings } from '@/utils/hotkeys'
import { useHotkeyActions } from './useHotkeyActions'

export const DESKTOP_HOTKEY_ACTION_EVENT = 'desktop-hotkey-action'

export interface DesktopHotkeySyncResult {
  supported: boolean
  registered: number
  message: string
}

export function useDesktopHotkeys() {
  const appStore = useAppStore()
  const hotkeyActions = useHotkeyActions()
  let unlistenHotkeyAction: null | (() => void) = null

  async function syncHotkeys(reason = 'manual') {
    const result = await invoke<DesktopHotkeySyncResult>('configure_hotkeys', {
      bindings: toBackendHotkeyBindings(appStore.hotkeys),
    })
    await debugLog('shortcut.sync', 'synchronized desktop hotkeys', {
      reason,
      supported: result.supported,
      registered: result.registered,
      message: result.message,
    })
    return result
  }

  async function listenToHotkeyActions() {
    if (unlistenHotkeyAction)
      return

    unlistenHotkeyAction = await listen<string>(DESKTOP_HOTKEY_ACTION_EVENT, (event) => {
      if (typeof event.payload !== 'string' || !isHotkeyActionId(event.payload))
        return
      hotkeyActions.handleHotkeyAction(event.payload)
      void debugLog('shortcut.event', 'handled desktop hotkey action', { action: event.payload })
    })
  }

  onBeforeUnmount(() => {
    unlistenHotkeyAction?.()
    unlistenHotkeyAction = null
  })

  return {
    syncHotkeys,
    listenToHotkeyActions,
  }
}