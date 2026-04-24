export const PRODUCT_EDITIONS = {
  STANDARD: 'standard',
  ADVANCED: 'advanced',
} as const

export type ProductEdition = typeof PRODUCT_EDITIONS[keyof typeof PRODUCT_EDITIONS]

export const PRODUCT_FEATURE_KEYS = {
  REALTIME: 'realtime',
  BATCH: 'batch',
  MEETING: 'meeting',
  VOICEPRINT: 'voiceprint',
  VOICE_CONTROL: 'voice_control',
} as const

export type ProductFeatureKey = typeof PRODUCT_FEATURE_KEYS[keyof typeof PRODUCT_FEATURE_KEYS]

const productFeatureKeySet = new Set<ProductFeatureKey>(Object.values(PRODUCT_FEATURE_KEYS))

export function isProductFeatureKey(value: unknown): value is ProductFeatureKey {
  return typeof value === 'string' && productFeatureKeySet.has(value as ProductFeatureKey)
}