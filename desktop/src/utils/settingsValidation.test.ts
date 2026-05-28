import { describe, expect, it } from 'vitest'

import {
  MAX_DEVICE_ALIAS_LENGTH,
  MAX_SERVER_URL_LENGTH,
  normalizeRecognitionSettings,
  validateDeviceAlias,
  validateServerAddressInput,
} from './settingsValidation'

describe('settings validation', () => {
  it('normalizes service URLs for desktop settings saves', () => {
    expect(validateServerAddressInput('192.168.1.1:8080')).toMatchObject({
      valid: true,
      value: 'http://192.168.1.1:8080',
    })
    expect(validateServerAddressInput('http://192.168.1.1/')).toMatchObject({
      valid: true,
      value: 'http://192.168.1.1',
    })
    expect(validateServerAddressInput('localhost:8080')).toMatchObject({
      valid: true,
      value: 'http://localhost:8080',
    })
  })

  it('rejects overlong or malformed service addresses before persistence', () => {
    expect(validateServerAddressInput('x'.repeat(MAX_SERVER_URL_LENGTH + 1))).toMatchObject({
      valid: false,
      message: expect.stringContaining('服务地址不能超过'),
    })
    expect(validateServerAddressInput('http://%')).toMatchObject({
      valid: false,
      message: expect.stringContaining('服务地址格式不正确'),
    })
  })

  it('validates device aliases at the documented 128-character boundary', () => {
    expect(validateDeviceAlias('测试设备-001')).toMatchObject({ valid: true, value: '测试设备-001' })
    expect(validateDeviceAlias('A'.repeat(MAX_DEVICE_ALIAS_LENGTH))).toMatchObject({ valid: true })
    expect(validateDeviceAlias('A'.repeat(MAX_DEVICE_ALIAS_LENGTH + 1))).toMatchObject({
      valid: false,
      message: expect.stringContaining('设备别名不能超过'),
    })
  })

  it('rejects empty or illegal device aliases without calling the backend', () => {
    expect(validateDeviceAlias('   ')).toMatchObject({
      valid: false,
      message: '设备别名不允许为空',
    })
    expect(validateDeviceAlias('设备@#名')).toMatchObject({
      valid: false,
      message: expect.stringContaining('非法字符'),
    })
  })

  it('clamps VAD settings to safe runtime ranges', () => {
    expect(normalizeRecognitionSettings({
      minSpeechThreshold: -1,
      noiseGateMultiplier: 0,
      endSilenceChunks: 10000,
      minEffectiveSpeechChunks: -3,
      singleChunkPeakMultiplier: 10,
    })).toEqual({
      keepPunctuation: false,
      minSpeechThreshold: 0.018,
      noiseGateMultiplier: 2.8,
      endSilenceChunks: 20,
      minEffectiveSpeechChunks: 2,
      singleChunkPeakMultiplier: 3,
    })
  })
})