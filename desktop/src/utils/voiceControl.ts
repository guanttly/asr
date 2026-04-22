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

interface WorkflowExecutionNodeResult {
  output_text?: string
}

interface WorkflowExecutionPayload {
  final_text?: string
  status?: string
  error_message?: string
  node_results?: WorkflowExecutionNodeResult[]
}

const VOICE_COMMAND_BYPASS_PREFIX = '__voice_command__:'

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

function normalizeIntentResult(value: unknown): IntentResult {
  if (!isObject(value))
    return createEmptyIntentResult('语音控制工作流返回了无效结果')

  return {
    wake_matched: value.wake_matched === true,
    wake_word: typeof value.wake_word === 'string' ? value.wake_word.trim() : '',
    wake_alias: typeof value.wake_alias === 'string' ? value.wake_alias.trim() : '',
    matched: value.matched === true,
    intent: typeof value.intent === 'string' ? value.intent.trim() : '',
    group_key: typeof value.group_key === 'string' ? value.group_key.trim() : '',
    command_id: typeof value.command_id === 'number' && Number.isFinite(value.command_id) ? value.command_id : 0,
    confidence: typeof value.confidence === 'number' && Number.isFinite(value.confidence) ? value.confidence : 0,
    reason: typeof value.reason === 'string' && value.reason.trim() ? value.reason.trim() : '未命中有效指令',
    raw_output: typeof value.raw_output === 'string' && value.raw_output.trim() ? value.raw_output : undefined,
  }
}

function parseIntentResult(raw: unknown) {
  if (isObject(raw))
    return normalizeIntentResult(raw)

  if (typeof raw === 'string') {
    const trimmed = raw.trim()
    if (!trimmed)
      return createEmptyIntentResult('语音控制工作流未返回结果')

    try {
      return normalizeIntentResult(JSON.parse(trimmed) as unknown)
    }
    catch {
      return createEmptyIntentResult(trimmed)
    }
  }

  return createEmptyIntentResult('语音控制工作流未返回结果')
}

function resolveWorkflowOutput(payload?: WorkflowExecutionPayload | null) {
  if (typeof payload?.final_text === 'string' && payload.final_text.trim())
    return payload.final_text

  const nodeResults = Array.isArray(payload?.node_results) ? payload.node_results : []
  for (let index = nodeResults.length - 1; index >= 0; index -= 1) {
    const outputText = nodeResults[index]?.output_text
    if (typeof outputText === 'string' && outputText.trim())
      return outputText
  }

  return ''
}

function createTimeoutSignal(timeoutMs?: number) {
  if (!timeoutMs || timeoutMs <= 0 || typeof AbortController === 'undefined')
    return { signal: undefined, cleanup: () => {} }

  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), timeoutMs)
  return {
    signal: controller.signal,
    cleanup: () => clearTimeout(timer),
  }
}

export async function primeVoiceControlAssets(force = false) {
  await ensureVoiceWorkflowBinding(force)
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
  const inputText = (text || '').trim()
  if (!inputText)
    return createEmptyIntentResult('输入为空')

  const workflowId = await ensureVoiceWorkflowBinding(false)
  if (workflowId == null)
    throw new Error('未绑定语音控制工作流')

  const requestText = options?.bypassWake ? `${VOICE_COMMAND_BYPASS_PREFIX}${inputText}` : inputText
  const { signal, cleanup } = createTimeoutSignal(options?.timeoutMs)

  try {
    const response = await authedFetch(`/api/admin/workflows/${workflowId}/execute`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ input_text: requestText }),
      signal,
    })
    const payload = await readResponseEnvelope<WorkflowExecutionPayload>(response)
    if (!response.ok)
      throw new Error(payload.message || '执行语音控制工作流失败')

    if (payload.data?.status === 'failed')
      throw new Error(payload.data.error_message || payload.message || '语音控制工作流执行失败')

    return parseIntentResult(resolveWorkflowOutput(payload.data || null))
  }
  catch (error) {
    if (error instanceof Error && error.name === 'AbortError')
      throw new Error('语音控制工作流请求超时')
    throw error
  }
  finally {
    cleanup()
  }
}
