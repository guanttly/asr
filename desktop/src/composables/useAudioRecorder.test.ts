import { describe, expect, it } from 'vitest'

import { mapRecorderError } from './useAudioRecorder'

function namedError(name: string) {
  const error = new Error(name)
  error.name = name
  return error
}

describe('audio recorder error mapping', () => {
  it('turns browser microphone failures into user-facing Chinese prompts', () => {
    expect(mapRecorderError(namedError('NotAllowedError')).message).toContain('未授予麦克风权限')
    expect(mapRecorderError(namedError('NotFoundError')).message).toContain('未检测到可用麦克风设备')
    expect(mapRecorderError(namedError('NotReadableError')).message).toContain('麦克风当前被其他应用占用')
  })

  it('keeps unknown recorder errors inspectable', () => {
    const original = namedError('OverconstrainedError')
    expect(mapRecorderError(original)).toBe(original)
    expect(mapRecorderError('boom').message).toBe('初始化录音失败')
  })
})