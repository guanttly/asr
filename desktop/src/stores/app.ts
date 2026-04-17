import { reactive, ref, watch } from 'vue'
import type { RecognitionSettings } from '@/composables/useSettings'
import { defineStore } from 'pinia'
import { DEFAULT_SERVER_URL, normalizeServerUrl } from '@/utils/server'

export const SETTINGS_STORAGE_KEY = 'asr-desktop-settings'
const MAX_HISTORY = 50

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
}

const DEFAULT_RECOGNITION: RecognitionSettings = {
  keepPunctuation: false,
  minSpeechThreshold: 0.018,
  noiseGateMultiplier: 2.8,
  endSilenceChunks: 4,
  minEffectiveSpeechChunks: 2,
  singleChunkPeakMultiplier: 1.45,
}

function loadPersisted(): PersistedState {
  try {
    const raw = localStorage.getItem(SETTINGS_STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<PersistedState>
      return {
        serverUrl: normalizeServerUrl(parsed.serverUrl || DEFAULT_SERVER_URL),
        token: parsed.token || '',
        deviceAlias: parsed.deviceAlias || '',
        machineCode: parsed.machineCode || '',
        username: parsed.username || '',
        displayName: parsed.displayName || '',
        role: parsed.role || '',
        autoInject: parsed.autoInject !== false,
        autoHideWindowOnRecordStart: false,
        microphonePermissionGranted: parsed.microphonePermissionGranted === true,
        debugLoggingEnabled: parsed.debugLoggingEnabled === true,
        recognitionSettings: { ...DEFAULT_RECOGNITION, ...parsed.recognitionSettings },
      }
    }
  }
  catch { /* ignore */ }
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
  }
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

  const history = ref<string[]>([])
  const isRecording = ref(false)
  const expanded = ref(false)

  function appendHistory(text: string) {
    history.value.unshift(text)
    if (history.value.length > MAX_HISTORY)
      history.value.splice(MAX_HISTORY)
  }

  function clearHistory() {
    history.value = []
  }

  function persist() {
    const state: PersistedState = {
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
    }
    localStorage.setItem(SETTINGS_STORAGE_KEY, JSON.stringify(state))
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
    autoInject,
    autoHideWindowOnRecordStart,
    microphonePermissionGranted,
    debugLoggingEnabled,
  ], () => persist())

  return {
    serverUrl,
    token,
    deviceAlias,
    machineCode,
    username,
    displayName,
    role,
    autoInject,
    autoHideWindowOnRecordStart,
    microphonePermissionGranted,
    debugLoggingEnabled,
    recognitionSettings,
    history,
    isRecording,
    expanded,
    applyAuthenticatedUser,
    clearAuth,
    appendHistory,
    clearHistory,
    persist,
  }
})
