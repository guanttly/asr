export const PRODUCT_EDITIONS = {
  STANDARD: 'standard',
  ADVANCED: 'advanced',
} as const

export type ProductEdition = typeof PRODUCT_EDITIONS[keyof typeof PRODUCT_EDITIONS]

export const PRODUCT_CAPABILITY_KEYS = {
  REALTIME: 'realtime',
  BATCH: 'batch',
  MEETING: 'meeting',
  VOICEPRINT: 'voiceprint',
  VOICE_CONTROL: 'voiceControl',
} as const

export type ProductCapabilityKey = typeof PRODUCT_CAPABILITY_KEYS[keyof typeof PRODUCT_CAPABILITY_KEYS]

export const PRODUCT_API_CAPABILITY_KEYS = {
  REALTIME: 'realtime',
  BATCH: 'batch',
  MEETING: 'meeting',
  VOICEPRINT: 'voiceprint',
  VOICE_CONTROL: 'voice_control',
} as const

export type ProductAPICapabilityKey = typeof PRODUCT_API_CAPABILITY_KEYS[keyof typeof PRODUCT_API_CAPABILITY_KEYS]

export const SCENE_MODES = {
  REPORT: 'report',
  MEETING: 'meeting',
} as const

export type SceneMode = typeof SCENE_MODES[keyof typeof SCENE_MODES]