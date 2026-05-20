import { describe, expect, it } from 'vitest'

import { buildNodeConfigOverrides, fallbackNodeDefaultConfig, formatConfigText, getNodeDefaultConfig, normalizeNodeConfig } from './workflowNodeConfig'

describe('workflow node config helpers', () => {
  it('merges server defaults without mutating fallback config', () => {
    const merged = getNodeDefaultConfig('llm_correction', [
      {
        type: 'llm_correction',
        default_config: {
          model: 'qwen-plus',
          temperature: 0.1,
          nested: { enabled: true },
        },
      },
    ])

    expect(merged).toMatchObject({
      model: 'qwen-plus',
      temperature: 0.1,
      allow_markdown: false,
      nested: { enabled: true },
    })
    expect(fallbackNodeDefaultConfig('llm_correction')).not.toHaveProperty('nested')
  })

  it('normalizes sensitive filter legacy words into custom words', () => {
    const normalized = normalizeNodeConfig('sensitive_filter', {
      dict_id: '9',
      words: ['身份证', '  ', '电话'],
      replacement: 123,
    })

    expect(normalized).toMatchObject({
      dict_id: 9,
      custom_words: ['身份证', '电话'],
      replacement: '123',
    })
    expect(normalized).not.toHaveProperty('words')
  })

  it('keeps only changed overrides for persisted workflow nodes', () => {
    const overrides = buildNodeConfigOverrides('voice_intent', {
      enable_llm: true,
      include_base: true,
      dict_ids: ['1', 2, 0, -1, 'bad'],
      max_tokens: 512,
    })

    expect(overrides).toEqual({
      enable_llm: true,
      dict_ids: [1, 2],
    })
  })

  it('formats config as stable pretty json', () => {
    expect(formatConfigText({ model: 'qwen', max_tokens: 1024 })).toBe(`{
  "model": "qwen",
  "max_tokens": 1024
}`)
  })
})
