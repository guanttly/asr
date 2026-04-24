export const VOICE_COMMAND_GROUP_KEYS = {
  sceneMode: 'scene_mode',
} as const

export const VOICE_COMMAND_INTENT_KEYS = {
  sceneReportSwitch: 'scene_report_switch',
  sceneMeetingSwitch: 'scene_meeting_switch',
} as const

export const VOICE_COMMAND_LEGACY_INTENT_KEYS = {
  sceneReportSwitch: 'report',
  sceneMeetingSwitch: 'meeting',
} as const

export type VoiceCommandGroupKey = typeof VOICE_COMMAND_GROUP_KEYS[keyof typeof VOICE_COMMAND_GROUP_KEYS]
export type VoiceCommandIntentKey = typeof VOICE_COMMAND_INTENT_KEYS[keyof typeof VOICE_COMMAND_INTENT_KEYS]

export interface BuiltinVoiceCommandIntentSpec {
  key: VoiceCommandIntentKey
  handlerName: string
  defaultLabel: string
  description: string
  legacyKeys?: readonly string[]
}

export interface BuiltinVoiceCommandGroupSpec {
  key: VoiceCommandGroupKey
  name: string
  description: string
  intents: readonly BuiltinVoiceCommandIntentSpec[]
}

export const BUILTIN_VOICE_COMMAND_GROUPS: readonly BuiltinVoiceCommandGroupSpec[] = [
  {
    key: VOICE_COMMAND_GROUP_KEYS.sceneMode,
    name: '场景切换控制',
    description: '桌面端语音控制里的场景切换命令组。',
    intents: [
      {
        key: VOICE_COMMAND_INTENT_KEYS.sceneReportSwitch,
        handlerName: '切换到报告模式',
        defaultLabel: '报告模式',
        description: '把桌面端切换到报告模式。',
        legacyKeys: [VOICE_COMMAND_LEGACY_INTENT_KEYS.sceneReportSwitch],
      },
      {
        key: VOICE_COMMAND_INTENT_KEYS.sceneMeetingSwitch,
        handlerName: '切换到会议模式',
        defaultLabel: '会议模式',
        description: '把桌面端切换到会议模式。',
        legacyKeys: [VOICE_COMMAND_LEGACY_INTENT_KEYS.sceneMeetingSwitch],
      },
    ],
  },
] as const

export function findBuiltinVoiceCommandGroup(groupKey?: string | null) {
  const key = (groupKey || '').trim()
  return BUILTIN_VOICE_COMMAND_GROUPS.find(group => group.key === key) || null
}

export function normalizeVoiceCommandGroupKey(groupKey?: string | null) {
  const key = (groupKey || '').trim()
  const group = BUILTIN_VOICE_COMMAND_GROUPS.find(item => item.key === key)
  return group?.key || ''
}

export function findBuiltinVoiceCommandIntent(intentKey?: string | null, groupKey?: string | null) {
  const group = findBuiltinVoiceCommandGroup(groupKey)
  if (!group)
    return null
  const key = (intentKey || '').trim()
  return group.intents.find(intent => intent.key === key || intent.legacyKeys?.includes(key)) || null
}

export function buildVoiceCommandGroupOptions() {
  return BUILTIN_VOICE_COMMAND_GROUPS.map(group => ({
    label: group.name,
    value: group.key,
    description: `${group.key} · ${group.description}`,
  }))
}

export function buildVoiceCommandIntentOptions(groupKey?: string | null) {
  const group = findBuiltinVoiceCommandGroup(groupKey)
  if (!group)
    return []
  return group.intents.map(intent => ({
    label: intent.handlerName,
    value: intent.key,
    description: `${intent.key} · ${intent.description}`,
  }))
}