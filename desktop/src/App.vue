<script setup lang="ts">
import { invoke } from '@tauri-apps/api/core'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { onBeforeUnmount, onMounted, watch } from 'vue'
import { useDesktopHotkeys } from './composables/useDesktopHotkeys'
import RecorderWindow from './components/RecorderWindow.vue'
import SettingsWindow from './components/SettingsWindow.vue'
import { useAppStore } from './stores/app'
import { ensureProductFeatures } from './utils/auth'
import { appendRuntimeLog, debugLog } from './utils/debug'
import { serializeHotkeyBindings } from './utils/hotkeys'

const appStore = useAppStore()
const desktopHotkeys = useDesktopHotkeys()

// getCurrentWindow() 在 settings 窗口可能因 IPC 初始化时序而失败，
// 必须用 try-catch 兜底，否则整个 Vue 无法挂载导致白屏。
let appWindow: ReturnType<typeof getCurrentWindow> | null = null
let isSettingsWindow = false
try {
  appWindow = getCurrentWindow()
  isSettingsWindow = appWindow.label === 'settings'
} catch {
  // IPC 未就绪，回退到初始化脚本注入的标记
}
if (!isSettingsWindow) {
  isSettingsWindow = (window as any).__ASR_WINDOW__ === 'settings'
}
let stopHotkeyWatcher: null | (() => void) = null

function handleKeydown(e: KeyboardEvent) {
  if (e.altKey && e.shiftKey && !e.ctrlKey && !e.metaKey && e.code === 'KeyD') {
    e.preventDefault()
    void invoke('open_devtools').catch(() => undefined)
  }
}

onMounted(() => {
  void appendRuntimeLog('frontend.window', JSON.stringify({ label: appWindow?.label ?? 'unknown', isSettingsWindow }))
  void ensureProductFeatures().catch(() => undefined)

  window.addEventListener('keydown', handleKeydown)

  if (isSettingsWindow)
    return

  void desktopHotkeys.listenToHotkeyActions().catch((error) => {
    console.warn(error)
    void appendRuntimeLog('frontend.shortcut', error instanceof Error ? error.stack || error.message : String(error))
  })

  stopHotkeyWatcher = watch(() => serializeHotkeyBindings(appStore.hotkeys), () => {
    void desktopHotkeys.syncHotkeys('main-window').catch((error) => {
      console.warn(error)
      void appendRuntimeLog('frontend.shortcut', error instanceof Error ? error.stack || error.message : String(error))
    })
  }, { immediate: true })
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeydown)
  stopHotkeyWatcher?.()
})
</script>

<template>
  <div class="app-shell">
    <SettingsWindow v-if="isSettingsWindow" />
    <RecorderWindow v-else />
  </div>
</template>

<style scoped>
.app-shell {
  height: 100vh;
  overflow: hidden;
  background: transparent;
}
</style>
