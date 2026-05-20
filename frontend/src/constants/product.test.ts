import { describe, expect, it } from 'vitest'

import { isProductFeatureKey, PRODUCT_FEATURE_KEYS } from './product'

describe('product feature keys', () => {
  it('accepts only known feature keys', () => {
    expect(isProductFeatureKey(PRODUCT_FEATURE_KEYS.MEETING)).toBe(true)
    expect(isProductFeatureKey('voice_control')).toBe(true)
    expect(isProductFeatureKey('unknown')).toBe(false)
    expect(isProductFeatureKey(null)).toBe(false)
  })
})
