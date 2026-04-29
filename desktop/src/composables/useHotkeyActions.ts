import { PRODUCT_CAPABILITY_KEYS, SCENE_MODES, type SceneMode } from '@/constants/product'
import { useAppStore } from '@/stores/app'
import { debugLog } from '@/utils/debug'
import { HOTKEY_ACTIONS, type HotkeyActionId } from '@/utils/hotkeys'
import { useVoiceControl } from './useVoiceControl'

function activateSceneMode(mode: SceneMode) {
  const appStore = useAppStore()
  const voiceControl = useVoiceControl()
  if (mode === SCENE_MODES.MEETING && !appStore.hasCapability(PRODUCT_CAPABILITY_KEYS.MEETING))
    return false

  if (appStore.voiceCommandActive || appStore.pendingVoiceCommandActivation)
    voiceControl.exitCommandMode('manual')

  if (appStore.sceneMode !== mode) {
    appStore.sceneMode = mode
    appStore.invalidateWorkflowBindings()
  }

  if (!appStore.isRecording)
    appStore.isRecording = true

  void debugLog('shortcut.action', 'activated scene mode', { mode, recording: appStore.isRecording })
  return true
}

function toggleRecording() {
  const appStore = useAppStore()
  const voiceControl = useVoiceControl()
  if (appStore.isRecording) {
    if (appStore.voiceCommandActive || appStore.pendingVoiceCommandActivation)
      voiceControl.exitCommandMode('manual')
    appStore.pendingVoiceCommandActivation = false
    appStore.voiceCommandAutoStartedRecording = false
    appStore.isRecording = false
    void debugLog('shortcut.action', 'stopped recording from hotkey action', { scene: appStore.sceneMode })
    return true
  }

  appStore.isRecording = true
  void debugLog('shortcut.action', 'started recording from hotkey action', { scene: appStore.sceneMode })
  return true
}

function toggleVoiceCommandMode() {
  const appStore = useAppStore()
  const voiceControl = useVoiceControl()
  if (!appStore.hasCapability(PRODUCT_CAPABILITY_KEYS.VOICE_CONTROL) || !appStore.voiceControl.enabled) {
    void debugLog('shortcut.action', 'voice command mode unavailable', {
      capability: appStore.hasCapability(PRODUCT_CAPABILITY_KEYS.VOICE_CONTROL),
      enabled: appStore.voiceControl.enabled,
    })
    return false
  }

  if (appStore.voiceCommandActive || appStore.pendingVoiceCommandActivation) {
    voiceControl.exitCommandMode('manual')
    void debugLog('shortcut.action', 'exited voice command mode from hotkey action', {
      recording: appStore.isRecording,
    })
    return true
  }

  if (!appStore.isRecording) {
    appStore.voiceCommandAutoStartedRecording = true
    appStore.pendingVoiceCommandActivation = true
    appStore.isRecording = true
    void debugLog('shortcut.action', 'armed voice command mode and started recording', {
      scene: appStore.sceneMode,
    })
    return true
  }

  appStore.pendingVoiceCommandActivation = false
  appStore.voiceCommandAutoStartedRecording = false
  const entered = voiceControl.enterCommandMode()
  void debugLog('shortcut.action', 'entered voice command mode from hotkey action', {
    entered,
    scene: appStore.sceneMode,
  })
  return entered
}

function cycleSceneMode() {
  const appStore = useAppStore()
  if (!appStore.hasCapability(PRODUCT_CAPABILITY_KEYS.MEETING))
    return activateSceneMode(SCENE_MODES.REPORT)

  const nextMode = appStore.sceneMode === SCENE_MODES.REPORT ? SCENE_MODES.MEETING : SCENE_MODES.REPORT
  return activateSceneMode(nextMode)
}

export function useHotkeyActions() {
  return {
    activateSceneMode,
    toggleRecording,
    toggleVoiceCommandMode,
    cycleSceneMode,
    handleHotkeyAction(action: HotkeyActionId) {
      switch (action) {
        case HOTKEY_ACTIONS.TOGGLE_RECORDING:
          return toggleRecording()
        case HOTKEY_ACTIONS.TOGGLE_VOICE_COMMAND_MODE:
          return toggleVoiceCommandMode()
        case HOTKEY_ACTIONS.CYCLE_SCENE_MODE:
          return cycleSceneMode()
        case HOTKEY_ACTIONS.ACTIVATE_REPORT_MODE:
          return activateSceneMode(SCENE_MODES.REPORT)
        case HOTKEY_ACTIONS.ACTIVATE_MEETING_MODE:
          return activateSceneMode(SCENE_MODES.MEETING)
        default:
          return false
      }
    },
  }
}