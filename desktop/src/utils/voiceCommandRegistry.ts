import type { SceneMode } from '@/stores/app'

import { SCENE_MODES } from '@/constants/product'

export const VOICE_COMMAND_GROUP_KEYS = {
  sceneMode: 'scene_mode',
} as const

export const VOICE_COMMAND_INTENT_KEYS = {
  sceneReportSwitch: 'scene_report_switch',
  sceneMeetingSwitch: 'scene_meeting_switch',
} as const

export const VOICE_COMMAND_LEGACY_SCENE_KEYS = {
  report: SCENE_MODES.REPORT,
  meeting: SCENE_MODES.MEETING,
} as const

const sceneModeIntentMap: Record<string, SceneMode> = {
  [VOICE_COMMAND_INTENT_KEYS.sceneReportSwitch]: SCENE_MODES.REPORT,
  [VOICE_COMMAND_LEGACY_SCENE_KEYS.report]: SCENE_MODES.REPORT,
  [VOICE_COMMAND_INTENT_KEYS.sceneMeetingSwitch]: SCENE_MODES.MEETING,
  [VOICE_COMMAND_LEGACY_SCENE_KEYS.meeting]: SCENE_MODES.MEETING,
}

export function resolveSceneModeFromVoiceIntent(intent?: string | null): SceneMode | null {
  const normalized = (intent || '').trim()
  return sceneModeIntentMap[normalized] || null
}