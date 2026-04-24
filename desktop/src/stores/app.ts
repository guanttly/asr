import { reactive, ref, watch } from 'vue'
import type { RecognitionSettings } from '@/composables/useSettings'
import { defineStore } from 'pinia'
import { PRODUCT_API_CAPABILITY_KEYS, PRODUCT_CAPABILITY_KEYS, PRODUCT_EDITIONS, SCENE_MODES, type ProductCapabilityKey, type ProductEdition, type SceneMode as ProductSceneMode } from '@/constants/product'
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
  serverUrl: string
  token: string
  deviceAlias: string
  machineCode: string
  username: string
  displayName: string
  role: string
  autoInject: boolean
  autoHideWindowOnRecordStart: boolean
  microphonePermissionGranted: boolean
  debugLoggingEnabled: boolean
  recognitionSettings: RecognitionSettings
  sceneMode: SceneMode
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
    serverUrl: DEFAULT_SERVER_URL,
    token: '',
    deviceAlias: '',
    machineCode: '',
    username: '',
    displayName: '',
    role: '',
    autoInject: true,
    autoHideWindowOnRecordStart: false,
    microphonePermissionGranted: false,
    debugLoggingEnabled: false,
    recognitionSettings: { ...DEFAULT_RECOGNITION },
    sceneMode: SCENE_MODES.REPORT,
  }
}

function normalizePersistedState(parsed?: Partial<PersistedState> | null): PersistedState {
  const defaults = defaultPersistedState()
  return {
    serverUrl: normalizeServerUrl(parsed?.serverUrl || defaults.serverUrl),
    token: parsed?.token || '',
    deviceAlias: parsed?.deviceAlias || '',
    machineCode: parsed?.machineCode || '',
    username: parsed?.username || '',
    displayName: parsed?.displayName || '',
    role: parsed?.role || '',
    autoInject: parsed?.autoInject !== false,
    autoHideWindowOnRecordStart: parsed?.autoHideWindowOnRecordStart === true,
    microphonePermissionGranted: parsed?.microphonePermissionGranted === true,
    debugLoggingEnabled: parsed?.debugLoggingEnabled === true,
    recognitionSettings: { ...DEFAULT_RECOGNITION, ...parsed?.recognitionSettings },
    sceneMode: parsed?.sceneMode === SCENE_MODES.MEETING ? SCENE_MODES.MEETING : SCENE_MODES.REPORT,
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
  return parsePersistedState(localStorage.getItem(SETTINGS_STORAGE_KEY)) || defaultPersistedState()
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
  const autoHideWindowOnRecordStart = ref(persisted.autoHideWindowOnRecordStart)
  const microphonePermissionGranted = ref(persisted.microphonePermissionGranted)
  const debugLoggingEnabled = ref(persisted.debugLoggingEnabled)
  const recognitionSettings = reactive<RecognitionSettings>({ ...persisted.recognitionSettings })
  const sceneMode = ref<SceneMode>(persisted.sceneMode)
  const voiceControl = reactive<VoiceControlConfig>({
    commandTimeoutMs: DEFAULT_COMMAND_TIMEOUT_MS,
    enabled: true,
  })
  const productEdition = ref<ProductEdition>(PRODUCT_EDITIONS.STANDARD)
  const productCapabilities = reactive<ProductCapabilities>(defaultProductCapabilities())
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

  let suppressPersist = false
  let lastSerializedState = serializePersistedState(persisted)
  const syncChannel = typeof BroadcastChannel !== 'undefined'
    ? new BroadcastChannel(SETTINGS_SYNC_CHANNEL)
    : null

  function snapshotState(): PersistedState {
    return {
      serverUrl: serverUrl.value,
      token: token.value,
      deviceAlias: deviceAlias.value,
      machineCode: machineCode.value,
      username: username.value,
      displayName: displayName.value,
      role: role.value,
      autoInject: autoInject.value,
      autoHideWindowOnRecordStart: autoHideWindowOnRecordStart.value,
      microphonePermissionGranted: microphonePermissionGranted.value,
      debugLoggingEnabled: debugLoggingEnabled.value,
      recognitionSettings: { ...recognitionSettings },
      sceneMode: sceneMode.value,
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
    autoHideWindowOnRecordStart.value = next.autoHideWindowOnRecordStart
    microphonePermissionGranted.value = next.microphonePermissionGranted
    debugLoggingEnabled.value = next.debugLoggingEnabled
    Object.assign(recognitionSettings, next.recognitionSettings)
    sceneMode.value = next.sceneMode
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
    autoHideWindowOnRecordStart,
    microphonePermissionGranted,
    debugLoggingEnabled,
    sceneMode,
  ], () => persist())

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
    autoHideWindowOnRecordStart,
    microphonePermissionGranted,
    debugLoggingEnabled,
    recognitionSettings,
    sceneMode,
    voiceControl,
    productEdition,
    productCapabilities,
    productFeaturesLoaded,
    voiceControlLoaded,
    voiceCommandActive,
    voiceCommandProcessing,
    voiceCommandRemainingMs,
    history,
    isRecording,
    expanded,
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
