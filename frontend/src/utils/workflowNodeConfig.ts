export interface WorkflowNodeTypeDefaultLike {
  type: string
  default_config?: Record<string, unknown>
}

interface RegexRule {
  pattern: string
  replacement: string
  enabled: boolean
}

export const DEFAULT_LLM_CORRECTION_PROMPT = `你是一个专业的语音转写文本校对助手。请只修正语音识别造成的错别字、同音误识别、标点和明显语序问题，保持原意、语气、人名、数字和专业术语不变。

要求：
1. 只输出纠错后的正文，不要解释、不要标题、不要列表。
2. 如果原文为空或只有空白，直接输出空字符串，不要补充提示语。
3. 无法确定的内容保持原样，不要编造。

原文：
{{TEXT}}`

export const DEFAULT_MEETING_SUMMARY_PROMPT = `# 角色
你是一位资深的会议纪要撰写专家。你的任务是将语音转写的原始文本整理为清晰、专业、可直接用于存档和分发的结构化会议纪要。

# 输出格式要求
- 使用 Markdown 格式输出
- 严格按照下方模板的标题层级和顺序组织内容
- 每个板块如果没有相关内容，省略板块
- 要点使用无序列表（- ），待办和决议使用有序列表（1. ）
- 语言简洁精炼，去除口语化表述和重复内容
- 不要输出任何解释、前言或结尾寒暄

# 输出模板

## 📋 会议概要
> 用 2-3 句话概括本次会议的主题、目的和整体结论。

## 📌 讨论要点
- **议题 1**：核心结论或讨论结果
- **议题 2**：核心结论或讨论结果
- ...（按讨论顺序列出）

## ✅ 决议事项
1. 【决议内容】
2. ...（如无明确决议写"无"）

## 📝 待办事项
| 序号 | 待办内容 | 责任人 | 截止时间 |
|------|----------|--------|----------|
| 1 | 具体任务描述 | 从文本中提取，未提及写"待定" | 未提及写"待定" |

## 💡 补充说明
- 会议中提到但未形成结论的开放性问题或风险点（如无写"无"）

---

以下是需要整理的会议转写文本：

{{TEXT}}`

export const DEFAULT_MEETING_SUMMARY_CHUNK_PROMPT = `# 角色
你是一位会议纪要助手，正在对一段较长会议的某个片段做关键信息提炼。

# 要求
- 输出 Markdown 格式
- 只输出该片段的信息，不要推测片段外的内容
- 语言精炼，去除口语化和重复表述
- 不要输出任何解释或前后文说明

# 输出结构

### 本段摘要
> 1-2 句话概括本片段讨论的核心内容。

### 讨论要点
- **要点**：结论或讨论结果

### 决议与待办
- 本片段中明确提到的决议或待办（如无写"无"）

### 关键信息
- 提及的人名、时间节点、数据指标等（如无写"无"）

---

以下是需要提炼的会议片段：

{{TEXT}}`

const TEXT_PLACEHOLDER_PATTERN = /\{\{\s*text\s*\}\}/i
const MEETING_SUMMARY_BODY_TOKEN_RESERVE = 2400
const MEETING_SUMMARY_MERGED_TOKEN_RESERVE = 1600
const MEETING_SUMMARY_CHUNK_OUTPUT_TOKEN_RESERVE = 1024
const MEETING_SUMMARY_FINAL_OUTPUT_TOKEN_RESERVE = 4096

export interface MeetingSummaryTokenBudget {
  finalPromptTokens: number
  chunkPromptTokens: number
  bodyReserveTokens: number
  mergedReserveTokens: number
  minimumInputTokens: number
  recommendedContextTokens: number
  currentRequestContextTokens: number
  recommendedOutputTokens: number
  currentOutputTokens: number
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

function ensureNonEmptyString(value: unknown, fallback = '') {
  const text = String(value ?? '')
  return text.trim() ? text : fallback
}

export function hasTextPlaceholder(value: unknown) {
  return TEXT_PLACEHOLDER_PATTERN.test(String(value || ''))
}

function estimateTextTokens(value: string) {
  const text = value.trim()
  if (!text)
    return 0

  let cjk = 0
  let ascii = 0
  let other = 0
  for (const char of Array.from(text)) {
    const code = char.codePointAt(0) || 0
    if ((code >= 0x3400 && code <= 0x9FFF) || (code >= 0xF900 && code <= 0xFAFF)) {
      cjk += 1
    }
    else if (code < 0x80) {
      ascii += 1
    }
    else {
      other += 1
    }
  }
  return Math.max(1, Math.ceil(cjk * 1.1 + other * 0.8 + ascii / 4))
}

function promptOverheadTokens(template: string) {
  const withoutText = template.replace(/\{\{\s*text\s*\}\}/gi, '')
  const fallbackTextLabelTokens = hasTextPlaceholder(template) ? 0 : estimateTextTokens('\n\n原文：\n')
  return estimateTextTokens(withoutText) + fallbackTextLabelTokens
}

function roundUpTokens(value: number, step = 128) {
  return Math.ceil(value / step) * step
}

export function buildMeetingSummaryTokenBudget(config: Record<string, unknown>): MeetingSummaryTokenBudget {
  const finalPrompt = String(config.prompt_template || DEFAULT_MEETING_SUMMARY_PROMPT)
  const chunkPrompt = String(config.chunk_prompt_template || DEFAULT_MEETING_SUMMARY_CHUNK_PROMPT)
  const finalPromptTokens = promptOverheadTokens(finalPrompt)
  const chunkPromptTokens = promptOverheadTokens(chunkPrompt)
  const currentOutputTokens = Math.max(1, ensureNumber(config.max_tokens, 100000))
  const minimumInputTokens = Math.max(
    chunkPromptTokens + MEETING_SUMMARY_BODY_TOKEN_RESERVE,
    finalPromptTokens + MEETING_SUMMARY_BODY_TOKEN_RESERVE,
    finalPromptTokens + MEETING_SUMMARY_MERGED_TOKEN_RESERVE,
  )
  const recommendedContextTokens = Math.max(
    chunkPromptTokens + MEETING_SUMMARY_BODY_TOKEN_RESERVE + MEETING_SUMMARY_CHUNK_OUTPUT_TOKEN_RESERVE,
    finalPromptTokens + MEETING_SUMMARY_BODY_TOKEN_RESERVE + MEETING_SUMMARY_FINAL_OUTPUT_TOKEN_RESERVE,
    finalPromptTokens + MEETING_SUMMARY_MERGED_TOKEN_RESERVE + MEETING_SUMMARY_FINAL_OUTPUT_TOKEN_RESERVE,
  )
  const currentRequestContextTokens = Math.max(
    recommendedContextTokens,
    finalPromptTokens + MEETING_SUMMARY_BODY_TOKEN_RESERVE + currentOutputTokens,
    finalPromptTokens + MEETING_SUMMARY_MERGED_TOKEN_RESERVE + currentOutputTokens,
  )

  return {
    finalPromptTokens: roundUpTokens(finalPromptTokens, 16),
    chunkPromptTokens: roundUpTokens(chunkPromptTokens, 16),
    bodyReserveTokens: MEETING_SUMMARY_BODY_TOKEN_RESERVE,
    mergedReserveTokens: MEETING_SUMMARY_MERGED_TOKEN_RESERVE,
    minimumInputTokens: roundUpTokens(minimumInputTokens),
    recommendedContextTokens: roundUpTokens(recommendedContextTokens),
    currentRequestContextTokens: roundUpTokens(currentRequestContextTokens),
    recommendedOutputTokens: MEETING_SUMMARY_FINAL_OUTPUT_TOKEN_RESERVE,
    currentOutputTokens,
  }
}

function ensureNumberArray(value: unknown) {
  if (!Array.isArray(value))
    return [] as number[]
  return value
    .map(item => Number(item))
    .filter(item => Number.isFinite(item) && item > 0)
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
    case 'voice_wake':
      return { wake_words: ['你好小鲨'], homophone_words: ['你好小沙', '你好小莎', '你好小善'] }
    case 'term_correction':
      return { dict_id: 0 }
    case 'filler_filter':
      return { dict_id: 0, filter_words: [], custom_words: [] }
    case 'sensitive_filter':
      return { dict_id: 0, custom_words: [], replacement: '[已过滤]' }
    case 'llm_correction':
      return { endpoint: '', model: '', api_key: '', prompt_template: DEFAULT_LLM_CORRECTION_PROMPT, temperature: 0.3, max_tokens: 4096, allow_markdown: false }
    case 'voice_intent':
      return { enable_llm: false, endpoint: '', model: '', api_key: '', prompt_template: '', extra_prompt: '', temperature: 0, max_tokens: 512, include_base: true, dict_ids: [] }
    case 'speaker_diarize':
      return { service_url: '', enable_voiceprint_match: false, fail_on_error: false }
    case 'meeting_summary':
      return { endpoint: '', model: '', api_key: '', prompt_template: DEFAULT_MEETING_SUMMARY_PROMPT, chunk_prompt_template: DEFAULT_MEETING_SUMMARY_CHUNK_PROMPT, output_format: 'markdown', max_tokens: 100000 }
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
    case 'voice_wake':
      return {
        ...base,
        ...raw,
        wake_words: ensureStringArray(raw.wake_words, ensureStringArray(base.wake_words)),
        homophone_words: ensureStringArray(raw.homophone_words, ensureStringArray(base.homophone_words)),
      }
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
        dict_id: ensureNumber(raw.dict_id, ensureNumber(base.dict_id, 0)),
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
    case 'voice_intent':
      return {
        ...base,
        ...raw,
        enable_llm: ensureBoolean(raw.enable_llm, ensureBoolean(base.enable_llm, false)),
        endpoint: String(raw.endpoint ?? base.endpoint ?? ''),
        model: String(raw.model ?? base.model ?? ''),
        api_key: String(raw.api_key ?? base.api_key ?? ''),
        prompt_template: String(raw.prompt_template ?? base.prompt_template ?? ''),
        extra_prompt: String(raw.extra_prompt ?? base.extra_prompt ?? ''),
        temperature: ensureNumber(raw.temperature, ensureNumber(base.temperature, 0)),
        max_tokens: ensureNumber(raw.max_tokens, ensureNumber(base.max_tokens, 512)),
        include_base: ensureBoolean(raw.include_base, ensureBoolean(base.include_base, true)),
        dict_ids: ensureNumberArray(raw.dict_ids ?? base.dict_ids),
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
        prompt_template: ensureNonEmptyString(raw.prompt_template, String(base.prompt_template || DEFAULT_MEETING_SUMMARY_PROMPT)),
        chunk_prompt_template: ensureNonEmptyString(raw.chunk_prompt_template, String(base.chunk_prompt_template || DEFAULT_MEETING_SUMMARY_CHUNK_PROMPT)),
        output_format: String(raw.output_format ?? base.output_format ?? 'markdown'),
        max_tokens: ensureNumber(raw.max_tokens, ensureNumber(base.max_tokens, 100000)),
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
