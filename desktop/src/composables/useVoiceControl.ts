import { onBeforeUnmount } from 'vue'
import { useAppStore, type SceneMode } from '@/stores/app'
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

export function useVoiceControl() {
  const appStore = useAppStore()
  let countdownTimer: ReturnType<typeof setInterval> | null = null
  let commandDeadline = 0
  let previousScene: SceneMode | null = null
  let pendingProcessing = 0
  let consecutiveFailures = 0

  function getCommandTimeoutMs() {
    return Math.max(2000, appStore.voiceControl.commandTimeoutMs || 10000)
  }

  function resetCommandCountdown() {
    const timeoutMs = getCommandTimeoutMs()
    commandDeadline = Date.now() + timeoutMs
    appStore.voiceCommandRemainingMs = timeoutMs
    return timeoutMs
  }

  function ensureCountdownTicker() {
    if (countdownTimer)
      return

    countdownTimer = setInterval(() => {
      if (!appStore.voiceCommandActive) {
        clearTimers()
        return
      }
      if (appStore.voiceCommandProcessing)
        return

      const remaining = Math.max(0, commandDeadline - Date.now())
      appStore.voiceCommandRemainingMs = remaining
      if (remaining > 0)
        return

      if (previousScene && appStore.sceneMode !== previousScene)
        appStore.sceneMode = previousScene
      exitCommandMode('timeout')
    }, COMMAND_COUNTDOWN_TICK_MS)
  }

  function setProcessing(active: boolean) {
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

  function clearTimers() {
    if (countdownTimer) {
      clearInterval(countdownTimer)
      countdownTimer = null
    }
    appStore.voiceCommandRemainingMs = 0
  }

  function exitCommandMode(reason: 'switched' | 'timeout' | 'manual' | 'cancelled' | 'failure') {
    if (!appStore.voiceCommandActive && reason !== 'cancelled')
      return
    appStore.voiceCommandActive = false
    clearTimers()
    consecutiveFailures = 0
    void debugLog('voice.command', 'exit command mode', { reason, restoredScene: appStore.sceneMode })
    previousScene = null
  }

  function enterCommandMode() {
    if (!appStore.voiceControl.enabled) return
    previousScene = appStore.sceneMode
    appStore.voiceCommandActive = true
    clearTimers()
    const timeoutMs = resetCommandCountdown()
    ensureCountdownTicker()
    void debugLog('voice.command', 'enter command mode', { timeoutMs, previousScene })
  }

  async function handleSegmentText(rawText: string): Promise<SegmentHandleResult> {
    const text = (rawText || '').trim()
    if (!text)
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
    appStore.voiceCommandProcessing = false
    consecutiveFailures = 0
  }

  onBeforeUnmount(() => {
    clearTimers()
    appStore.voiceCommandProcessing = false
  })

  return {
    handleSegmentText,
    ensureLoaded,
    reset,
    exitCommandMode,
  }
}
