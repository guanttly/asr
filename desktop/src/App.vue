<script setup lang="ts">
import { getCurrentWindow } from '@tauri-apps/api/window'
import { onBeforeUnmount, onMounted } from 'vue'
import RecorderWindow from './components/RecorderWindow.vue'
import SettingsWindow from './components/SettingsWindow.vue'
import { useAppStore } from './stores/app'
import { appendRuntimeLog, debugLog } from './utils/debug'

const appStore = useAppStore()
const shortcut = 'CmdOrCtrl+Shift+Space'

// getCurrentWindow() 在 settings 窗口可能因 IPC 初始化时序而失败，
// 必须用 try-catch 兜底，否则整个 Vue 无法挂载导致白屏。
let appWindow: ReturnType<typeof getCurrentWindow> | null = null
let isSettingsWindow = false
try {
  appWindow = getCurrentWindow()
  isSettingsWindow = appWindow.label === 'settings'
} catch {
  // IPC 未就绪，回退到 URL 参数检测
  isSettingsWindow = new URLSearchParams(window.location.search).get('window') === 'settings'
}
if (!isSettingsWindow) {
  isSettingsWindow = new URLSearchParams(window.location.search).get('window') === 'settings'
}
let unregisterShortcut: null | (() => Promise<void>) = null

onMounted(() => {
  void appendRuntimeLog('frontend.window', JSON.stringify({ label: appWindow?.label ?? 'unknown', isSettingsWindow }))

  if (isSettingsWindow)
    return

  import('@tauri-apps/plugin-global-shortcut').then(async ({ register, unregister }) => {
    await unregister(shortcut).catch(() => undefined)
    await register(shortcut, () => {
      appStore.isRecording = !appStore.isRecording
      void debugLog('shortcut', 'toggled recording from global shortcut', { recording: appStore.isRecording })
    })
    unregisterShortcut = () => unregister(shortcut)
    void debugLog('shortcut', 'registered global shortcut', { shortcut })
  })
    .catch((error) => {
      console.warn(error)
      void appendRuntimeLog('frontend.shortcut', error instanceof Error ? error.stack || error.message : String(error))
    })
})

onBeforeUnmount(() => {
  void unregisterShortcut?.().catch(() => undefined)
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
