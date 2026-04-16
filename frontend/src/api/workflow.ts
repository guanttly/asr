import type { ActiveWorkflowType, WorkflowSourceKind, WorkflowTargetKind, WorkflowType } from '@/types/workflow'

import request from './request'

export interface WorkflowNodePayload {
  id?: number
  node_type: string
  position: number
  config: Record<string, unknown>
  enabled: boolean
  is_fixed?: boolean
}

export interface WorkflowNodeTypeInfo {
  type: string
  label: string
  role?: 'source' | 'transform' | 'sink'
  description?: string
  default_config?: Record<string, unknown>
}

export interface GetWorkflowsParams {
  offset?: number
  limit?: number
  scope?: 'system' | 'user'
  workflow_type?: WorkflowType
  source_kind?: WorkflowSourceKind
  target_kind?: WorkflowTargetKind
  include_legacy?: boolean
}

/* ── 工作流 CRUD ── */

export function getWorkflows(params?: GetWorkflowsParams) {
  return request.get('/api/admin/workflows', { params })
}

export function createWorkflow(payload: { name: string, description?: string, source_id?: number, owner_type?: 'system' | 'user', workflow_type?: ActiveWorkflowType }) {
  return request.post('/api/admin/workflows', payload)
}

export function getWorkflow(id: number | string) {
  return request.get(`/api/admin/workflows/${id}`)
}

export function updateWorkflow(id: number | string, payload: { name?: string, description?: string, is_published?: boolean }) {
  return request.put(`/api/admin/workflows/${id}`, payload)
}

export function deleteWorkflow(id: number | string) {
  return request.delete(`/api/admin/workflows/${id}`)
}

/* ── 节点管理 ── */

export function updateWorkflowNodes(workflowId: number | string, nodes: WorkflowNodePayload[]) {
  return request.put(`/api/admin/workflows/${workflowId}/nodes`, { nodes })
}

/* ── 执行 & 测试 ── */

export function executeWorkflow(workflowId: number | string, payload: { input_text?: string, audio_url?: string } | FormData) {
  return request.post(`/api/admin/workflows/${workflowId}/execute`, payload, payload instanceof FormData
    ? {
        headers: {
          'Content-Type': 'multipart/form-data',
        },
        timeout: 0,
      }
    : undefined)
}

export function cloneWorkflow(workflowId: number | string) {
  return request.post(`/api/admin/workflows/${workflowId}/clone`)
}

export function testNode(payload: { node_type: string, config: Record<string, unknown>, input_text?: string, audio_url?: string } | FormData) {
  return request.post('/api/admin/workflows/test-node', payload, payload instanceof FormData
    ? {
        headers: {
          'Content-Type': 'multipart/form-data',
        },
        timeout: 0,
      }
    : undefined)
}

export interface TestNodeStreamEvent {
  type: 'status' | 'delta' | 'done'
  message?: string
  delta?: string
  output_text?: string
  detail?: Record<string, unknown> | string | null
  duration_ms?: number
  error?: string
}

function workflowAPIBaseURL() {
  return import.meta.env.DEV ? '' : (import.meta.env.VITE_API_BASE_URL || '')
}

function parseWorkflowErrorMessage(raw: string) {
  try {
    const payload = JSON.parse(raw) as { message?: string }
    if (typeof payload.message === 'string' && payload.message.trim())
      return payload.message
  }
  catch {
    // ignore invalid JSON body
  }
  return raw.trim() || '节点测试失败'
}

export async function testNodeStream(
  payload: { node_type: string, config: Record<string, unknown>, input_text?: string, audio_url?: string } | FormData,
  options: { onEvent?: (event: TestNodeStreamEvent) => void } = {},
) {
  const headers: Record<string, string> = {
    Accept: 'application/x-ndjson',
  }
  const token = localStorage.getItem('asr_token')
  if (token)
    headers.Authorization = `Bearer ${token}`

  let body: BodyInit
  if (payload instanceof FormData) {
    body = payload
  }
  else {
    headers['Content-Type'] = 'application/json'
    body = JSON.stringify(payload)
  }

  const response = await fetch(`${workflowAPIBaseURL()}/api/admin/workflows/test-node?stream=1`, {
    method: 'POST',
    headers,
    body,
  })
  if (!response.ok) {
    throw new Error(parseWorkflowErrorMessage(await response.text()))
  }
  if (!response.body)
    throw new Error('节点测试流为空')

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  while (true) {
    const { done, value } = await reader.read()
    buffer += decoder.decode(value || new Uint8Array(), { stream: !done })
    const lines = buffer.split('\n')
    buffer = lines.pop() || ''
    for (const line of lines) {
      const trimmed = line.trim()
      if (!trimmed)
        continue
      options.onEvent?.(JSON.parse(trimmed) as TestNodeStreamEvent)
    }
    if (done)
      break
  }

  const tail = buffer.trim()
  if (tail)
    options.onEvent?.(JSON.parse(tail) as TestNodeStreamEvent)
}

export function getNodeTypes() {
  return request.get('/api/admin/workflows/node-types')
}

export function updateNodeDefault(nodeType: string, payload: { config: Record<string, unknown> }) {
  return request.put(`/api/admin/workflows/node-defaults/${nodeType}`, payload)
}

/* ── 执行记录 ── */

export function getWorkflowExecution(executionId: number | string) {
  return request.get(`/api/admin/workflow-executions/${executionId}`)
}
