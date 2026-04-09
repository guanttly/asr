import type { WorkflowSourceKind, WorkflowTargetKind, WorkflowType } from '@/types/workflow'

import request from './request'

export interface WorkflowNodePayload {
  id?: number
  node_type: string
  position: number
  config: Record<string, unknown>
  enabled: boolean
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

export function createWorkflow(payload: { name: string, description?: string, source_id?: number, owner_type?: 'system' | 'user' }) {
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

export function executeWorkflow(workflowId: number | string, payload: { input_text: string, audio_url?: string }) {
  return request.post(`/api/admin/workflows/${workflowId}/execute`, payload)
}

export function cloneWorkflow(workflowId: number | string) {
  return request.post(`/api/admin/workflows/${workflowId}/clone`)
}

export function testNode(payload: { node_type: string, config: Record<string, unknown>, input_text: string }) {
  return request.post('/api/admin/workflows/test-node', payload)
}

export function getNodeTypes() {
  return request.get('/api/admin/workflows/node-types')
}

/* ── 执行记录 ── */

export function getWorkflowExecution(executionId: number | string) {
  return request.get(`/api/admin/workflow-executions/${executionId}`)
}
