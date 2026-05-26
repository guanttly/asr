import { describe, expect, it } from 'vitest'

import { buildMeetingSummaryTokenBudget, buildNodeConfigOverrides, fallbackNodeDefaultConfig, formatConfigText, getNodeDefaultConfig, hasTextPlaceholder, normalizeNodeConfig } from './workflowNodeConfig'

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

  it('exposes meeting summary final and chunk prompts in defaults', () => {
    const config = fallbackNodeDefaultConfig('meeting_summary')

    expect(String(config.prompt_template)).toContain('{{TEXT}}')
    expect(String(config.chunk_prompt_template)).toContain('{{TEXT}}')
    expect(String(config.chunk_prompt_template)).toContain('会议片段')
  })

  it('keeps meeting summary chunk prompt overrides', () => {
    const overrides = buildNodeConfigOverrides('meeting_summary', {
      prompt_template: String(fallbackNodeDefaultConfig('meeting_summary').prompt_template),
      chunk_prompt_template: '自定义分片：{{Text}}',
      output_format: 'markdown',
      max_tokens: 100000,
    })

    expect(overrides).toEqual({
      chunk_prompt_template: '自定义分片：{{Text}}',
    })
  })

  it('restores meeting summary builtin prompts when draft prompts are blank', () => {
    const normalized = normalizeNodeConfig('meeting_summary', {
      prompt_template: '',
      chunk_prompt_template: '   ',
    })

    expect(String(normalized.prompt_template)).toContain('{{TEXT}}')
    expect(String(normalized.chunk_prompt_template)).toContain('会议片段')
  })

  it('estimates meeting summary token budget from prompts', () => {
    const budget = buildMeetingSummaryTokenBudget({
      prompt_template: '最终：{{TEXT}}',
      chunk_prompt_template: '分片：{{Text}}',
      max_tokens: 2048,
    })

    expect(budget.minimumInputTokens).toBeGreaterThan(2400)
    expect(budget.recommendedContextTokens).toBeGreaterThan(budget.minimumInputTokens)
    expect(hasTextPlaceholder('正文：{{Text}}')).toBe(true)
  })

  it('formats config as stable pretty json', () => {
    expect(formatConfigText({ model: 'qwen', max_tokens: 1024 })).toBe(`{
  "model": "qwen",
  "max_tokens": 1024
}`)
  })
})
