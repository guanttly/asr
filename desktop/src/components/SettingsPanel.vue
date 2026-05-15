<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { invoke } from '@tauri-apps/api/core'
import { useDesktopHotkeys } from '@/composables/useDesktopHotkeys'
import { PRODUCT_CAPABILITY_KEYS, SCENE_MODES } from '@/constants/product'
import { useSettings } from '@/composables/useSettings'
import { useAudioRecorder } from '@/composables/useAudioRecorder'
import { useInputBridge, type InputBridgeStateView, type InputBridgeTargetView } from '@/composables/useInputBridge'
import { useVoiceControl } from '@/composables/useVoiceControl'
import { useAppStore, type SceneMode } from '@/stores/app'
import { ensureAnonymousLogin, ensureProductFeatures, ensureRealtimeWorkflowBinding, getCurrentUser, getMachineIdentity, pingServer, updateProfile, type MachineIdentity } from '@/utils/auth'
import { appendRuntimeLog, debugLog, getRuntimeLogPath, readRuntimeLogTail } from '@/utils/debug'
import { HOTKEY_ACTIONS, HOTKEY_ACTION_DEFINITIONS, HOTKEY_MOUSE_BUTTONS, cloneHotkeyBindings, findConflictingHotkeyAction, formatHotkeyBinding, normalizeHotkeyBinding, replaceHotkeyBindings, serializeHotkeyBindings, type HotkeyActionId, type HotkeyBinding } from '@/utils/hotkeys'
import { DEFAULT_SERVER_URL, FALLBACK_SERVER_URL, normalizeServerUrl } from '@/utils/server'

const appStore = useAppStore()
const { settings, reset } = useSettings()
const recorder = useAudioRecorder()
const voiceControl = useVoiceControl()
const desktopHotkeys = useDesktopHotkeys()
const inputBridge = useInputBridge()
const machineIdentity = ref<MachineIdentity | null>(null)
const runtimeLogPath = ref('')
const runtimeLogTail = ref('')
const inputBridgeState = ref<InputBridgeStateView | null>(null)
const inputBridgeLoading = ref(false)
const hotkeyDefinitions = HOTKEY_ACTION_DEFINITIONS
const RECOGNITION_CHUNK_MS = 200
const RECOGNITION_PRESETS = [
  {
    key: 'fast',
    title: '抢实时',
    scene: '报告短句、边说边插入',
    effect: '说完约 0.6s 切句，响应最快。',
    values: { minSpeechThreshold: 0.016, noiseGateMultiplier: 2.4, endSilenceChunks: 3, minEffectiveSpeechChunks: 1, singleChunkPeakMultiplier: 1.25 },
  },
  {
    key: 'balanced',
    title: '均衡',
    scene: '常规口述、普通办公室',
    effect: '说完约 0.8s 切句，兼顾速度和完整性。',
    values: { minSpeechThreshold: 0.018, noiseGateMultiplier: 2.8, endSilenceChunks: 4, minEffectiveSpeechChunks: 2, singleChunkPeakMultiplier: 1.45 },
  },
  {
    key: 'steady',
    title: '稳健长句',
    scene: '讲话有停顿、希望少切断',
    effect: '说完约 1.2s 切句，更少误切。',
    values: { minSpeechThreshold: 0.02, noiseGateMultiplier: 3.0, endSilenceChunks: 6, minEffectiveSpeechChunks: 2, singleChunkPeakMultiplier: 1.5 },
  },
  {
    key: 'noisy',
    title: '嘈杂环境',
    scene: '键盘声、环境噪声较多',
    effect: '说完约 1.4s 切句，优先抗噪。',
    values: { minSpeechThreshold: 0.03, noiseGateMultiplier: 3.8, endSilenceChunks: 7, minEffectiveSpeechChunks: 3, singleChunkPeakMultiplier: 1.8 },
  },
] as const

const endSilenceSeconds = computed({
  get: () => Number(((settings.value.endSilenceChunks * RECOGNITION_CHUNK_MS) / 1000).toFixed(1)),
  set: (value: number) => {
    settings.value.endSilenceChunks = Math.max(1, Math.min(20, Math.round((Number(value) * 1000) / RECOGNITION_CHUNK_MS)))
  },
})
const activeRecognitionPresetKey = computed(() => {
  const current = settings.value
  return RECOGNITION_PRESETS.find((preset) => {
    const values = preset.values
    return Math.abs(current.minSpeechThreshold - values.minSpeechThreshold) < 0.0005
      && Math.abs(current.noiseGateMultiplier - values.noiseGateMultiplier) < 0.05
      && current.endSilenceChunks === values.endSilenceChunks
      && current.minEffectiveSpeechChunks === values.minEffectiveSpeechChunks
      && Math.abs(current.singleChunkPeakMultiplier - values.singleChunkPeakMultiplier) < 0.02
  })?.key || 'custom'
})
const recognitionEffectSummary = computed(() => {
  const current = settings.value
  const speed = current.endSilenceChunks <= 3 ? '出字更快，适合短句连续口述' : current.endSilenceChunks <= 5 ? '速度和完整性比较均衡' : '更少误切，适合长句和思考停顿'
  const noise = current.noiseGateMultiplier >= 3.5 || current.minSpeechThreshold >= 0.028 ? '抗噪更强，但轻声可能更难触发' : '更容易捕捉轻声，但嘈杂环境可能误触发'
  return `预计说完后等待 ${formatRecognitionSeconds(endSilenceSeconds.value)} 切句，之后叠加 ASR 与工作流耗时。${speed}；${noise}。`
})

const authLoading = ref(false)
const authMessage = ref('')
const authMessageType = ref<'success' | 'error' | 'info'>('info')
const hotkeyMessage = ref('')
const hotkeyMessageType = ref<'success' | 'error' | 'info'>('info')
const inputBridgeMessage = ref('')
const inputBridgeMessageType = ref<'success' | 'error' | 'info'>('info')
const capturingHotkeyAction = ref<HotkeyActionId | null>(null)
const isElectronDesktop = typeof window !== 'undefined' && Boolean((window as { __electronBridge__?: unknown }).__electronBridge__)
const supportsMouseGlobalHotkeys = computed(() => !isElectronDesktop)
const serverHint = FALLBACK_SERVER_URL
  ? `打包时可通过环境变量 VITE_DEFAULT_SERVER_URL 覆盖默认服务器地址。当前默认地址：${DEFAULT_SERVER_URL}；连接失败时会自动回退到 ${FALLBACK_SERVER_URL}。`
  : `打包时可通过环境变量 VITE_DEFAULT_SERVER_URL 覆盖默认服务器地址。当前默认地址：${DEFAULT_SERVER_URL}`
const hotkeySectionHint = computed(() => {
  if (supportsMouseGlobalHotkeys.value)
    return 'Windows 下全局生效，支持键盘组合和鼠标侧键。点击当前热键开始录制；Esc 取消，Backspace 或 Delete 清空。补充场景里提供“直达报告模式 / 直达会议模式”，比循环切换更快。'

  return '当前 Win7 兼容版仅支持键盘全局热键。点击当前热键开始录制；Esc 取消，Backspace 或 Delete 清空。补充场景里提供“直达报告模式 / 直达会议模式”，比循环切换更快。'
})
const lockedInputTarget = computed(() => inputBridgeState.value?.lockedTarget || null)
const candidateInputTarget = computed(() => inputBridgeState.value?.candidateTarget || null)
const inputBridgeStateLabel = computed(() => {
  const state = inputBridgeState.value?.state || 'Idle'
  const labels: Record<string, string> = {
    Unsupported: '不可用',
    Idle: '未绑定',
    CandidateReady: '检测到候选框',
    Locked: '已绑定',
    Recovering: '正在恢复',
    FallbackCurrentFocus: '当前焦点兜底',
    Invalid: '目标失效',
  }
  return labels[state] || state
})
const inputBridgeStatusClass = computed(() => {
  const state = inputBridgeState.value?.state
  if (state === 'Locked')
    return 'valid'
  if (state === 'Invalid' || state === 'Unsupported')
    return 'invalid'
  if (state === 'CandidateReady' || state === 'FallbackCurrentFocus' || state === 'Recovering')
    return 'warning'
  return 'idle'
})

const modifierOnlyCodes = new Set([
  'ControlLeft',
  'ControlRight',
  'ShiftLeft',
  'ShiftRight',
  'AltLeft',
  'AltRight',
  'MetaLeft',
  'MetaRight',
])

function setSceneMode(mode: SceneMode) {
  if (mode === SCENE_MODES.MEETING && !appStore.hasCapability(PRODUCT_CAPABILITY_KEYS.MEETING))
    return
  if (appStore.sceneMode === mode) return
  if (appStore.voiceCommandActive || appStore.pendingVoiceCommandActivation)
    voiceControl.exitCommandMode('manual')
  appStore.sceneMode = mode
  appStore.invalidateWorkflowBindings()
  void debugLog('settings.scene', 'scene mode changed', { mode })
}

function formatRecognitionSeconds(seconds: number) {
  return `${Number(seconds.toFixed(1))}s`
}

function applyRecognitionPreset(preset: typeof RECOGNITION_PRESETS[number]) {
  Object.assign(settings.value, preset.values)
}

async function refreshVoiceControl() {
  await voiceControl.ensureLoaded(true)
}

function setAuthMessage(type: 'success' | 'error' | 'info', message: string) {
  authMessageType.value = type
  authMessage.value = message
}

function setHotkeyMessage(type: 'success' | 'error' | 'info', message: string) {
  hotkeyMessageType.value = type
  hotkeyMessage.value = message
}

function setInputBridgeMessage(type: 'success' | 'error' | 'info', message: string) {
  inputBridgeMessageType.value = type
  inputBridgeMessage.value = message
}

function formatTargetMeta(target?: InputBridgeTargetView | null) {
  if (!target)
    return '暂无目标'
  const parts = [target.processName, target.controlClassName].filter(Boolean)
  return parts.join(' / ') || target.status
}

function formatTargetUsedAt(value?: number) {
  if (!value)
    return '未写入'
  return new Date(value).toLocaleString()
}

async function refreshInputBridgeState(silent = false) {
  inputBridgeLoading.value = true
  try {
    inputBridgeState.value = await inputBridge.getState()
    if (!silent)
      setInputBridgeMessage(inputBridgeState.value.supported ? 'info' : 'error', inputBridgeState.value.message)
  }
  catch (error) {
    setInputBridgeMessage('error', error instanceof Error ? error.message : '读取输入桥状态失败')
  }
  finally {
    inputBridgeLoading.value = false
  }
}

async function flashInputTarget() {
  inputBridgeLoading.value = true
  try {
    const result = await inputBridge.flashOverlay(2000)
    setInputBridgeMessage(result.success ? 'success' : 'error', result.message)
    await refreshInputBridgeState(true)
  }
  catch (error) {
    setInputBridgeMessage('error', error instanceof Error ? error.message : '提示输入目标失败')
  }
  finally {
    inputBridgeLoading.value = false
  }
}

async function unlockInputTarget() {
  inputBridgeLoading.value = true
  try {
    const result = await inputBridge.unlock()
    setInputBridgeMessage(result.success ? 'success' : 'error', result.message)
    await refreshInputBridgeState(true)
  }
  catch (error) {
    setInputBridgeMessage('error', error instanceof Error ? error.message : '解除输入目标失败')
  }
  finally {
    inputBridgeLoading.value = false
  }
}

async function useHistoryInputTarget(targetId: string) {
  inputBridgeLoading.value = true
  try {
    const result = await inputBridge.useHistory(targetId)
    setInputBridgeMessage(result.success ? 'success' : 'error', result.message)
    await refreshInputBridgeState(true)
  }
  catch (error) {
    setInputBridgeMessage('error', error instanceof Error ? error.message : '切换历史目标失败')
  }
  finally {
    inputBridgeLoading.value = false
  }
}

async function deleteHistoryInputTarget(targetId: string) {
  inputBridgeLoading.value = true
  try {
    const result = await inputBridge.deleteHistory(targetId)
    setInputBridgeMessage(result.success ? 'success' : 'error', result.message)
    await refreshInputBridgeState(true)
  }
  catch (error) {
    setInputBridgeMessage('error', error instanceof Error ? error.message : '删除历史目标失败')
  }
  finally {
    inputBridgeLoading.value = false
  }
}

function shouldShowHotkeyAction(actionId: HotkeyActionId) {
  if (actionId === HOTKEY_ACTIONS.ACTIVATE_MEETING_MODE)
    return appStore.hasCapability(PRODUCT_CAPABILITY_KEYS.MEETING)
  return true
}

function getHotkeyTitle(actionId: HotkeyActionId) {
  return hotkeyDefinitions.find(item => item.id === actionId)?.title || actionId
}

function stopHotkeyCapture() {
  capturingHotkeyAction.value = null
}

function beginHotkeyCapture(actionId: HotkeyActionId) {
  capturingHotkeyAction.value = actionId
  setHotkeyMessage(
    'info',
    supportsMouseGlobalHotkeys.value
      ? '请直接按下组合键，支持鼠标侧键；Esc 取消，Backspace/Delete 清空。'
      : '请直接按下键盘组合；当前 Win7 兼容版不支持鼠标侧键全局热键。Esc 取消，Backspace/Delete 清空。',
  )
}

function applyHotkeyBinding(actionId: HotkeyActionId, binding: Partial<HotkeyBinding> | null) {
  const normalized = normalizeHotkeyBinding(binding)
  const conflictAction = findConflictingHotkeyAction(appStore.hotkeys, normalized, actionId)
  if (conflictAction) {
    setHotkeyMessage('error', `热键已被“${getHotkeyTitle(conflictAction)}”占用，请换一个组合。`)
    return false
  }

  appStore.hotkeys[actionId] = normalized
  stopHotkeyCapture()
  setHotkeyMessage('success', `${getHotkeyTitle(actionId)}已更新为 ${formatHotkeyBinding(normalized)}。`)
  return true
}

function clearHotkey(actionId: HotkeyActionId) {
  appStore.hotkeys[actionId] = normalizeHotkeyBinding(null)
  if (capturingHotkeyAction.value === actionId)
    stopHotkeyCapture()
  setHotkeyMessage('info', `${getHotkeyTitle(actionId)}已清空。`)
}

function restoreDefaultHotkeys() {
  replaceHotkeyBindings(appStore.hotkeys, cloneHotkeyBindings())
  stopHotkeyCapture()
  setHotkeyMessage('success', '已恢复默认热键配置。')
}

async function syncHotkeysNow(reason = 'settings') {
  try {
    const result = await desktopHotkeys.syncHotkeys(reason)
    setHotkeyMessage(result.supported ? 'success' : 'info', result.message)
  }
  catch (error) {
    setHotkeyMessage('error', error instanceof Error ? error.message : '热键同步失败')
  }
}

function handleHotkeyCaptureKeydown(event: KeyboardEvent) {
  const actionId = capturingHotkeyAction.value
  if (!actionId)
    return

  event.preventDefault()
  event.stopPropagation()

  if (event.code === 'Escape') {
    stopHotkeyCapture()
    setHotkeyMessage('info', '已取消热键录制。')
    return
  }

  if (event.code === 'Backspace' || event.code === 'Delete') {
    clearHotkey(actionId)
    return
  }

  if (event.repeat || modifierOnlyCodes.has(event.code))
    return

  void applyHotkeyBinding(actionId, {
    enabled: true,
    modifiers: {
      ctrl: event.ctrlKey,
      alt: event.altKey,
      shift: event.shiftKey,
      meta: event.metaKey,
    },
    trigger: {
      type: 'keyboard',
      code: event.code,
    },
  })
}

function handleHotkeyCaptureMousedown(event: MouseEvent) {
  const actionId = capturingHotkeyAction.value
  if (!actionId)
    return

  if (event.button !== 3 && event.button !== 4)
    return

  event.preventDefault()
  event.stopPropagation()

  if (!supportsMouseGlobalHotkeys.value) {
    setHotkeyMessage('info', '当前 Win7 兼容版仅支持键盘全局热键，请改用键盘组合。')
    return
  }

  const button = event.button === 3 ? HOTKEY_MOUSE_BUTTONS.BACK : HOTKEY_MOUSE_BUTTONS.FORWARD
  void applyHotkeyBinding(actionId, {
    enabled: true,
    modifiers: {
      ctrl: event.ctrlKey,
      alt: event.altKey,
      shift: event.shiftKey,
      meta: event.metaKey,
    },
    trigger: {
      type: 'mouse',
      button,
    },
  })
}

async function syncIdentityAndLogin(forceLogin = false) {
  authLoading.value = true
  try {
    await debugLog('settings.auth', 'starting identity sync', { forceLogin, serverUrl: appStore.serverUrl })
    appStore.serverUrl = normalizeServerUrl(appStore.serverUrl)
    machineIdentity.value = await getMachineIdentity()
    if (forceLogin || !appStore.token.trim())
      await ensureAnonymousLogin(forceLogin)
    else
      await getCurrentUser().catch(async () => await ensureAnonymousLogin(true))
    await ensureProductFeatures(true)
    await ensureRealtimeWorkflowBinding(true)
    setAuthMessage('success', '服务连接正常，已完成匿名登录')
    await debugLog('settings.auth', 'identity sync completed', { username: appStore.username, machineCode: appStore.machineCode, realtimeWorkflowId: appStore.realtimeWorkflowId })
  }
  catch (error) {
    setAuthMessage('error', error instanceof Error ? error.message : '匿名登录失败')
    void debugLog('settings.error', 'identity sync failed', error instanceof Error ? { message: error.message, stack: error.stack } : error)
  }
  finally {
    authLoading.value = false
  }
}

async function verifyServer() {
  authLoading.value = true
  try {
    await pingServer()
    setAuthMessage('success', `服务可达，当前地址 ${normalizeServerUrl(appStore.serverUrl)}`)
    await debugLog('settings.server', 'server health check passed', { serverUrl: appStore.serverUrl })
  }
  catch (error) {
    setAuthMessage('error', error instanceof Error ? error.message : '服务校验失败')
    void debugLog('settings.error', 'server health check failed', error instanceof Error ? { message: error.message, stack: error.stack } : error)
  }
  finally {
    authLoading.value = false
  }
}

async function saveAlias() {
  authLoading.value = true
  try {
    const alias = appStore.deviceAlias.trim()
    if (!alias) {
      setAuthMessage('error', '请先输入设备别名')
      return
    }
    await updateProfile(alias)
    setAuthMessage('success', '设备别名已保存')
    await debugLog('settings.profile', 'device alias updated', { alias })
  }
  catch (error) {
    setAuthMessage('error', error instanceof Error ? error.message : '保存别名失败')
    void debugLog('settings.error', 'device alias update failed', error instanceof Error ? { message: error.message, stack: error.stack } : error)
  }
  finally {
    authLoading.value = false
  }
}

async function requestMicrophonePermission() {
  authLoading.value = true
  try {
    await recorder.requestPermission()
    setAuthMessage('success', '麦克风授权已完成，之后点击开始将不再弹出首次授权提示')
    await debugLog('settings.audio', 'microphone permission initialized')
  }
  catch (error) {
    setAuthMessage('error', error instanceof Error ? error.message : '麦克风授权失败')
    void debugLog('settings.error', 'microphone permission init failed', error instanceof Error ? { message: error.message, stack: error.stack } : error)
  }
  finally {
    authLoading.value = false
  }
}

async function refreshRuntimeLogs() {
  runtimeLogPath.value = await getRuntimeLogPath().catch(() => '')
  runtimeLogTail.value = await readRuntimeLogTail(120).catch(() => '')
}

async function copyRuntimeLogs() {
  const content = runtimeLogTail.value.trim()
  if (!content) {
    setAuthMessage('info', '当前还没有可复制的调试日志')
    return
  }

  await navigator.clipboard.writeText(content)
  setAuthMessage('success', '最近调试日志已复制到剪贴板')
}

watch(() => appStore.debugLoggingEnabled, async (enabled) => {
  if (enabled) {
    await appendRuntimeLog('settings.debug', 'debug logging enabled from settings')
    await refreshRuntimeLogs()
  }
  else {
    await appendRuntimeLog('settings.debug', 'debug logging disabled from settings')
  }
})

watch(() => serializeHotkeyBindings(appStore.hotkeys), () => {
  void syncHotkeysNow('settings-watch')
}, { immediate: true })

onMounted(() => {
  void syncIdentityAndLogin(false)
  void refreshRuntimeLogs()
  void refreshInputBridgeState(true)
  void voiceControl.ensureLoaded()
  window.addEventListener('keydown', handleHotkeyCaptureKeydown, true)
  window.addEventListener('mousedown', handleHotkeyCaptureMousedown, true)
})

onBeforeUnmount(() => {
  stopHotkeyCapture()
  window.removeEventListener('keydown', handleHotkeyCaptureKeydown, true)
  window.removeEventListener('mousedown', handleHotkeyCaptureMousedown, true)
})
</script>

<template>
  <div class="settings-panel">
    <section class="settings-section">
      <h4 class="section-title">连接与身份</h4>
      <p class="section-hint">{{ serverHint }}</p>
      <div class="field">
        <label>服务器地址</label>
        <input
          v-model="appStore.serverUrl"
          type="text"
          :placeholder="DEFAULT_SERVER_URL"
          spellcheck="false"
        >
      </div>
      <div class="field">
        <label>设备别名</label>
        <input
          v-model="appStore.deviceAlias"
          type="text"
          placeholder="例如：张医生诊室电脑"
          spellcheck="false"
        >
      </div>
      <div class="field action-row">
        <button class="action-btn primary" :disabled="authLoading" @click="syncIdentityAndLogin(true)">连接并登录</button>
        <button class="action-btn" :disabled="authLoading" @click="verifyServer">校验服务</button>
      </div>
      <div class="field action-row">
        <button class="action-btn" :disabled="authLoading" @click="saveAlias">保存别名</button>
      </div>
      <div class="identity-grid">
        <div class="identity-card">
          <span class="identity-label">当前账号</span>
          <strong>{{ appStore.displayName || appStore.username || '未登录' }}</strong>
        </div>
        <div class="identity-card">
          <span class="identity-label">机器码</span>
          <strong class="machine-code">{{ appStore.machineCode || '读取中...' }}</strong>
        </div>
      </div>
      <div v-if="machineIdentity" class="identity-meta">
        <span>{{ machineIdentity.hostname || '未知主机' }}</span>
        <span>{{ machineIdentity.platform }}</span>
        <span>{{ machineIdentity.ip_addresses.join(' / ') || '无可用 IP' }}</span>
      </div>
      <p v-if="authMessage" class="auth-message" :class="authMessageType">
        {{ authMessage }}
      </p>
    </section>

    <section class="settings-section">
      <h4 class="section-title">录入行为</h4>
      <p class="section-hint">默认推荐使用 Ctrl+Shift+Space 开始和停止录音，这样更容易保持外部应用的输入焦点，自动注入也更稳定；也可以在下方全局热键里改成更顺手的组合。</p>
      <div class="field row permission-row">
        <div>
          <label class="status-label">麦克风权限</label>
          <p class="permission-hint">{{ appStore.microphonePermissionGranted ? '已可直接开始录音' : '可选：预先检测并初始化麦克风权限' }}</p>
        </div>
        <button class="action-btn primary compact" :disabled="authLoading" @click="requestMicrophonePermission">
          {{ appStore.microphonePermissionGranted ? '重新检测' : '检测麦克风' }}
        </button>
      </div>
      <div class="field row">
        <label>自动注入到光标</label>
        <label class="toggle">
          <input v-model="appStore.autoInject" type="checkbox">
          <span class="toggle-slider" />
        </label>
      </div>
      <div class="field row">
        <label>保留标点符号</label>
        <label class="toggle">
          <input v-model="settings.keepPunctuation" type="checkbox">
          <span class="toggle-slider" />
        </label>
      </div>
    </section>

    <section class="settings-section">
      <h4 class="section-title">语音写入目标</h4>
      <p class="section-hint">先点击 RIS 报告输入框，再按“绑定语音写入目标”热键；识别文本会优先写入该绑定目标，历史目标可自动恢复。</p>
      <div class="bridge-status" :class="inputBridgeStatusClass">
        <div class="bridge-status-head">
          <span>当前状态</span>
          <strong>{{ inputBridgeStateLabel }}</strong>
        </div>
        <div class="bridge-target-name">
          {{ lockedInputTarget?.displayName || candidateInputTarget?.displayName || inputBridgeState?.message || '暂无可写入目标' }}
        </div>
        <div class="bridge-target-meta">
          {{ formatTargetMeta(lockedInputTarget || candidateInputTarget) }}
        </div>
      </div>
      <div class="field action-row bridge-actions">
        <button class="action-btn" :disabled="inputBridgeLoading" @click="refreshInputBridgeState(false)">刷新状态</button>
        <button class="action-btn" :disabled="inputBridgeLoading || !lockedInputTarget" @click="flashInputTarget">提示目标</button>
        <button class="action-btn" :disabled="inputBridgeLoading || !lockedInputTarget" @click="unlockInputTarget">解除绑定</button>
      </div>
      <div v-if="inputBridgeState?.history.length" class="bridge-history">
        <article v-for="target in inputBridgeState.history" :key="target.targetId" class="bridge-history-item">
          <div class="bridge-history-main">
            <strong>{{ target.displayName }}</strong>
            <span>{{ formatTargetMeta(target) }}</span>
            <span>最近写入：{{ formatTargetUsedAt(target.lastUsedAt) }} · {{ target.useCount || 0 }} 次</span>
          </div>
          <div class="bridge-history-actions">
            <button class="action-btn compact" :disabled="inputBridgeLoading" @click="useHistoryInputTarget(target.targetId)">使用</button>
            <button class="action-btn compact" :disabled="inputBridgeLoading" @click="deleteHistoryInputTarget(target.targetId)">删除</button>
          </div>
        </article>
      </div>
      <p v-if="inputBridgeMessage" class="auth-message" :class="inputBridgeMessageType">
        {{ inputBridgeMessage }}
      </p>
    </section>

    <section class="settings-section">
      <h4 class="section-title">全局热键</h4>
      <p class="section-hint">{{ hotkeySectionHint }}</p>
      <div class="hotkey-list">
        <article
          v-for="definition in hotkeyDefinitions"
          v-show="shouldShowHotkeyAction(definition.id)"
          :key="definition.id"
          class="hotkey-item"
          :class="{ capturing: capturingHotkeyAction === definition.id }"
        >
          <div class="hotkey-head">
            <div>
              <div class="hotkey-title-row">
                <strong class="hotkey-title">{{ definition.title }}</strong>
                <span v-if="definition.optional" class="hotkey-badge">补充场景</span>
              </div>
              <p class="hotkey-description">{{ definition.description }}</p>
            </div>
          </div>
          <div class="hotkey-actions">
            <button
              type="button"
              class="hotkey-binding-btn"
              :class="{
                capturing: capturingHotkeyAction === definition.id,
                empty: !appStore.hotkeys[definition.id].enabled || !appStore.hotkeys[definition.id].trigger,
              }"
              @click="beginHotkeyCapture(definition.id)"
            >
              {{ capturingHotkeyAction === definition.id ? '按下热键...' : formatHotkeyBinding(appStore.hotkeys[definition.id]) }}
            </button>
            <button
              type="button"
              class="action-btn compact"
              :disabled="!appStore.hotkeys[definition.id].enabled"
              @click="clearHotkey(definition.id)"
            >
              清空
            </button>
          </div>
        </article>
      </div>
      <div class="field action-row hotkey-footer">
        <button class="action-btn" @click="restoreDefaultHotkeys">恢复默认热键</button>
        <button class="action-btn" @click="syncHotkeysNow('settings-button')">重新同步</button>
      </div>
      <p v-if="hotkeyMessage" class="auth-message" :class="hotkeyMessageType">
        {{ hotkeyMessage }}
      </p>
    </section>

    <section class="settings-section">
      <h4 class="section-title">使用场景</h4>
      <p class="section-hint">{{ appStore.hasCapability(PRODUCT_CAPABILITY_KEYS.MEETING) ? '报告模式：录音结束仅保存为实时转写历史；会议模式：录音结束自动创建会议纪要任务。终端语音控制可在录音中切换两种场景。' : '当前版本仅开放报告模式，录音结束后保存为实时转写历史。' }}</p>
      <div class="scene-segmented" role="tablist">
        <button
          type="button"
          class="scene-btn report"
          :class="{ active: appStore.sceneMode === SCENE_MODES.REPORT }"
          @click="setSceneMode(SCENE_MODES.REPORT)"
        >
          <span class="scene-dot" /> 报告模式
          <span class="scene-tag">实时</span>
        </button>
        <button
          v-if="appStore.hasCapability(PRODUCT_CAPABILITY_KEYS.MEETING)"
          type="button"
          class="scene-btn meeting"
          :class="{ active: appStore.sceneMode === SCENE_MODES.MEETING }"
          @click="setSceneMode(SCENE_MODES.MEETING)"
        >
          <span class="scene-dot" /> 会议模式
          <span class="scene-tag">纪要</span>
        </button>
      </div>
    </section>

    <section v-if="appStore.hasCapability(PRODUCT_CAPABILITY_KEYS.VOICE_CONTROL)" class="settings-section">
      <h4 class="section-title">终端语音控制</h4>
      <p class="section-hint">桌面端会把每段转写文本发送给当前绑定的语音控制工作流，由后台统一执行 voice_wake 与 voice_intent 节点；命中后，小球会进入“等待指令”状态（蓝色）。</p>
      <div class="voice-card">
        <div class="voice-row">
          <span class="voice-label">等待时长</span>
          <span>{{ Math.round(appStore.voiceControl.commandTimeoutMs / 1000) }} 秒</span>
        </div>
        <div class="voice-row">
          <span class="voice-label">语音控制</span>
          <span :class="appStore.voiceControl.enabled ? 'voice-on' : 'voice-off'">{{ appStore.voiceControl.enabled ? '已启用' : '已关闭' }}</span>
        </div>
      </div>
      <div class="field action-row">
        <button class="action-btn" :disabled="authLoading" @click="refreshVoiceControl">刷新语音控制配置</button>
      </div>
      <p class="section-hint">实际唤醒词、同音词归一化和命令命中结果，均以后台执行当前绑定工作流返回的结果为准。这里仅展示全局运行状态和等待时长。</p>
    </section>

    <section class="settings-section">
      <h4 class="section-title">调试</h4>
      <p class="section-hint">打开后会把前端关键事件、录音、上传和窗口启动信息追加到本地日志文件，便于排查 Windows 打包运行问题。</p>
      <div class="field row">
        <label>启用详细调试日志</label>
        <label class="toggle">
          <input v-model="appStore.debugLoggingEnabled" type="checkbox">
          <span class="toggle-slider" />
        </label>
      </div>
      <div class="field action-row">
        <button class="action-btn" @click="refreshRuntimeLogs">刷新日志</button>
        <button class="action-btn" @click="copyRuntimeLogs">复制日志</button>
        <button class="action-btn" @click="invoke('open_devtools').catch(() => undefined)">打开控制台</button>
      </div>
      <p class="section-hint">日志文件：{{ runtimeLogPath || '读取中...' }}</p>
      <textarea class="debug-log" readonly :value="runtimeLogTail" placeholder="最近日志会显示在这里" />
    </section>

    <!-- VAD Parameters -->
    <section class="settings-section">
      <h4 class="section-title">语音检测参数</h4>
      <p class="section-hint">先按场景选择预设，再微调句尾等待。当前按 200ms 小块做本地分句，配置只影响后续录音。</p>

      <div class="preset-grid">
        <button
          v-for="preset in RECOGNITION_PRESETS"
          :key="preset.key"
          type="button"
          class="preset-card"
          :class="{ active: activeRecognitionPresetKey === preset.key }"
          @click="applyRecognitionPreset(preset)"
        >
          <span class="preset-head">
            <span class="preset-title">{{ preset.title }}</span>
            <span class="preset-time">{{ formatRecognitionSeconds((preset.values.endSilenceChunks * RECOGNITION_CHUNK_MS) / 1000) }}</span>
          </span>
          <span class="preset-scene">{{ preset.scene }}</span>
          <span class="preset-effect">{{ preset.effect }}</span>
        </button>
      </div>

      <div class="recognition-summary">{{ recognitionEffectSummary }}</div>

      <div class="field">
        <label>句尾等待时间 ({{ formatRecognitionSeconds(endSilenceSeconds) }} / {{ settings.endSilenceChunks }} 块)</label>
        <input
          v-model.number="endSilenceSeconds"
          type="range"
          min="0.2"
          max="4"
          step="0.2"
        >
      </div>
      <div class="field">
        <label>最小语音阈值 ({{ settings.minSpeechThreshold.toFixed(3) }})</label>
        <input
          v-model.number="settings.minSpeechThreshold"
          type="range"
          min="0.005"
          max="0.08"
          step="0.001"
        >
      </div>
      <div class="field">
        <label>噪声门开倍数 ({{ settings.noiseGateMultiplier.toFixed(1) }})</label>
        <input
          v-model.number="settings.noiseGateMultiplier"
          type="range"
          min="1.2"
          max="6"
          step="0.1"
        >
      </div>
      <div class="field">
        <label>最小有效语音块数 ({{ settings.minEffectiveSpeechChunks }})</label>
        <input
          v-model.number="settings.minEffectiveSpeechChunks"
          type="range"
          min="1"
          max="6"
          step="1"
        >
      </div>
      <div class="field">
        <label>单块峰值倍数 ({{ settings.singleChunkPeakMultiplier.toFixed(2) }})</label>
        <input
          v-model.number="settings.singleChunkPeakMultiplier"
          type="range"
          min="1"
          max="3"
          step="0.05"
        >
      </div>
      <button class="reset-btn" @click="reset">恢复默认参数</button>
    </section>
  </div>
</template>

<style scoped>
.settings-panel {
  padding: 8px;
}

.settings-section {
  margin-bottom: 16px;
}

.section-title {
  font-size: 12px;
  font-weight: 600;
  color: #435266;
  margin-bottom: 8px;
  padding: 0 4px;
}

.section-hint {
  padding: 0 4px;
  margin-bottom: 8px;
  font-size: 11px;
  line-height: 1.5;
  color: #64748b;
}

.field {
  margin-bottom: 8px;
  padding: 0 4px;
}

.preset-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  padding: 0 4px 8px;
}

.preset-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px;
  border-radius: 12px;
  border: 1px solid rgba(148, 163, 184, 0.26);
  background: rgba(255, 255, 255, 0.78);
  color: #1f2937;
  text-align: left;
  cursor: pointer;
}

.preset-card.active {
  border-color: rgba(15, 118, 110, 0.32);
  background: rgba(15, 118, 110, 0.08);
  color: #0f766e;
}

.preset-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.preset-title {
  font-size: 12px;
  font-weight: 700;
}

.preset-time {
  padding: 1px 6px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.8);
  color: #64748b;
  font-size: 10px;
}

.preset-scene,
.preset-effect {
  font-size: 10px;
  line-height: 1.45;
  color: #64748b;
}

.recognition-summary {
  margin: 0 4px 10px;
  padding: 9px 10px;
  border-radius: 12px;
  border: 1px solid rgba(15, 118, 110, 0.12);
  background: rgba(15, 118, 110, 0.06);
  color: #0f766e;
  font-size: 11px;
  line-height: 1.55;
}

.identity-grid {
  display: grid;
  gap: 8px;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  padding: 0 4px;
}

.identity-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px;
  border-radius: 10px;
  background: rgba(15, 118, 110, 0.05);
  border: 1px solid rgba(15, 118, 110, 0.08);
}

.identity-label {
  font-size: 11px;
  color: #64748b;
}

.machine-code {
  font-size: 11px;
  line-height: 1.5;
  word-break: break-all;
}

.identity-meta {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 4px 0;
  font-size: 11px;
  line-height: 1.5;
  color: #64748b;
}

.field label {
  display: block;
  font-size: 11px;
  color: #64748b;
  margin-bottom: 4px;
}

.field.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.field.row label:first-child {
  margin-bottom: 0;
}

.permission-row {
  align-items: center;
  gap: 12px;
}

.status-label {
  margin-bottom: 2px;
}

.permission-hint {
  font-size: 11px;
  line-height: 1.4;
  color: #64748b;
}

.field input[type="text"],
.field input[type="password"],
.field select {
  width: 100%;
  padding: 6px 8px;
  font-size: 12px;
  border: 1px solid rgba(0, 0, 0, 0.1);
  border-radius: 6px;
  background: rgba(0, 0, 0, 0.02);
  outline: none;
  color: #16202c;
  transition: border-color 0.15s;
}

.field input[type="text"]:focus,
.field input[type="password"]:focus,
.field select:focus {
  border-color: #0f766e;
}

.field input[type="range"] {
  width: 100%;
  accent-color: #0f766e;
}

.bridge-status {
  margin: 0 4px 8px;
  padding: 10px 12px;
  border-radius: 10px;
  border: 1px solid rgba(148, 163, 184, 0.26);
  background: rgba(248, 250, 252, 0.78);
}

.bridge-status.valid {
  border-color: rgba(22, 163, 74, 0.28);
  background: rgba(240, 253, 244, 0.85);
}

.bridge-status.warning {
  border-color: rgba(217, 119, 6, 0.28);
  background: rgba(255, 251, 235, 0.85);
}

.bridge-status.invalid {
  border-color: rgba(220, 38, 38, 0.24);
  background: rgba(254, 242, 242, 0.86);
}

.bridge-status-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  font-size: 11px;
  color: #64748b;
}

.bridge-status-head strong {
  color: #16202c;
  font-size: 12px;
}

.bridge-target-name {
  margin-top: 6px;
  font-size: 12px;
  font-weight: 700;
  color: #1f2937;
  line-height: 1.45;
  word-break: break-word;
}

.bridge-target-meta {
  margin-top: 4px;
  font-size: 11px;
  line-height: 1.45;
  color: #64748b;
  word-break: break-word;
}

.bridge-actions {
  margin-top: 8px;
}

.bridge-history {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 0 4px;
}

.bridge-history-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 10px;
  border-radius: 10px;
  border: 1px solid rgba(148, 163, 184, 0.22);
  background: rgba(255, 255, 255, 0.74);
}

.bridge-history-main {
  min-width: 0;
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 3px;
}

.bridge-history-main strong {
  color: #1f2937;
  font-size: 12px;
  line-height: 1.4;
  word-break: break-word;
}

.bridge-history-main span {
  color: #64748b;
  font-size: 10px;
  line-height: 1.4;
  word-break: break-word;
}

.bridge-history-actions {
  display: flex;
  flex-direction: column;
  gap: 6px;
  flex: 0 0 auto;
}

.hotkey-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 0 4px;
}

.hotkey-item {
  padding: 12px 14px;
  border-radius: 12px;
  border: 1px solid rgba(148, 163, 184, 0.28);
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.92), rgba(248, 250, 252, 0.9));
  transition: border-color 0.16s ease, box-shadow 0.16s ease, transform 0.16s ease;
}

.hotkey-item.capturing {
  border-color: rgba(37, 99, 235, 0.4);
  box-shadow: 0 8px 18px rgba(37, 99, 235, 0.12);
  transform: translateY(-1px);
}

.hotkey-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.hotkey-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.hotkey-title {
  font-size: 12px;
  color: #1f2937;
}

.hotkey-badge {
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 10px;
  color: #92400e;
  background: rgba(245, 158, 11, 0.14);
  border: 1px solid rgba(245, 158, 11, 0.2);
}

.hotkey-description {
  margin-top: 6px;
  font-size: 11px;
  line-height: 1.5;
  color: #64748b;
}

.hotkey-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
}

.hotkey-binding-btn {
  flex: 1;
  min-height: 34px;
  padding: 8px 10px;
  border-radius: 10px;
  border: 1px dashed rgba(15, 23, 42, 0.12);
  background: rgba(15, 23, 42, 0.03);
  color: #0f172a;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
  cursor: pointer;
  transition: border-color 0.15s ease, background 0.15s ease, color 0.15s ease;
}

.hotkey-binding-btn:hover {
  border-color: rgba(15, 118, 110, 0.26);
  background: rgba(15, 118, 110, 0.05);
}

.hotkey-binding-btn.capturing {
  color: #1d4ed8;
  border-color: rgba(37, 99, 235, 0.35);
  background: rgba(37, 99, 235, 0.08);
}

.hotkey-binding-btn.empty {
  color: #94a3b8;
  font-weight: 500;
}

.hotkey-footer {
  margin-top: 10px;
}

.action-row {
  display: flex;
  gap: 8px;
}

.action-btn {
  flex: 1;
  padding: 7px 8px;
  font-size: 12px;
  color: #435266;
  background: rgba(0, 0, 0, 0.03);
  border: 1px solid rgba(0, 0, 0, 0.08);
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s;
}

.action-btn:hover:not(:disabled) {
  background: rgba(0, 0, 0, 0.06);
}

.action-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.action-btn.primary {
  color: white;
  background: #0f766e;
  border-color: #0f766e;
}

.action-btn.primary:hover:not(:disabled) {
  background: #0d5f59;
}

.action-btn.compact {
  flex: 0 0 auto;
  min-width: 92px;
}

.auth-message {
  padding: 0 4px;
  margin-top: 8px;
  font-size: 11px;
  line-height: 1.5;
}

.auth-message.success {
  color: #0f766e;
}

.auth-message.error {
  color: #dc2626;
}

.auth-message.info {
  color: #64748b;
}

/* Toggle switch */
.toggle {
  position: relative;
  display: inline-block;
  width: 36px;
  height: 20px;
  flex-shrink: 0;
}

.toggle input {
  opacity: 0;
  width: 0;
  height: 0;
}

.toggle-slider {
  position: absolute;
  cursor: pointer;
  top: 0; left: 0; right: 0; bottom: 0;
  background: #cbd5e1;
  border-radius: 10px;
  transition: 0.2s;
}

.toggle-slider::before {
  content: '';
  position: absolute;
  width: 16px;
  height: 16px;
  left: 2px;
  bottom: 2px;
  background: white;
  border-radius: 50%;
  transition: 0.2s;
}

.toggle input:checked + .toggle-slider {
  background: #0f766e;
}

.toggle input:checked + .toggle-slider::before {
  transform: translateX(16px);
}

.reset-btn {
  width: 100%;
  padding: 6px;
  font-size: 12px;
  color: #64748b;
  background: rgba(0, 0, 0, 0.03);
  border: 1px solid rgba(0, 0, 0, 0.06);
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s;
  margin-top: 4px;
}

.reset-btn:hover {
  background: rgba(0, 0, 0, 0.06);
}

.debug-log {
  width: 100%;
  min-height: 180px;
  padding: 10px;
  font-size: 11px;
  line-height: 1.5;
  color: #16202c;
  background: rgba(15, 23, 42, 0.04);
  border: 1px solid rgba(148, 163, 184, 0.2);
  border-radius: 10px;
  resize: vertical;
}

/* Scene mode segmented control */
.scene-segmented {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  padding: 0 4px;
}

.scene-btn {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 14px;
  border-radius: 12px;
  border: 1.5px solid rgba(148, 163, 184, 0.35);
  background: rgba(255, 255, 255, 0.6);
  color: #475569;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.18s ease;
  text-align: left;
}

.scene-btn:hover { background: rgba(255, 255, 255, 0.9); }

.scene-btn .scene-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: currentColor;
  opacity: 0.6;
}

.scene-btn .scene-tag {
  margin-left: auto;
  font-size: 11px;
  opacity: 0.7;
}

.scene-btn.report.active {
  color: #0f766e;
  border-color: #0f766e;
  background: rgba(15, 118, 110, 0.08);
  box-shadow: 0 4px 12px rgba(15, 118, 110, 0.15);
}

.scene-btn.meeting.active {
  color: #b45309;
  border-color: #d97706;
  background: rgba(217, 119, 6, 0.08);
  box-shadow: 0 4px 12px rgba(217, 119, 6, 0.18);
}

/* Voice control read-only card */
.voice-card {
  margin: 0 4px;
  padding: 12px 14px;
  border-radius: 12px;
  background: rgba(37, 99, 235, 0.06);
  border: 1px solid rgba(37, 99, 235, 0.18);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.voice-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
  color: #1f2937;
}

.voice-label {
  color: #64748b;
}

.voice-wake {
  font-size: 14px;
  letter-spacing: 0.5px;
  color: #1d4ed8;
}

.voice-on { color: #0f766e; }
.voice-off { color: #94a3b8; }
</style>
