import type { RecognitionSettings } from '@/composables/useSettings'
import { normalizeServerUrl } from './server'

export const MAX_SERVER_URL_LENGTH = 2048
export const MAX_DEVICE_ALIAS_LENGTH = 128

export const DEFAULT_RECOGNITION_SETTINGS: RecognitionSettings = {
  keepPunctuation: false,
  minSpeechThreshold: 0.018,
  noiseGateMultiplier: 2.8,
  endSilenceChunks: 4,
  minEffectiveSpeechChunks: 2,
  singleChunkPeakMultiplier: 1.45,
}

export interface ValidationResult {
  valid: boolean
  value: string
  message: string
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value))
}

function characterLength(value: string) {
  return Array.from(value).length
}

function positiveNumberOrDefault(value: unknown, fallback: number) {
  const numberValue = Number(value)
  return Number.isFinite(numberValue) && numberValue > 0 ? numberValue : fallback
}

export function normalizeRecognitionSettings(raw?: Partial<RecognitionSettings>): RecognitionSettings {
  const settings = raw || {}
  return {
    keepPunctuation: Boolean(settings.keepPunctuation),
    minSpeechThreshold: clamp(positiveNumberOrDefault(settings.minSpeechThreshold, DEFAULT_RECOGNITION_SETTINGS.minSpeechThreshold), 0.005, 0.08),
    noiseGateMultiplier: clamp(positiveNumberOrDefault(settings.noiseGateMultiplier, DEFAULT_RECOGNITION_SETTINGS.noiseGateMultiplier), 1.2, 6),
    endSilenceChunks: Math.round(clamp(positiveNumberOrDefault(settings.endSilenceChunks, DEFAULT_RECOGNITION_SETTINGS.endSilenceChunks), 1, 20)),
    minEffectiveSpeechChunks: Math.round(clamp(positiveNumberOrDefault(settings.minEffectiveSpeechChunks, DEFAULT_RECOGNITION_SETTINGS.minEffectiveSpeechChunks), 1, 6)),
    singleChunkPeakMultiplier: clamp(positiveNumberOrDefault(settings.singleChunkPeakMultiplier, DEFAULT_RECOGNITION_SETTINGS.singleChunkPeakMultiplier), 1, 3),
  }
}

export function validateServerAddressInput(raw?: string | null): ValidationResult {
  const value = (raw || '').trim()
  if (characterLength(value) > MAX_SERVER_URL_LENGTH) {
    return {
      valid: false,
      value,
      message: `服务地址不能超过 ${MAX_SERVER_URL_LENGTH} 个字符`,
    }
  }

  const normalized = normalizeServerUrl(value)
  try {
    const url = new URL(normalized)
    if (url.protocol !== 'http:' && url.protocol !== 'https:')
      throw new Error('unsupported protocol')
  }
  catch {
    return {
      valid: false,
      value,
      message: '服务地址格式不正确，请检查地址或协议',
    }
  }

  return { valid: true, value: normalized, message: '' }
}

export function validateDeviceAlias(raw?: string | null): ValidationResult {
  const value = (raw || '').trim()
  if (!value) {
    return {
      valid: false,
      value,
      message: '设备别名不允许为空',
    }
  }

  if (characterLength(value) > MAX_DEVICE_ALIAS_LENGTH) {
    return {
      valid: false,
      value,
      message: `设备别名不能超过 ${MAX_DEVICE_ALIAS_LENGTH} 个字符`,
    }
  }

  if (!/^[\p{Script=Han}\p{Letter}\p{Number} _\-.（）()]+$/u.test(value)) {
    return {
      valid: false,
      value,
      message: '设备别名包含非法字符，请使用中文、字母、数字、空格、短横线、下划线或括号',
    }
  }

  return { valid: true, value, message: '' }
}