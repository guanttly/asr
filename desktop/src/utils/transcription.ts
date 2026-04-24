import type { TranscriptionTaskType } from '@/constants/transcription'

import { TRANSCRIPTION_TASK_TYPES } from '@/constants/transcription'

import { authedFetch, readResponseEnvelope } from './auth'

export interface TranscriptionTaskItem {
  id: number
  type: TranscriptionTaskType
  status: string
  post_process_status: string
  post_process_error?: string
  result_text?: string
  duration?: number
  workflow_id?: number | null
  created_at: string
  updated_at: string
}

export interface WorkflowNodeResultItem {
  id: number
  node_id: number
  node_type: string
  label: string
  position: number
  input_text: string
  output_text: string
  status: string
  duration_ms: number
  executed_at?: string | null
}

export interface WorkflowExecutionItem {
  id: number
  workflow_id: number
  trigger_type: string
  trigger_id: string
  input_text: string
  final_text: string
  status: string
  error_message?: string
  node_results?: WorkflowNodeResultItem[]
  started_at?: string | null
  completed_at?: string | null
  created_at: string
}

export interface TaskListResult {
  items: TranscriptionTaskItem[]
  total: number
}

export interface ClearTasksResult {
  deleted_count: number
  skipped_count: number
}

function buildQuery(params: Record<string, string | number | undefined>) {
  const search = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value == null || value === '')
      return
    search.set(key, String(value))
  })
  const query = search.toString()
  return query ? `?${query}` : ''
}

export async function getTranscriptionTasks(params?: { offset?: number, limit?: number, type?: TranscriptionTaskType }) {
  const response = await authedFetch(`/api/asr/tasks${buildQuery({
    offset: params?.offset,
    limit: params?.limit,
    type: params?.type,
  })}`)
  const payload = await readResponseEnvelope<TaskListResult>(response)
  if (!response.ok)
    throw new Error(payload.message || '加载转写记录失败')
  return payload.data || { items: [], total: 0 }
}

export async function getTranscriptionTaskExecutions(taskId: number | string) {
  const response = await authedFetch(`/api/asr/tasks/${taskId}/executions`)
  const payload = await readResponseEnvelope<WorkflowExecutionItem[]>(response)
  if (!response.ok)
    throw new Error(payload.message || '加载处理链路失败')
  return payload.data || []
}

export async function deleteTranscriptionTask(taskId: number | string) {
  const response = await authedFetch(`/api/asr/tasks/${taskId}`, {
    method: 'DELETE',
  })
  const payload = await readResponseEnvelope<{ deleted?: boolean }>(response)
  if (!response.ok)
    throw new Error(payload.message || '删除转写记录失败')
  return payload.data || { deleted: true }
}

export async function clearTranscriptionTasks(type?: TranscriptionTaskType) {
  const response = await authedFetch(`/api/asr/tasks${buildQuery({ type })}`, {
    method: 'DELETE',
  })
  const payload = await readResponseEnvelope<ClearTasksResult>(response)
  if (!response.ok)
    throw new Error(payload.message || '清空转写记录失败')
  return payload.data || { deleted_count: 0, skipped_count: 0 }
}

export async function createRealtimeTranscriptionTask(payload: { result_text: string, duration?: number, workflow_id?: number }) {
  const response = await authedFetch('/api/asr/tasks', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      type: TRANSCRIPTION_TASK_TYPES.REALTIME,
      result_text: payload.result_text,
      duration: payload.duration,
      workflow_id: payload.workflow_id,
    }),
  })
  const envelope = await readResponseEnvelope<TranscriptionTaskItem>(response)
  if (!response.ok)
    throw new Error(envelope.message || '保存实时转写失败')
  return envelope.data || null
}

export async function uploadRealtimeSessionTask(payload: FormData) {
  const response = await authedFetch('/api/asr/realtime-tasks/upload', {
    method: 'POST',
    body: payload,
  })
  const envelope = await readResponseEnvelope<{
    task?: TranscriptionTaskItem
    audio_url?: string
    filename?: string
  }>(response)
  if (!response.ok)
    throw new Error(envelope.message || '上传实时录音失败')
  return envelope.data || null
}

export async function uploadMeetingFromAudio(payload: FormData) {
  const response = await authedFetch('/api/meetings/upload', {
    method: 'POST',
    body: payload,
  })
  const envelope = await readResponseEnvelope<{
    meeting?: { id: number, title?: string, status?: string }
    audio_url?: string
    filename?: string
  }>(response)
  if (!response.ok)
    throw new Error(envelope.message || '会议录音上传失败')
  return envelope.data || null
}