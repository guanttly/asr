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
  filename?: string
  title?: string
  workflow_id?: number | string
  language?: string
}

export interface MeetingUploadInit {
  uploadId: string
  nextIndex: number
  maxChunkSize: number
}

export interface MeetingUploadState {
  uploadId: string
  status: string
  nextIndex: number
  duration: number
  totalBytes: number
  meetingId: number | null
}

export interface MeetingUploadAppendResult {
  received: number
  nextIndex: number
  duration: number
  status: string
  meetingId: number | null
  duplicate: boolean
}

export interface MeetingUploadCompleteResult {
  meetingId: number | null
  status: string
  duration: number
}

// MeetingUploadError carries the HTTP status so callers can distinguish
// idempotent duplicates (409) and transient failures (>=500) from fatal ones.
export class MeetingUploadError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.name = 'MeetingUploadError'
    this.status = status
  }
}

interface RawMeetingUploadState {
  upload_id?: string
  status?: string
  next_index?: number
  duration?: number
  total_bytes?: number
  meeting_id?: number | null
}

function toMeetingUploadState(data?: RawMeetingUploadState | null): MeetingUploadState | null {
  if (!data?.upload_id)
    return null
  return {
    uploadId: data.upload_id,
    status: data.status ?? '',
    nextIndex: data.next_index ?? 0,
    duration: data.duration ?? 0,
    totalBytes: data.total_bytes ?? 0,
    meetingId: data.meeting_id ?? null,
  }
}

// initMeetingLiveUpload opens a resumable, crash-safe meeting upload session.
// The server persists each chunk to disk immediately, so a client crash never
// loses more than the last unsent buffer.
export async function initMeetingLiveUpload(fields: MeetingChunkUploadFields): Promise<MeetingUploadInit> {
  const form = new FormData()
  form.append('filename', fields.filename ?? `meeting-${Date.now()}.wav`)
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
  const envelope = await readResponseEnvelope<{ upload_id?: string, next_index?: number, max_chunk_size?: number }>(response)
  if (!response.ok || !envelope.data?.upload_id)
    throw new MeetingUploadError(envelope.message || '初始化会议上传失败', response.status)
  return {
    uploadId: envelope.data.upload_id,
    nextIndex: envelope.data.next_index ?? 0,
    maxChunkSize: envelope.data.max_chunk_size ?? 0,
  }
}

// appendMeetingLiveChunk durably stores one raw little-endian PCM chunk. The
// checksum lets the server reject corrupted retransmits and de-duplicate
// idempotent retries, which is what makes resends safe.
export async function appendMeetingLiveChunk(
  uploadId: string,
  index: number,
  data: BlobPart,
  checksum: string,
): Promise<MeetingUploadAppendResult> {
  const query = `upload_id=${encodeURIComponent(uploadId)}&index=${index}${checksum ? `&checksum=${checksum}` : ''}`
  const response = await authedFetch(`/api/meetings/upload/chunk?${query}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/octet-stream',
    },
    body: data,
  })
  const envelope = await readResponseEnvelope<{
    received?: number
    next_index?: number
    duration?: number
    status?: string
    meeting_id?: number | null
    duplicate?: boolean
  }>(response)
  if (!response.ok)
    throw new MeetingUploadError(envelope.message || '会议分片上传失败', response.status)
  return {
    received: envelope.data?.received ?? 0,
    nextIndex: envelope.data?.next_index ?? index + 1,
    duration: envelope.data?.duration ?? 0,
    status: envelope.data?.status ?? 'recording',
    meetingId: envelope.data?.meeting_id ?? null,
    duplicate: Boolean(envelope.data?.duplicate),
  }
}

// heartbeatMeetingUpload refreshes the session's last-seen timestamp so the
// server-side maintenance loop does not treat an active recording as abandoned.
export async function heartbeatMeetingUpload(uploadId: string): Promise<MeetingUploadState | null> {
  const response = await authedFetch(`/api/meetings/upload/heartbeat?upload_id=${encodeURIComponent(uploadId)}`, {
    method: 'POST',
  })
  const envelope = await readResponseEnvelope<RawMeetingUploadState>(response)
  if (!response.ok)
    throw new MeetingUploadError(envelope.message || '会议上传心跳失败', response.status)
  return toMeetingUploadState(envelope.data)
}

// completeMeetingLiveUpload assembles the stored chunks into the final WAV and
// hands the meeting into the normal transcription pipeline. The server discards
// recordings shorter than the configured minimum.
export async function completeMeetingLiveUpload(uploadId: string): Promise<MeetingUploadCompleteResult> {
  const response = await authedFetch(`/api/meetings/upload/complete?upload_id=${encodeURIComponent(uploadId)}`, {
    method: 'POST',
  })
  const envelope = await readResponseEnvelope<{ meeting_id?: number | null, status?: string, duration?: number }>(response)
  if (!response.ok)
    throw new MeetingUploadError(envelope.message || '会议录音上传失败', response.status)
  return {
    meetingId: envelope.data?.meeting_id ?? null,
    status: envelope.data?.status ?? 'completed',
    duration: envelope.data?.duration ?? 0,
  }
}

// abortMeetingUpload explicitly discards a session and its chunks. Best-effort:
// failures are ignored because the server-side cleanup loop is the safety net.
export async function abortMeetingUpload(uploadId: string): Promise<void> {
  try {
    await authedFetch(`/api/meetings/upload/abort?upload_id=${encodeURIComponent(uploadId)}`, {
      method: 'POST',
    })
  }
  catch {
    // best-effort cleanup; ignore failures
  }
}

// getMeetingUploadStatus returns the server-side session state, or null when the
// session no longer exists (already finalized or cleaned up).
export async function getMeetingUploadStatus(uploadId: string): Promise<MeetingUploadState | null> {
  const response = await authedFetch(`/api/meetings/upload/${encodeURIComponent(uploadId)}`, {
    method: 'GET',
  })
  if (response.status === 404)
    return null
  const envelope = await readResponseEnvelope<RawMeetingUploadState>(response)
  if (!response.ok)
    throw new MeetingUploadError(envelope.message || '查询会议上传状态失败', response.status)
  return toMeetingUploadState(envelope.data)
}