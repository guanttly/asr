import { beforeEach, describe, expect, it, vi } from 'vitest'

import { TRANSCRIPTION_TASK_TYPES } from '@/constants/transcription'

const authMocks = vi.hoisted(() => ({
  authedFetch: vi.fn(),
}))

vi.mock('@/utils/auth', () => ({
  authedFetch: authMocks.authedFetch,
  readResponseEnvelope: async (response: Response) => response.json(),
}))

import {
  clearTranscriptionTasks,
  createRealtimeTranscriptionTask,
  deleteTranscriptionTask,
  getTranscriptionTasks,
} from './transcription'

function envelope(data: unknown, status = 200, message = 'ok') {
  return new Response(JSON.stringify({ code: status < 400 ? 0 : -1, message, data }), { status })
}

describe('transcription history API helpers', () => {
  beforeEach(() => {
    authMocks.authedFetch.mockReset()
  })

  it('loads realtime history with pagination and task type filtering', async () => {
    authMocks.authedFetch.mockResolvedValue(envelope({ items: [], total: 0 }))

    await getTranscriptionTasks({ offset: 10, limit: 10, type: TRANSCRIPTION_TASK_TYPES.REALTIME })

    expect(authMocks.authedFetch).toHaveBeenCalledWith('/api/asr/tasks?offset=10&limit=10&type=realtime')
  })

  it('deletes one realtime history item without mutating other list state', async () => {
    authMocks.authedFetch.mockResolvedValue(envelope({ deleted: true }))

    await expect(deleteTranscriptionTask(42)).resolves.toEqual({ deleted: true })
    expect(authMocks.authedFetch).toHaveBeenCalledWith('/api/asr/tasks/42', { method: 'DELETE' })
  })

  it('clears only the realtime task type when requested', async () => {
    authMocks.authedFetch.mockResolvedValue(envelope({ deleted_count: 3, skipped_count: 1 }))

    await expect(clearTranscriptionTasks(TRANSCRIPTION_TASK_TYPES.REALTIME)).resolves.toEqual({
      deleted_count: 3,
      skipped_count: 1,
    })
    expect(authMocks.authedFetch).toHaveBeenCalledWith('/api/asr/tasks?type=realtime', { method: 'DELETE' })
  })

  it('creates realtime history tasks with workflow and duration metadata', async () => {
    authMocks.authedFetch.mockResolvedValue(envelope({ id: 7 }))

    await createRealtimeTranscriptionTask({ result_text: '最终文本', duration: 1.2, workflow_id: 9 })

    expect(authMocks.authedFetch).toHaveBeenCalledWith('/api/asr/tasks', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({
        type: TRANSCRIPTION_TASK_TYPES.REALTIME,
        result_text: '最终文本',
        duration: 1.2,
        workflow_id: 9,
      }),
    }))
  })
})