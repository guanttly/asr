import { describe, expect, it } from 'vitest'

import { buildTextDiff } from './textDiff'

describe('buildTextDiff', () => {
  it('returns unchanged segments for identical text', () => {
    const result = buildTextDiff('影像报告', '影像报告')

    expect(result.changed).toBe(false)
    expect(result.addedCount).toBe(0)
    expect(result.removedCount).toBe(0)
    expect(result.beforeSegments).toEqual([{ kind: 'same', text: '影像报告' }])
    expect(result.afterSegments).toEqual([{ kind: 'same', text: '影像报告' }])
  })

  it('splits removed and added segments by character', () => {
    const result = buildTextDiff('肺部结节', '肺部小结节')

    expect(result.changed).toBe(true)
    expect(result.addedCount).toBe(1)
    expect(result.removedCount).toBe(0)
    expect(result.beforeSegments).toEqual([{ kind: 'same', text: '肺部结节' }])
    expect(result.afterSegments).toEqual([
      { kind: 'same', text: '肺部' },
      { kind: 'added', text: '小' },
      { kind: 'same', text: '结节' },
    ])
  })

  it('handles unicode code points without splitting surrogate pairs', () => {
    const result = buildTextDiff('A😀C', 'A😃C')

    expect(result.addedCount).toBe(1)
    expect(result.removedCount).toBe(1)
    expect(result.beforeSegments).toEqual([
      { kind: 'same', text: 'A' },
      { kind: 'removed', text: '😀' },
      { kind: 'same', text: 'C' },
    ])
    expect(result.afterSegments).toEqual([
      { kind: 'same', text: 'A' },
      { kind: 'added', text: '😃' },
      { kind: 'same', text: 'C' },
    ])
  })
})
