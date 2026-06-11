import { reactive, ref, watch } from 'vue'
import type { RecognitionSettings } from '@/composables/useSettings'
import { defineStore } from 'pinia'
import { PRODUCT_API_CAPABILITY_KEYS, PRODUCT_CAPABILITY_KEYS, PRODUCT_EDITIONS, SCENE_MODES, type ProductCapabilityKey, type ProductEdition, type SceneMode as ProductSceneMode } from '@/constants/product'
import { cloneHotkeyBindings, normalizeHotkeyBindings, replaceHotkeyBindings, serializeHotkeyBindings, type HotkeyBindings } from '@/utils/hotkeys'
import { DEFAULT_SERVER_URL, normalizeServerUrl } from '@/utils/server'

export const SETTINGS_STORAGE_KEY = 'asr-desktop-settings'
const SETTINGS_SYNC_CHANNEL = `${SETTINGS_STORAGE_KEY}:sync`
const MAX_HISTORY = 50

export type SceneMode = ProductSceneMode
export const DEFAULT_COMMAND_TIMEOUT_MS = 10000

export interface ProductCapabilities {
  realtime: boolean
  batch: boolean
  meeting: boolean
  voiceprint: boolean
  voiceControl: boolean
}

export interface VoiceControlConfig {
  commandTimeoutMs: number
  enabled: boolean
}

export interface ProductLanguageOption {
  code: string
  label: string
}

function defaultProductCapabilities(): ProductCapabilities {
  return {
    [PRODUCT_CAPABILITY_KEYS.REALTIME]: true,
    [PRODUCT_CAPABILITY_KEYS.BATCH]: true,
    [PRODUCT_CAPABILITY_KEYS.MEETING]: false,
    [PRODUCT_CAPABILITY_KEYS.VOICEPRINT]: false,
    [PRODUCT_CAPABILITY_KEYS.VOICE_CONTROL]: false,
  }
}

interface PersistedState {
  packagedServerUrl: string
  serverUrl: string
  token: string
  deviceAlias: string
  machineCode: string
  username: string
  displayName: string
  role: string
  autoInject: boolean
  autoStart: boolean
  autoHideWindowOnRecordStart: boolean
  microphonePermissionGranted: boolean
  debugLoggingEnabled: boolean
  recognitionSettings: RecognitionSettings
  sceneMode: SceneMode
  hotkeys: HotkeyBindings
}

const DEFAULT_RECOGNITION: RecognitionSettings = {
  keepPunctuation: false,
  minSpeechThreshold: 0.018,
  noiseGateMultiplier: 2.8,
  endSilenceChunks: 4,
  minEffectiveSpeechChunks: 2,
  singleChunkPeakMultiplier: 1.45,
}

function defaultPersistedState(): PersistedState {
  return {
    packagedServerUrl: DEFAULT_SERVER_URL,
    serverUrl: DEFAULT_SERVER_URL,
    token: '',
    deviceAlias: '',
    machineCode: '',
    username: '',
    displayName: '',
    role: '',
    autoInject: true,
    autoStart: false,
    autoHideWindowOnRecordStart: false,
    microphonePermissionGranted: false,
    debugLoggingEnabled: false,
    recognitionSettings: { ...DEFAULT_RECOGNITION },
    sceneMode: SCENE_MODES.REPORT,
    hotkeys: cloneHotkeyBindings(),
  }
}

function isLoopbackServerUrl(raw?: string | null) {
  try {
    const hostname = new URL(normalizeServerUrl(raw)).hostname.toLowerCase()
    return hostname === 'localhost' || hostname === '127.0.0.1'
  }
  catch {
    return false
  }
}

function resolvePersistedServerUrl(parsed: Partial<PersistedState> | null | undefined, defaults: PersistedState) {
  const currentPackagedServerUrl = defaults.packagedServerUrl
  const persistedServerUrl = parsed?.serverUrl ? normalizeServerUrl(parsed.serverUrl) : ''
  if (!persistedServerUrl)
    return currentPackagedServerUrl

  const previousPackagedServerUrl = parsed?.packagedServerUrl ? normalizeServerUrl(parsed.packagedServerUrl) : ''
  if (previousPackagedServerUrl) {
    if (previousPackagedServerUrl !== currentPackagedServerUrl && persistedServerUrl === previousPackagedServerUrl)
      return currentPackagedServerUrl
    return persistedServerUrl
  }

  // 旧版本没有记录打包默认地址，且常把 localhost 写进持久化设置；
  // 仅在当前安装包已切到非本机地址时，才把这类历史默认值迁过来。
  if (!isLoopbackServerUrl(currentPackagedServerUrl) && isLoopbackServerUrl(persistedServerUrl))
    return currentPackagedServerUrl

  return persistedServerUrl
}

function normalizePersistedState(parsed?: Partial<PersistedState> | null): PersistedState {
  const defaults = defaultPersistedState()
  return {
    packagedServerUrl: defaults.packagedServerUrl,
    serverUrl: resolvePersistedServerUrl(parsed, defaults),
    token: parsed?.token || '',
    deviceAlias: parsed?.deviceAlias || '',
    machineCode: parsed?.machineCode || '',
    username: parsed?.username || '',
    displayName: parsed?.displayName || '',
    role: parsed?.role || '',
    autoInject: parsed?.autoInject !== false,
    autoStart: parsed?.autoStart === true,
    autoHideWindowOnRecordStart: parsed?.autoHideWindowOnRecordStart === true,
    microphonePermissionGranted: parsed?.microphonePermissionGranted === true,
    debugLoggingEnabled: parsed?.debugLoggingEnabled === true,
    recognitionSettings: { ...DEFAULT_RECOGNITION, ...parsed?.recognitionSettings },
    sceneMode: parsed?.sceneMode === SCENE_MODES.MEETING ? SCENE_MODES.MEETING : SCENE_MODES.REPORT,
    hotkeys: normalizeHotkeyBindings(parsed?.hotkeys),
  }
}

function parsePersistedState(raw?: string | null): PersistedState | null {
  if (!raw)
    return null

  try {
    return normalizePersistedState(JSON.parse(raw) as Partial<PersistedState>)
  }
  catch {
    return null
  }
}

function serializePersistedState(state: PersistedState) {
  return JSON.stringify(state)
}

function loadPersisted(): PersistedState {
  const raw = localStorage.getItem(SETTINGS_STORAGE_KEY)
  const parsed = parsePersistedState(raw)
  if (parsed) {
    const serialized = serializePersistedState(parsed)
    if (raw !== serialized)
      localStorage.setItem(SETTINGS_STORAGE_KEY, serialized)
    return parsed
  }
  return defaultPersistedState()
}

function defaultProductLanguages(): ProductLanguageOption[] {
  // 首选项必须是 auto：医学场景常有中英混合（缩写/单位/药名），
  // 锁中文会让英文术语被错听，锁英文会丢中文病历主体。
  return [
    { code: 'auto', label: '自动识别（中英混合）' },
    { code: 'zh-CN', label: '普通话' },
    { code: 'en-US', label: '英文（美）' },
  ]
}

export const useAppStore = defineStore('app', () => {
  const persisted = loadPersisted()

  const serverUrl = ref(persisted.serverUrl)
  const token = ref(persisted.token)
  const deviceAlias = ref(persisted.deviceAlias)
  const machineCode = ref(persisted.machineCode)
  const username = ref(persisted.username)
  const displayName = ref(persisted.displayName)
  const role = ref(persisted.role)
  const autoInject = ref(persisted.autoInject)
  const autoStart = ref(persisted.autoStart)
  watch(autoStart, (enabled) => {
    // 开机自启是系统级设置，变更时同步到 Tauri/Electron 原生层。
    void invoke('set_autostart', { enabled }).catch(() => undefined)
  })
  const autoHideWindowOnRecordStart = ref(persisted.autoHideWindowOnRecordStart)
  const microphonePermissionGranted = ref(persisted.microphonePermissionGranted)
  const microphoneDetected = ref(true)
  const debugLoggingEnabled = ref(persisted.debugLoggingEnabled)
  const recognitionSettings = reactive<RecognitionSettings>({ ...persisted.recognitionSettings })
  const sceneMode = ref<SceneMode>(persisted.sceneMode)
  const hotkeys = reactive<HotkeyBindings>(cloneHotkeyBindings(persisted.hotkeys))
  const voiceControl = reactive<VoiceControlConfig>({
    commandTimeoutMs: DEFAULT_COMMAND_TIMEOUT_MS,
    enabled: true,
  })
  const productEdition = ref<ProductEdition>(PRODUCT_EDITIONS.STANDARD)
  const productCapabilities = reactive<ProductCapabilities>(defaultProductCapabilities())
  const productSupportedLanguages = ref<ProductLanguageOption[]>(defaultProductLanguages())
  const productFeaturesLoaded = ref(false)
  const voiceControlLoaded = ref(false)
  const voiceCommandActive = ref(false)
  const voiceCommandProcessing = ref(false)
  const voiceCommandRemainingMs = ref(0)
  const realtimeWorkflowId = ref<number | null>(null)
  const meetingWorkflowId = ref<number | null>(null)
  const voiceWorkflowId = ref<number | null>(null)
  const workflowBindingsLoaded = ref(false)

  const history = ref<string[]>([])
  const isRecording = ref(false)
  const expanded = ref(false)
  const pendingVoiceCommandActivation = ref(false)
  const voiceCommandAutoStartedRecording = ref(false)

  let suppressPersist = false
  let lastSerializedState = serializePersistedState(persisted)
  const syncChannel = typeof BroadcastChannel !== 'undefined'
    ? new BroadcastChannel(SETTINGS_SYNC_CHANNEL)
    : null

  function snapshotState(): PersistedState {
    return {
      packagedServerUrl: DEFAULT_SERVER_URL,
      serverUrl: serverUrl.value,
      token: token.value,
      deviceAlias: deviceAlias.value,
      machineCode: machineCode.value,
      username: username.value,
      displayName: displayName.value,
      role: role.value,
      autoInject: autoInject.value,
      autoStart: autoStart.value,
      autoHideWindowOnRecordStart: autoHideWindowOnRecordStart.value,
      microphonePermissionGranted: microphonePermissionGranted.value,
      debugLoggingEnabled: debugLoggingEnabled.value,
      recognitionSettings: { ...recognitionSettings },
      sceneMode: sceneMode.value,
      hotkeys: cloneHotkeyBindings(hotkeys),
    }
  }

  function applyPersistedState(next: PersistedState) {
    suppressPersist = true
    serverUrl.value = next.serverUrl
    token.value = next.token
    deviceAlias.value = next.deviceAlias
    machineCode.value = next.machineCode
    username.value = next.username
    displayName.value = next.displayName
    role.value = next.role
    autoInject.value = next.autoInject
    autoStart.value = next.autoStart
    autoHideWindowOnRecordStart.value = next.autoHideWindowOnRecordStart
    microphonePermissionGranted.value = next.microphonePermissionGranted
    debugLoggingEnabled.value = next.debugLoggingEnabled
    Object.assign(recognitionSettings, next.recognitionSettings)
    sceneMode.value = next.sceneMode
    replaceHotkeyBindings(hotkeys, next.hotkeys)
    suppressPersist = false
    lastSerializedState = serializePersistedState(next)
  }

  function syncFromRaw(raw?: string | null) {
    const next = parsePersistedState(raw)
    if (!next)
      return

    const serialized = serializePersistedState(next)
    if (serialized === lastSerializedState)
      return

    applyPersistedState(next)
  }

  function appendHistory(text: string) {
    history.value.unshift(text)
    if (history.value.length > MAX_HISTORY)
      history.value.splice(MAX_HISTORY)
  }

  function clearHistory() {
    history.value = []
  }

  function persist() {
    if (suppressPersist)
      return

    const state = snapshotState()
    const serialized = serializePersistedState(state)
    if (serialized === lastSerializedState)
      return

    lastSerializedState = serialized
    localStorage.setItem(SETTINGS_STORAGE_KEY, serialized)
    syncChannel?.postMessage(serialized)
  }

  function applyAuthenticatedUser(user?: { display_name?: string, role?: string, username?: string } | null) {
    username.value = user?.username?.trim() || ''
    displayName.value = user?.display_name?.trim() || ''
    role.value = user?.role?.trim() || ''
    if (!deviceAlias.value && displayName.value)
      deviceAlias.value = displayName.value
  }

  function clearAuth() {
    token.value = ''
    username.value = ''
    displayName.value = ''
    role.value = ''
    realtimeWorkflowId.value = null
    meetingWorkflowId.value = null
    voiceWorkflowId.value = null
    workflowBindingsLoaded.value = false
  }

  function applyWorkflowBindings(bindings?: { realtime?: number | null, meeting?: number | null, voice_control?: number | null } | null) {
    realtimeWorkflowId.value = typeof bindings?.realtime === 'number' ? bindings.realtime : null
    meetingWorkflowId.value = productCapabilities[PRODUCT_CAPABILITY_KEYS.MEETING] && typeof bindings?.meeting === 'number' ? bindings.meeting : null
    voiceWorkflowId.value = productCapabilities[PRODUCT_CAPABILITY_KEYS.VOICE_CONTROL] && typeof bindings?.voice_control === 'number' ? bindings.voice_control : null
    workflowBindingsLoaded.value = true
  }

  function invalidateWorkflowBindings() {
    workflowBindingsLoaded.value = false
  }

  function applyVoiceControl(cfg?: Partial<VoiceControlConfig> | null) {
    if (!productCapabilities[PRODUCT_CAPABILITY_KEYS.VOICE_CONTROL]) {
    voiceControl.commandTimeoutMs = DEFAULT_COMMAND_TIMEOUT_MS
    voiceControl.enabled = false
    voiceControlLoaded.value = true
    voiceCommandActive.value = false
    voiceCommandProcessing.value = false
    voiceCommandRemainingMs.value = 0
    return
  }
    if (cfg && typeof cfg.commandTimeoutMs === 'number' && cfg.commandTimeoutMs > 0) {
      voiceControl.commandTimeoutMs = cfg.commandTimeoutMs
    }
    if (cfg && typeof cfg.enabled === 'boolean') {
      voiceControl.enabled = cfg.enabled
    }
    voiceControlLoaded.value = true
  }

  function hasCapability(key: ProductCapabilityKey) {
    return Boolean(productCapabilities[key])
  }

  function applyProductFeatures(payload?: {
    edition?: string
    supported_languages?: ProductLanguageOption[]
    capabilities?: {
      realtime?: boolean
      batch?: boolean
      meeting?: boolean
      voiceprint?: boolean
      voice_control?: boolean
    }
  } | null) {
    productEdition.value = payload?.edition === PRODUCT_EDITIONS.ADVANCED ? PRODUCT_EDITIONS.ADVANCED : PRODUCT_EDITIONS.STANDARD
    productCapabilities[PRODUCT_CAPABILITY_KEYS.REALTIME] = payload?.capabilities?.[PRODUCT_API_CAPABILITY_KEYS.REALTIME] !== false
    productCapabilities[PRODUCT_CAPABILITY_KEYS.BATCH] = payload?.capabilities?.[PRODUCT_API_CAPABILITY_KEYS.BATCH] !== false
    productCapabilities[PRODUCT_CAPABILITY_KEYS.MEETING] = payload?.capabilities?.[PRODUCT_API_CAPABILITY_KEYS.MEETING] === true
    productCapabilities[PRODUCT_CAPABILITY_KEYS.VOICEPRINT] = payload?.capabilities?.[PRODUCT_API_CAPABILITY_KEYS.VOICEPRINT] === true
    productCapabilities[PRODUCT_CAPABILITY_KEYS.VOICE_CONTROL] = payload?.capabilities?.[PRODUCT_API_CAPABILITY_KEYS.VOICE_CONTROL] === true
    productSupportedLanguages.value = Array.isArray(payload?.supported_languages) && payload.supported_languages.length > 0
      ? payload.supported_languages.filter(item => typeof item.code === 'string' && typeof item.label === 'string')
      : defaultProductLanguages()
    productFeaturesLoaded.value = true
    if (!productCapabilities[PRODUCT_CAPABILITY_KEYS.MEETING]) {
      sceneMode.value = SCENE_MODES.REPORT
      meetingWorkflowId.value = null
    }
    if (!productCapabilities[PRODUCT_CAPABILITY_KEYS.VOICE_CONTROL]) {
      voiceWorkflowId.value = null
      voiceCommandActive.value = false
      voiceCommandProcessing.value = false
      voiceCommandRemainingMs.value = 0
      voiceControl.commandTimeoutMs = DEFAULT_COMMAND_TIMEOUT_MS
      voiceControl.enabled = false
      voiceControlLoaded.value = true
    }
  }

  // Auto-persist on key config changes
  watch([
    serverUrl,
    token,
    deviceAlias,
    machineCode,
    username,
    displayName,
    role,
    realtimeWorkflowId,
    meetingWorkflowId,
    voiceWorkflowId,
    workflowBindingsLoaded,
    autoInject,
    autoStart,
    autoHideWindowOnRecordStart,
    microphonePermissionGranted,
    debugLoggingEnabled,
    sceneMode,
  ], () => persist())

  watch(() => serializeHotkeyBindings(hotkeys), () => persist())

  if (typeof window !== 'undefined') {
    window.addEventListener('storage', (event) => {
      if (event.key === SETTINGS_STORAGE_KEY)
        syncFromRaw(event.newValue)
    })
  }

  syncChannel?.addEventListener('message', (event) => {
    if (typeof event.data === 'string')
      syncFromRaw(event.data)
  })

  return {
    serverUrl,
    token,
    deviceAlias,
    machineCode,
    username,
    displayName,
    role,
    realtimeWorkflowId,
    meetingWorkflowId,
    voiceWorkflowId,
    workflowBindingsLoaded,
    autoInject,
    autoStart,
    autoHideWindowOnRecordStart,
    microphonePermissionGranted,
    microphoneDetected,
    debugLoggingEnabled,
    recognitionSettings,
    sceneMode,
    hotkeys,
    voiceControl,
    productEdition,
    productCapabilities,
    productSupportedLanguages,
    productFeaturesLoaded,
    voiceControlLoaded,
    voiceCommandActive,
    voiceCommandProcessing,
    voiceCommandRemainingMs,
    history,
    isRecording,
    expanded,
    pendingVoiceCommandActivation,
    voiceCommandAutoStartedRecording,
    applyAuthenticatedUser,
    applyWorkflowBindings,
    invalidateWorkflowBindings,
    applyVoiceControl,
    applyProductFeatures,
    clearAuth,
    appendHistory,
    clearHistory,
    hasCapability,
    persist,
  }
})
