export interface WorkflowNodeTypeDefaultLike {
  type: string
  default_config?: Record<string, unknown>
}

interface RegexRule {
  pattern: string
  replacement: string
  enabled: boolean
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return Object.prototype.toString.call(value) === '[object Object]'
}

function cloneValue<T>(value: T): T {
  if (Array.isArray(value))
    return value.map(item => cloneValue(item)) as T
  if (isPlainObject(value)) {
    const next: Record<string, unknown> = {}
    Object.entries(value).forEach(([key, item]) => {
      next[key] = cloneValue(item)
    })
    return next as T
  }
  return value
}

function mergeObjects(base: Record<string, unknown>, override: Record<string, unknown>) {
  const result: Record<string, unknown> = cloneValue(base)
  Object.entries(override).forEach(([key, value]) => {
    const current = result[key]
    if (isPlainObject(current) && isPlainObject(value)) {
      result[key] = mergeObjects(current, value)
      return
    }
    result[key] = cloneValue(value)
  })
  return result
}

function ensureStringArray(value: unknown, fallback: string[] = []) {
  if (!Array.isArray(value))
    return [...fallback]
  return value.map(item => String(item).trim()).filter(Boolean)
}

function ensureBoolean(value: unknown, fallback = false) {
  return typeof value === 'boolean' ? value : fallback
}

function ensureNumber(value: unknown, fallback = 0) {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : fallback
}

function ensureRegexRules(value: unknown) {
  if (!Array.isArray(value))
    return [{ pattern: '', replacement: '', enabled: true }]

  const rules = value.map((item): RegexRule => ({
    pattern: String((item as Record<string, unknown>)?.pattern || ''),
    replacement: String((item as Record<string, unknown>)?.replacement || ''),
    enabled: ensureBoolean((item as Record<string, unknown>)?.enabled, true),
  }))

  return rules.length > 0 ? rules : [{ pattern: '', replacement: '', enabled: true }]
}

export function fallbackNodeDefaultConfig(type: string): Record<string, unknown> {
  switch (type) {
    case 'term_correction':
      return { dict_id: 0 }
    case 'filler_filter':
      return { filter_words: ['嗯', '啊', '呃', '那个', '就是', '然后'], custom_words: [] }
    case 'sensitive_filter':
      return { dict_id: 0, custom_words: [], replacement: '[已过滤]' }
    case 'llm_correction':
      return { endpoint: '', model: '', api_key: '', prompt_template: '', temperature: 0.3, max_tokens: 4096, allow_markdown: false }
    case 'speaker_diarize':
      return { service_url: '', enable_voiceprint_match: false, fail_on_error: false }
    case 'meeting_summary':
      return { endpoint: '', model: '', api_key: '', prompt_template: '', output_format: 'markdown', max_tokens: 65536 }
    case 'custom_regex':
      return { rules: [{ pattern: '', replacement: '', enabled: true }] }
    default:
      return {}
  }
}

export function getNodeDefaultConfig(type: string, nodeTypes?: WorkflowNodeTypeDefaultLike[]) {
  const fallback = fallbackNodeDefaultConfig(type)
  const fromServer = nodeTypes?.find(item => item.type === type)?.default_config
  if (!fromServer || !isPlainObject(fromServer))
    return cloneValue(fallback)
  return mergeObjects(fallback, fromServer)
}

export function normalizeNodeConfig(type: string, raw: Record<string, unknown>, defaults?: Record<string, unknown>) {
  const base = mergeObjects(fallbackNodeDefaultConfig(type), defaults || {})
  switch (type) {
    case 'term_correction':
      return {
        ...base,
        ...raw,
        dict_id: ensureNumber(raw.dict_id, ensureNumber(base.dict_id, 0)),
      }
    case 'filler_filter':
      return {
        ...base,
        ...raw,
        filter_words: ensureStringArray(raw.filter_words, ensureStringArray(base.filter_words)),
        custom_words: ensureStringArray(raw.custom_words, ensureStringArray(base.custom_words)),
      }
    case 'sensitive_filter':
      return {
        ...base,
        ...Object.fromEntries(Object.entries(raw).filter(([key]) => key !== 'words')),
        dict_id: ensureNumber(raw.dict_id, ensureNumber(base.dict_id, 0)),
        custom_words: ensureStringArray(raw.custom_words, ensureStringArray(raw.words, ensureStringArray(base.custom_words))),
        replacement: String(raw.replacement ?? base.replacement ?? '[已过滤]'),
      }
    case 'llm_correction':
      return {
        ...base,
        ...raw,
        endpoint: String(raw.endpoint ?? base.endpoint ?? ''),
        model: String(raw.model ?? base.model ?? ''),
        api_key: String(raw.api_key ?? base.api_key ?? ''),
        prompt_template: String(raw.prompt_template ?? base.prompt_template ?? ''),
        temperature: ensureNumber(raw.temperature, ensureNumber(base.temperature, 0.3)),
        max_tokens: ensureNumber(raw.max_tokens, ensureNumber(base.max_tokens, 4096)),
        allow_markdown: ensureBoolean(raw.allow_markdown, ensureBoolean(base.allow_markdown, false)),
      }
    case 'speaker_diarize':
      return {
        ...base,
        ...raw,
        service_url: String(raw.service_url ?? base.service_url ?? ''),
        enable_voiceprint_match: ensureBoolean(raw.enable_voiceprint_match, ensureBoolean(base.enable_voiceprint_match, false)),
        fail_on_error: ensureBoolean(raw.fail_on_error, ensureBoolean(base.fail_on_error, false)),
      }
    case 'meeting_summary':
      return {
        ...base,
        ...raw,
        endpoint: String(raw.endpoint ?? base.endpoint ?? ''),
        model: String(raw.model ?? base.model ?? ''),
        api_key: String(raw.api_key ?? base.api_key ?? ''),
        prompt_template: String(raw.prompt_template ?? base.prompt_template ?? ''),
        output_format: String(raw.output_format ?? base.output_format ?? 'markdown'),
        max_tokens: ensureNumber(raw.max_tokens, ensureNumber(base.max_tokens, 65536)),
      }
    case 'custom_regex':
      return {
        ...base,
        ...raw,
        rules: ensureRegexRules(raw.rules ?? base.rules),
      }
    default:
      return mergeObjects(base, raw)
  }
}

function areEqual(left: unknown, right: unknown): boolean {
  if (left === right)
    return true

  if (Array.isArray(left) && Array.isArray(right)) {
    if (left.length !== right.length)
      return false
    return left.every((item, index) => areEqual(item, right[index]))
  }

  if (isPlainObject(left) && isPlainObject(right)) {
    const leftKeys = Object.keys(left)
    const rightKeys = Object.keys(right)
    if (leftKeys.length !== rightKeys.length)
      return false
    return leftKeys.every(key => areEqual(left[key], right[key]))
  }

  return false
}

function stripDefaultValues(defaultValue: unknown, currentValue: unknown): unknown {
  if (areEqual(defaultValue, currentValue))
    return undefined

  if (isPlainObject(defaultValue) && isPlainObject(currentValue)) {
    const result: Record<string, unknown> = {}
    Object.keys(currentValue).forEach((key) => {
      const next = stripDefaultValues(defaultValue[key], currentValue[key])
      if (next !== undefined)
        result[key] = next
    })
    return Object.keys(result).length > 0 ? result : undefined
  }

  return cloneValue(currentValue)
}

export function buildNodeConfigOverrides(type: string, raw: Record<string, unknown>, defaults?: Record<string, unknown>) {
  const mergedDefaults = getNodeDefaultConfig(type, defaults ? [{ type, default_config: defaults }] : undefined)
  const normalizedDefaults = normalizeNodeConfig(type, mergedDefaults, mergedDefaults)
  const normalizedCurrent = normalizeNodeConfig(type, raw, mergedDefaults)
  const diff = stripDefaultValues(normalizedDefaults, normalizedCurrent)
  return isPlainObject(diff) ? diff : {}
}

export function formatConfigText(config: Record<string, unknown>) {
  return JSON.stringify(config, null, 2)
}
