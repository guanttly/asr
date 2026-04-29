import { PRODUCT_CAPABILITY_KEYS } from '@/constants/product'
import { useAppStore, type SceneMode } from '@/stores/app'
import { ensureProductFeatures } from '@/utils/auth'
import { classifyVoiceIntent, fetchVoiceControl, primeVoiceControlAssets } from '@/utils/voiceControl'
import { resolveSceneModeFromVoiceIntent } from '@/utils/voiceCommandRegistry'
import { debugLog } from '@/utils/debug'

export interface SegmentHandleResult {
  /** True if the segment text should NOT be appended to history / injected. */
  swallow: boolean
  /** True when the command flow just switched the scene mode. */
  switched?: boolean
  /** True when the wake word triggered command-listening this turn. */
  enteredCommandMode?: boolean
}

// After this many consecutive failures we exit command mode early so we don't
// silently swallow every spoken segment until the user's command times out.
const COMMAND_FAILURE_LIMIT = 3
const COMMAND_COUNTDOWN_TICK_MS = 200

let countdownTimer: ReturnType<typeof setInterval> | null = null
let commandDeadline = 0
let previousScene: SceneMode | null = null
let pendingProcessing = 0
let consecutiveFailures = 0

function getAppStore() {
  return useAppStore()
}

function getCommandTimeoutMs() {
  const appStore = getAppStore()
  return Math.max(2000, appStore.voiceControl.commandTimeoutMs || 10000)
}

function resetCommandCountdown() {
  const appStore = getAppStore()
  const timeoutMs = getCommandTimeoutMs()
  commandDeadline = Date.now() + timeoutMs
  appStore.voiceCommandRemainingMs = timeoutMs
  return timeoutMs
}

function clearTimers() {
  const appStore = getAppStore()
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
  appStore.voiceCommandRemainingMs = 0
}

function stopAutoStartedRecording() {
  const appStore = getAppStore()
  appStore.pendingVoiceCommandActivation = false
  if (!appStore.voiceCommandAutoStartedRecording)
    return

  appStore.voiceCommandAutoStartedRecording = false
  if (appStore.isRecording)
    appStore.isRecording = false
}

function ensureCountdownTicker() {
  const appStore = getAppStore()
  if (countdownTimer)
    return

  countdownTimer = setInterval(() => {
    const store = getAppStore()
    if (!store.voiceCommandActive) {
      clearTimers()
      return
    }
    if (store.voiceCommandProcessing)
      return

    const remaining = Math.max(0, commandDeadline - Date.now())
    store.voiceCommandRemainingMs = remaining
    if (remaining > 0)
      return

    if (previousScene && store.sceneMode !== previousScene)
      store.sceneMode = previousScene
    exitCommandMode('timeout')
  }, COMMAND_COUNTDOWN_TICK_MS)

  appStore.voiceCommandRemainingMs = Math.max(0, commandDeadline - Date.now())
}

function setProcessing(active: boolean) {
  const appStore = getAppStore()
  if (active) {
    pendingProcessing += 1
    appStore.voiceCommandProcessing = true
    if (appStore.voiceCommandActive)
      resetCommandCountdown()
    return
  }
  pendingProcessing = Math.max(0, pendingProcessing - 1)
  if (pendingProcessing === 0)
    appStore.voiceCommandProcessing = false
}

function enterCommandMode() {
  const appStore = getAppStore()
  if (!appStore.hasCapability(PRODUCT_CAPABILITY_KEYS.VOICE_CONTROL))
    return false
  if (!appStore.voiceControl.enabled)
    return false

  previousScene = appStore.sceneMode
  appStore.pendingVoiceCommandActivation = false
  appStore.voiceCommandActive = true
  clearTimers()
  const timeoutMs = resetCommandCountdown()
  ensureCountdownTicker()
  void debugLog('voice.command', 'enter command mode', { timeoutMs, previousScene })
  return true
}

function exitCommandMode(reason: 'switched' | 'timeout' | 'manual' | 'cancelled' | 'failure') {
  const appStore = getAppStore()
  if (!appStore.voiceCommandActive && !appStore.pendingVoiceCommandActivation && reason !== 'cancelled')
    return false

  appStore.pendingVoiceCommandActivation = false
  appStore.voiceCommandActive = false
  clearTimers()
  consecutiveFailures = 0
  void debugLog('voice.command', 'exit command mode', { reason, restoredScene: appStore.sceneMode })
  previousScene = null
  stopAutoStartedRecording()
  return true
}

export function useVoiceControl() {
  const appStore = getAppStore()

  async function handleSegmentText(rawText: string): Promise<SegmentHandleResult> {
    const text = (rawText || '').trim()
    if (!text)
      return { swallow: false }

    if (!appStore.hasCapability(PRODUCT_CAPABILITY_KEYS.VOICE_CONTROL))
		return { swallow: false }

    if (!appStore.voiceControl.enabled)
      return { swallow: false }

    if (appStore.voiceCommandActive) {
      // Already waiting for a command — bypass wake detection and only classify the command intent.
      setProcessing(true)
      try {
        const result = await classifyVoiceIntent(text, { bypassWake: true })
        consecutiveFailures = 0
        void debugLog('voice.command', 'classify result', { text, result })
        const nextScene = resolveSceneModeFromVoiceIntent(result.intent)
        if (result.matched && nextScene) {
          if (appStore.sceneMode !== nextScene) {
            appStore.sceneMode = nextScene
            // Bindings are scoped per workflow; force-refresh on scene change so
            // the next realtime segment uses the freshest workflow.
            appStore.invalidateWorkflowBindings()
          }
          exitCommandMode('switched')
          return { swallow: true, switched: true }
        }
      }
      catch (error) {
        consecutiveFailures += 1
        void debugLog('voice.command', 'classify failed', error instanceof Error ? { message: error.message } : error)
        if (consecutiveFailures >= COMMAND_FAILURE_LIMIT) {
          // Restore the previous scene and bail so we stop swallowing speech.
          if (previousScene && appStore.sceneMode !== previousScene)
            appStore.sceneMode = previousScene
          exitCommandMode('failure')
          return { swallow: false }
        }
      }
      finally {
        setProcessing(false)
      }
      // Unknown intent → stay in command mode until timeout, but suppress this segment.
      return { swallow: true }
    }

    setProcessing(true)
    try {
      const result = await classifyVoiceIntent(text)
      void debugLog('voice.command', 'wake workflow result', { text, result })
      if (!result.wake_matched)
        return { swallow: false }

      enterCommandMode()
      const nextScene = resolveSceneModeFromVoiceIntent(result.intent)
      if (result.matched && nextScene) {
        if (appStore.sceneMode !== nextScene) {
          appStore.sceneMode = nextScene
          appStore.invalidateWorkflowBindings()
        }
        exitCommandMode('switched')
        return { swallow: true, switched: true, enteredCommandMode: true }
      }
      return { swallow: true, enteredCommandMode: true }
    }
    catch (error) {
      void debugLog('voice.command', 'wake workflow failed', error instanceof Error ? { message: error.message } : error)
      return { swallow: false }
    }
    finally {
      setProcessing(false)
    }
  }

  async function ensureLoaded(force = false) {
    await ensureProductFeatures(force)
    if (!appStore.hasCapability(PRODUCT_CAPABILITY_KEYS.VOICE_CONTROL)) {
		appStore.applyVoiceControl({ commandTimeoutMs: 10000, enabled: false })
		return
	}
    if (appStore.voiceControlLoaded && !force)
      return
    try {
      const cfg = await fetchVoiceControl()
      appStore.applyVoiceControl(cfg
        ? {
            commandTimeoutMs: cfg.command_timeout_ms,
            enabled: cfg.enabled,
          }
        : null)
      if (cfg?.enabled !== false)
        void primeVoiceControlAssets(force).catch(() => undefined)
    }
    catch (error) {
      // Server may be offline — fall back to defaults but mark as loaded so we
      // don't spam retries. The user can re-trigger via settings.
      void debugLog('voice.command', 'load voice control failed', error instanceof Error ? { message: error.message } : error)
      appStore.applyVoiceControl(null)
      void primeVoiceControlAssets(force).catch(() => undefined)
    }
  }

  function reset() {
    exitCommandMode('cancelled')
    pendingProcessing = 0
    appStore.pendingVoiceCommandActivation = false
    appStore.voiceCommandAutoStartedRecording = false
    appStore.voiceCommandProcessing = false
    consecutiveFailures = 0
  }

  return {
    handleSegmentText,
    ensureLoaded,
    reset,
    exitCommandMode,
    enterCommandMode,
  }
}
