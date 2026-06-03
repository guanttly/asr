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
  final_text?: string
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

export interface MeetingChunkUploadFields {
  filename: string
  title?: string
  workflow_id?: number | string
  language?: string
}

// MEETING_CHUNK_SIZE keeps every chunk request comfortably below the backend
// and nginx single-request limits so arbitrarily long meetings can be uploaded.
const MEETING_CHUNK_SIZE = 8 * 1024 * 1024

// MEETING_DIRECT_UPLOAD_LIMIT is the largest file we still upload in a single
// multipart request; larger files switch to the chunked protocol.
export const MEETING_DIRECT_UPLOAD_LIMIT = 150 * 1024 * 1024

async function initMeetingChunkUpload(fields: MeetingChunkUploadFields): Promise<string> {
  const form = new FormData()
  form.append('filename', fields.filename)
  if (fields.title != null)
    form.append('title', fields.title)
  if (fields.workflow_id != null)
    form.append('workflow_id', String(fields.workflow_id))
  if (fields.language != null)
    form.append('language', fields.language)
  const response = await authedFetch('/api/meetings/upload/init', {
    method: 'POST',
    body: form,
  })
  const envelope = await readResponseEnvelope<{ upload_id?: string }>(response)
  if (!response.ok || !envelope.data?.upload_id)
    throw new Error(envelope.message || '初始化会议上传失败')
  return envelope.data.upload_id
}

async function appendMeetingChunk(uploadId: string, index: number, chunk: Blob): Promise<void> {
  const response = await authedFetch(`/api/meetings/upload/chunk?upload_id=${encodeURIComponent(uploadId)}&index=${index}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/octet-stream',
    },
    body: chunk,
  })
  if (!response.ok) {
    const envelope = await readResponseEnvelope(response)
    throw new Error(envelope.message || '会议分片上传失败')
  }
}

async function completeMeetingChunkUpload(uploadId: string) {
  const response = await authedFetch(`/api/meetings/upload/complete?upload_id=${encodeURIComponent(uploadId)}`, {
    method: 'POST',
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

async function abortMeetingChunkUpload(uploadId: string): Promise<void> {
  try {
    await authedFetch(`/api/meetings/upload/abort?upload_id=${encodeURIComponent(uploadId)}`, {
      method: 'POST',
    })
  }
  catch {
    // best-effort cleanup; ignore failures
  }
}

// uploadMeetingFromAudioChunked uploads a (potentially very large) meeting
// recording by splitting it into sequential chunks. The backend reassembles the
// chunks on disk and then creates the meeting.
export async function uploadMeetingFromAudioChunked(
  file: File,
  fields: Omit<MeetingChunkUploadFields, 'filename'>,
  onProgress?: (uploaded: number, total: number) => void,
) {
  const uploadId = await initMeetingChunkUpload({ ...fields, filename: file.name })
  try {
    const total = file.size
    let offset = 0
    let index = 0
    while (offset < total) {
      const end = Math.min(offset + MEETING_CHUNK_SIZE, total)
      await appendMeetingChunk(uploadId, index, file.slice(offset, end))
      offset = end
      index += 1
      onProgress?.(offset, total)
    }
    return await completeMeetingChunkUpload(uploadId)
  }
  catch (error) {
    await abortMeetingChunkUpload(uploadId)
    throw error
  }
}