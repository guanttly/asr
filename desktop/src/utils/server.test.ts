import { describe, expect, it } from 'vitest'

import { buildServerCandidates, describeNetworkError, normalizeServerUrl } from './server'

describe('server URL utilities', () => {
  it('normalizes default, protocol-less and trailing-slash addresses', () => {
    expect(normalizeServerUrl()).toBe('http://127.0.0.1:10010')
    expect(normalizeServerUrl('  192.168.1.1:8080/  ')).toBe('http://192.168.1.1:8080')
    expect(normalizeServerUrl('https://example.com///')).toBe('https://example.com')
  })

  it('tries an HTTP sibling after an HTTPS candidate on the packaged host and port', () => {
    expect(buildServerCandidates('https://127.0.0.1:10010/')).toEqual([
      'https://127.0.0.1:10010',
      'http://127.0.0.1:10010',
    ])
  })

  it('keeps user-provided remote hosts isolated from packaged fallbacks', () => {
    expect(buildServerCandidates('https://10.0.0.8:10010')).toEqual(['https://10.0.0.8:10010'])
  })

  it('describes current and fallback candidates for final failure prompts', () => {
    expect(describeNetworkError(new Error('ECONNREFUSED'), [
      'https://127.0.0.1:10010',
      'http://127.0.0.1:10010',
    ])).toContain('还尝试过 http://127.0.0.1:10010')
  })
})