import type { SceneMode } from '@/stores/app'

export const VOICE_COMMAND_GROUP_KEYS = {
  sceneMode: 'scene_mode',
} as const

export const VOICE_COMMAND_INTENT_KEYS = {
  sceneReportSwitch: 'scene_report_switch',
  sceneMeetingSwitch: 'scene_meeting_switch',
} as const

const sceneModeIntentMap: Record<string, SceneMode> = {
  [VOICE_COMMAND_INTENT_KEYS.sceneReportSwitch]: 'report',
  report: 'report',
  [VOICE_COMMAND_INTENT_KEYS.sceneMeetingSwitch]: 'meeting',
  meeting: 'meeting',
}

export function resolveSceneModeFromVoiceIntent(intent?: string | null): SceneMode | null {
  const normalized = (intent || '').trim()
  return sceneModeIntentMap[normalized] || null
}