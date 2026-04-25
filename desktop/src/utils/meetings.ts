import { authedFetch, readResponseEnvelope } from './auth'

export interface MeetingItem {
  id: number
  title: string
  duration: number
  status: 'uploaded' | 'processing' | 'completed' | 'failed' | string
  workflow_id?: number | null
  sync_fail_count: number
  last_sync_error?: string
  last_sync_at?: string | null
  next_sync_at?: string | null
  created_at: string
  updated_at: string
}

export interface MeetingTranscript {
  speaker_label: string
  text: string
  start_time: number
  end_time: number
}

export interface MeetingSummaryItem {
  content: string
  model_version: string
  created_at: string
}

export interface MeetingDetailResponse extends MeetingItem {
  transcripts: MeetingTranscript[]
  summary?: MeetingSummaryItem
}

export interface MeetingListResult {
  items: MeetingItem[]
  total: number
}

export interface UpdateMeetingPayload {
  title?: string
  summary_content?: string
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

export async function listMeetings(params?: { offset?: number, limit?: number }): Promise<MeetingListResult> {
  const response = await authedFetch(`/api/meetings${buildQuery({
    offset: params?.offset,
    limit: params?.limit,
  })}`)
  const payload = await readResponseEnvelope<MeetingListResult>(response)
  if (!response.ok)
    throw new Error(payload.message || '加载会议列表失败')
  return payload.data || { items: [], total: 0 }
}

export async function getMeetingDetail(id: number): Promise<MeetingDetailResponse> {
  const response = await authedFetch(`/api/meetings/${id}`)
  const payload = await readResponseEnvelope<MeetingDetailResponse>(response)
  if (!response.ok || !payload.data)
    throw new Error(payload.message || '加载会议详情失败')
  return payload.data
}

export async function deleteMeeting(id: number): Promise<void> {
  const response = await authedFetch(`/api/meetings/${id}`, { method: 'DELETE' })
  const payload = await readResponseEnvelope<{ deleted?: boolean }>(response)
  if (!response.ok)
    throw new Error(payload.message || '删除会议失败')
}

export async function updateMeeting(id: number, payload: UpdateMeetingPayload): Promise<MeetingDetailResponse> {
  const response = await authedFetch(`/api/meetings/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const result = await readResponseEnvelope<MeetingDetailResponse>(response)
  if (!response.ok || !result.data)
    throw new Error(result.message || '保存会议纪要失败')
  return result.data
}

export async function regenerateMeetingSummary(id: number, workflowId?: number | null): Promise<void> {
  const response = await authedFetch(`/api/meetings/${id}/summary`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(workflowId != null ? { workflow_id: workflowId } : {}),
  })
  const payload = await readResponseEnvelope<unknown>(response)
  if (!response.ok)
    throw new Error(payload.message || '重新生成纪要失败')
}
