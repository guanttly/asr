import { authedFetch, ensureVoiceWorkflowBinding, readResponseEnvelope } from './auth'

export interface VoiceControlPayload {
  command_timeout_ms: number
  enabled: boolean
}

export interface IntentResult {
  wake_matched: boolean
  wake_word: string
  wake_alias: string
  matched: boolean
  intent: string
  group_key: string
  command_id: number
  confidence: number
  reason: string
  raw_output?: string
}

interface VoiceWakeResult {
  wake_matched: boolean
  wake_word: string
  wake_alias: string
  residue: string
  reason: string
}

interface VoiceWakeConfig {
  wake_words?: string[]
  homophone_words?: string[]
}

interface VoiceIntentConfig {
  include_base?: boolean
  dict_ids?: number[]
}

interface WorkflowNodeTypeInfo {
  type?: string
  default_config?: unknown
}

interface WorkflowNodePayload {
  node_type?: string
  config?: unknown
  enabled?: boolean
}

interface WorkflowPayload {
  id?: number
  nodes?: WorkflowNodePayload[]
}

interface VoiceCommandDictListPayload {
  items?: VoiceCommandDict[]
  total?: number
}

interface VoiceCommandDict {
  id: number
  name?: string
  group_key?: string
  is_base?: boolean
}

interface VoiceCommandEntry {
  id: number
  dict_id: number
  intent: string
  label?: string
  utterances?: string[]
  enabled?: boolean
  sort_order?: number
}

interface VoiceCommandCandidate {
  commandId: number
  groupKey: string
  intent: string
  label: string
  utterance: string
  normalized: string
  sortOrder: number
}

interface VoiceControlAssets {
  workflowId: number | null
  wakeConfig: VoiceWakeConfig
  commandCandidates: VoiceCommandCandidate[]
}

interface WakeCandidate {
  wakeWord: string
  alias: string
  normalized: string
}

const DEFAULT_WAKE_CONFIG: VoiceWakeConfig = {
  wake_words: ['你好小鲨'],
  homophone_words: ['你好小沙', '你好小莎', '你好小善'],
}

const VOICE_COMMAND_DICT_LIMIT = 1000
const COMMAND_MATCH_THRESHOLD = 0.72

let nodeDefaultWakePromise: Promise<VoiceWakeConfig> | null = null
let voiceControlAssetsPromise: Promise<VoiceControlAssets> | null = null
let voiceControlAssetsKey = ''

function createEmptyIntentResult(reason = '未命中有效指令'): IntentResult {
  return {
    wake_matched: false,
    wake_word: '',
    wake_alias: '',
    matched: false,
    intent: '',
    group_key: '',
    command_id: 0,
    confidence: 0,
    reason,
  }
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function normalizeNodeConfig(value: unknown) {
  if (isObject(value))
    return value

  if (typeof value === 'string') {
    try {
      const parsed = JSON.parse(value) as unknown
      if (isObject(parsed))
        return parsed
    }
    catch {
    }
  }

  return {}
}

function normalizeStringList(value: unknown): string[] {
  if (!Array.isArray(value))
    return []

  return value
    .map(item => typeof item === 'string' ? item.trim() : '')
    .filter(Boolean)
}

function normalizeNumberList(value: unknown): number[] {
  if (!Array.isArray(value))
    return []

  return value
    .map((item) => {
      const numeric = Number(item)
      return Number.isFinite(numeric) && numeric > 0 ? numeric : 0
    })
    .filter(item => item > 0)
}

function normalizeVoiceWakeConfig(value: unknown): VoiceWakeConfig {
  const config = normalizeNodeConfig(value)
  const wakeWords = normalizeStringList(config.wake_words)
  const homophoneWords = normalizeStringList(config.homophone_words)

  if (wakeWords.length === 0 && homophoneWords.length === 0)
    return { ...DEFAULT_WAKE_CONFIG }

  return {
    wake_words: wakeWords,
    homophone_words: homophoneWords,
  }
}

function normalizeVoiceIntentConfig(value: unknown): VoiceIntentConfig {
  const config = normalizeNodeConfig(value)
  return {
    include_base: config.include_base !== false,
    dict_ids: normalizeNumberList(config.dict_ids),
  }
}

async function fetchDefaultWakeConfig() {
  if (nodeDefaultWakePromise)
    return await nodeDefaultWakePromise

  nodeDefaultWakePromise = (async () => {
    const response = await authedFetch('/api/admin/workflows/node-types')
    const payload = await readResponseEnvelope<WorkflowNodeTypeInfo[]>(response)
    if (!response.ok)
      throw new Error(payload.message || '加载语音控制默认节点失败')

    const items = Array.isArray(payload.data) ? payload.data : []
    const wakeNode = items.find(item => item?.type === 'voice_wake')
    if (!wakeNode)
      return { ...DEFAULT_WAKE_CONFIG }

    return normalizeVoiceWakeConfig(wakeNode.default_config)
  })()

  try {
    return await nodeDefaultWakePromise
  }
  catch (error) {
    nodeDefaultWakePromise = null
    throw error
  }
}

async function fetchWorkflowPayload(workflowId: number) {
  const response = await authedFetch(`/api/admin/workflows/${workflowId}`)
  const payload = await readResponseEnvelope<WorkflowPayload>(response)
  if (!response.ok)
    throw new Error(payload.message || '加载语音控制工作流失败')
  return payload.data || null
}

function buildWakeCandidates(config: VoiceWakeConfig): WakeCandidate[] {
  const items: WakeCandidate[] = []
  const seen = new Set<string>()
  const wakeWords = normalizeStringList(config.wake_words)
  const homophoneWords = normalizeStringList(config.homophone_words)

  const appendCandidate = (value: string, wakeWord: string) => {
    const alias = value.trim()
    const normalized = normalizeLooseText(alias)
    if (!normalized || seen.has(normalized))
      return
    seen.add(normalized)
    items.push({ wakeWord, alias, normalized })
  }

  wakeWords.forEach((item) => {
    const trimmed = item.trim()
    appendCandidate(trimmed, trimmed)
  })

  const primaryWakeWord = wakeWords[0]?.trim() || ''
  homophoneWords.forEach(item => appendCandidate(item, primaryWakeWord))

  items.sort((left, right) => {
    if (left.normalized.length === right.normalized.length)
      return left.alias.localeCompare(right.alias)
    return right.normalized.length - left.normalized.length
  })
  return items
}

function sliceOriginalAfterNormalized(text: string, target: number) {
  const chars = Array.from(text)
  let consumed = 0

  for (let index = 0; index < chars.length; index += 1) {
    consumed += Array.from(normalizeLooseText(chars[index])).length
    if (consumed >= target)
      return chars.slice(0, index + 1).join('').length
  }

  return -1
}

function matchWakeCandidate(text: string, candidates: WakeCandidate[]): VoiceWakeResult {
  const normalizedText = normalizeLooseText(text)
  if (!normalizedText || candidates.length === 0) {
    return {
      wake_matched: false,
      wake_word: '',
      wake_alias: '',
      residue: text.trim(),
      reason: '未命中唤醒词',
    }
  }

  for (const candidate of candidates) {
    const matchIndex = normalizedText.indexOf(candidate.normalized)
    if (matchIndex < 0)
      continue

    const cutAt = sliceOriginalAfterNormalized(text, matchIndex + candidate.normalized.length)
    const residue = cutAt >= 0 ? text.slice(cutAt).trim() : ''
    return {
      wake_matched: true,
      wake_word: candidate.wakeWord,
      wake_alias: candidate.alias,
      residue,
      reason: residue ? '已命中唤醒词，并提取尾随指令' : '已命中唤醒词，等待后续指令',
    }
  }

  return {
    wake_matched: false,
    wake_word: '',
    wake_alias: '',
    residue: text.trim(),
    reason: '未命中唤醒词',
  }
}

function withWakeFields(result: IntentResult, wakeResult?: VoiceWakeResult | null): IntentResult {
  if (!wakeResult)
    return result

  return {
    ...result,
    wake_matched: wakeResult.wake_matched,
    wake_word: wakeResult.wake_word,
    wake_alias: wakeResult.wake_alias,
    reason: result.reason || wakeResult.reason,
  }
}

function normalizeLooseText(value: string) {
  return value
    .toLowerCase()
    .trim()
    .replace(/[\s，,。.!！?？、\-_(（）：:；;【】\[\]"'“”‘’]/g, '')
}

function buildCommandTextVariants(value: string) {
  const variants = new Set<string>()
  const base = normalizeLooseText(value)
  if (!base)
    return []

  const enqueue = (candidate: string) => {
    const normalized = normalizeLooseText(candidate)
    if (normalized)
      variants.add(normalized)
  }

  enqueue(base)
  enqueue(base.replace(/^(请你|请帮我|帮我|麻烦你|麻烦|给我|请)/, ''))
  enqueue(base.replace(/[吧呀啊呢啦嘛哦]$/g, ''))
  enqueue(base.replace(/^(切换到|切换成|切到|切成|改成|改到|进入|开始)/, ''))
  enqueue(base.replace(/^(请你|请帮我|帮我|麻烦你|麻烦|给我|请)(切换到|切换成|切到|切成|改成|改到|进入|开始)/, ''))

  return Array.from(variants)
}

function levenshteinDistance(left: string, right: string) {
  const rows = left.length + 1
  const cols = right.length + 1
  const matrix = Array.from({ length: rows }, () => Array<number>(cols).fill(0))

  for (let row = 0; row < rows; row += 1)
    matrix[row][0] = row
  for (let col = 0; col < cols; col += 1)
    matrix[0][col] = col

  for (let row = 1; row < rows; row += 1) {
    for (let col = 1; col < cols; col += 1) {
      const cost = left[row - 1] === right[col - 1] ? 0 : 1
      matrix[row][col] = Math.min(
        matrix[row - 1][col] + 1,
        matrix[row][col - 1] + 1,
        matrix[row - 1][col - 1] + cost,
      )
    }
  }

  return matrix[rows - 1][cols - 1]
}

function scoreNormalizedMatch(text: string, candidate: string) {
  if (!text || !candidate)
    return 0
  if (text === candidate)
    return 1
  if (text.includes(candidate))
    return Math.min(0.99, 0.92 + Math.min(candidate.length / text.length, 1) * 0.07)
  if (text.length >= 2 && candidate.includes(text))
    return 0.84 + Math.min(text.length / candidate.length, 1) * 0.08
  if (Math.abs(text.length - candidate.length) > 4)
    return 0

  const distance = levenshteinDistance(text, candidate)
  const similarity = 1 - distance / Math.max(text.length, candidate.length)
  return similarity >= COMMAND_MATCH_THRESHOLD ? similarity * 0.9 : 0
}

function roundConfidence(value: number) {
  return Math.round(value * 100) / 100
}

async function fetchVoiceCommandDicts() {
  const response = await authedFetch(`/api/admin/voice-command-dicts?offset=0&limit=${VOICE_COMMAND_DICT_LIMIT}`)
  const payload = await readResponseEnvelope<VoiceCommandDictListPayload>(response)
  if (!response.ok)
    throw new Error(payload.message || '加载控制指令组失败')
  return Array.isArray(payload.data?.items) ? payload.data.items : []
}

async function fetchVoiceCommandEntries(dictId: number) {
  const response = await authedFetch(`/api/admin/voice-command-dicts/${dictId}/entries`)
  const payload = await readResponseEnvelope<VoiceCommandEntry[]>(response)
  if (!response.ok)
    throw new Error(payload.message || `加载控制指令失败: ${dictId}`)
  return Array.isArray(payload.data) ? payload.data : []
}

function selectVoiceCommandDicts(dicts: VoiceCommandDict[], config: VoiceIntentConfig) {
  const includeBase = config.include_base !== false
  const selectedIds = new Set(config.dict_ids || [])
  const items = dicts.filter(dict => (includeBase && dict.is_base === true) || selectedIds.has(dict.id))
  if (items.length > 0)
    return items
  return dicts.filter(dict => dict.is_base === true)
}

function buildVoiceCommandCandidates(dicts: VoiceCommandDict[], entries: VoiceCommandEntry[]) {
  const seen = new Set<string>()
  const dictById = new Map(dicts.map(dict => [dict.id, dict]))
  const items: VoiceCommandCandidate[] = []

  entries.forEach((entry, entryIndex) => {
    if (entry.enabled === false)
      return
    const dict = dictById.get(entry.dict_id)
    if (!dict)
      return
    const groupKey = String(dict.group_key || '').trim()
    const intent = String(entry.intent || '').trim()
    if (!groupKey || !intent)
      return

    normalizeStringList(entry.utterances).forEach((utterance, utteranceIndex) => {
      const normalized = normalizeLooseText(utterance)
      if (!normalized)
        return

      const dedupeKey = `${groupKey}:${intent}:${normalized}`
      if (seen.has(dedupeKey))
        return
      seen.add(dedupeKey)
      items.push({
        commandId: entry.id,
        groupKey,
        intent,
        label: String(entry.label || utterance).trim() || utterance,
        utterance,
        normalized,
        sortOrder: typeof entry.sort_order === 'number' ? entry.sort_order : entryIndex * 100 + utteranceIndex,
      })
    })
  })

  items.sort((left, right) => {
    if (left.normalized.length === right.normalized.length) {
      if (left.sortOrder === right.sortOrder)
        return left.intent.localeCompare(right.intent)
      return left.sortOrder - right.sortOrder
    }
    return right.normalized.length - left.normalized.length
  })

  return items
}

async function fetchCommandCandidates(config: VoiceIntentConfig) {
  try {
    const dicts = await fetchVoiceCommandDicts()
    const selectedDicts = selectVoiceCommandDicts(dicts, config)
    if (selectedDicts.length === 0)
      return []

    const entryGroups = await Promise.all(selectedDicts.map(dict => fetchVoiceCommandEntries(dict.id)))
    return buildVoiceCommandCandidates(selectedDicts, entryGroups.flat())
  }
  catch {
    return []
  }
}

async function loadVoiceControlAssets(force = false) {
  const workflowId = await ensureVoiceWorkflowBinding(force)
  const cacheKey = String(workflowId ?? 'default')
  if (!force && voiceControlAssetsPromise && voiceControlAssetsKey === cacheKey)
    return await voiceControlAssetsPromise

  voiceControlAssetsKey = cacheKey
  voiceControlAssetsPromise = (async () => {
    const defaultWakeConfig = await fetchDefaultWakeConfig().catch(() => ({ ...DEFAULT_WAKE_CONFIG }))
    let wakeConfig = defaultWakeConfig
    let intentConfig: VoiceIntentConfig = { include_base: true, dict_ids: [] }

    if (workflowId != null) {
      try {
        const workflow = await fetchWorkflowPayload(workflowId)
        const nodes = Array.isArray(workflow?.nodes) ? workflow.nodes : []
        const wakeNode = nodes.find(node => node?.node_type === 'voice_wake' && node.enabled !== false)
        const intentNode = nodes.find(node => node?.node_type === 'voice_intent' && node.enabled !== false)

        if (wakeNode)
          wakeConfig = normalizeVoiceWakeConfig(wakeNode.config)
        if (intentNode)
          intentConfig = normalizeVoiceIntentConfig(intentNode.config)
      }
      catch {
      }
    }

    const commandCandidates = await fetchCommandCandidates(intentConfig)
    return {
      workflowId,
      wakeConfig,
      commandCandidates,
    } satisfies VoiceControlAssets
  })()

  try {
    return await voiceControlAssetsPromise
  }
  catch (error) {
    voiceControlAssetsPromise = null
    voiceControlAssetsKey = ''
    throw error
  }
}

function matchLocalCommandIntent(text: string, candidates: VoiceCommandCandidate[]) {
  const variants = buildCommandTextVariants(text)
  if (variants.length === 0 || candidates.length === 0)
    return createEmptyIntentResult('未命中控制指令')

  let bestCandidate: VoiceCommandCandidate | null = null
  let bestScore = 0

  for (const candidate of candidates) {
    let score = 0
    for (const variant of variants)
      score = Math.max(score, scoreNormalizedMatch(variant, candidate.normalized))

    if (score > bestScore || (score === bestScore && bestCandidate && candidate.normalized.length > bestCandidate.normalized.length)) {
      bestScore = score
      bestCandidate = candidate
    }
  }

  if (!bestCandidate || bestScore < COMMAND_MATCH_THRESHOLD)
    return createEmptyIntentResult('未命中控制指令')

  return {
    wake_matched: false,
    wake_word: '',
    wake_alias: '',
    matched: true,
    intent: bestCandidate.intent,
    group_key: bestCandidate.groupKey,
    command_id: bestCandidate.commandId,
    confidence: roundConfidence(bestScore),
    reason: bestScore >= 0.95 ? `已命中指令：${bestCandidate.label}` : `已匹配近义指令：${bestCandidate.label}`,
    raw_output: bestCandidate.utterance,
  } satisfies IntentResult
}

export async function primeVoiceControlAssets(force = false) {
  await loadVoiceControlAssets(force)
}

export async function fetchVoiceControl() {
  const response = await authedFetch('/api/admin/app-settings/voice-control')
  const payload = await readResponseEnvelope<VoiceControlPayload>(response)
  if (!response.ok)
    throw new Error(payload.message || '加载语音控制配置失败')
  return payload.data || null
}

export async function updateVoiceControl(body: { command_timeout_ms?: number, enabled?: boolean }) {
  const response = await authedFetch('/api/admin/app-settings/voice-control', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  const payload = await readResponseEnvelope<VoiceControlPayload>(response)
  if (!response.ok)
    throw new Error(payload.message || '保存语音控制配置失败')
  return payload.data || null
}

export async function classifyVoiceIntent(text: string, options?: { bypassWake?: boolean, timeoutMs?: number }) {
  const assets = await loadVoiceControlAssets(false)

  if (options?.bypassWake)
    return matchLocalCommandIntent(text, assets.commandCandidates)

  const wakeResult = matchWakeCandidate(text, buildWakeCandidates(assets.wakeConfig))
  if (!wakeResult.wake_matched)
    return withWakeFields(createEmptyIntentResult(wakeResult.reason || '未命中唤醒词'), wakeResult)

  if (!wakeResult.residue)
    return withWakeFields(createEmptyIntentResult(wakeResult.reason || '已命中唤醒词，等待后续指令'), wakeResult)

  return withWakeFields(matchLocalCommandIntent(wakeResult.residue, assets.commandCandidates), wakeResult)
}
