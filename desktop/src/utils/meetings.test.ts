import { beforeEach, describe, expect, it, vi } from 'vitest'

const authMocks = vi.hoisted(() => ({
  authedFetch: vi.fn(),
}))

vi.mock('@/utils/auth', () => ({
  authedFetch: authMocks.authedFetch,
  readResponseEnvelope: async (response: Response) => response.json(),
}))

import { deleteMeeting, listMeetings, regenerateMeetingSummary, updateMeeting } from './meetings'

function envelope(data: unknown, status = 200, message = 'ok') {
  return new Response(JSON.stringify({ code: status < 400 ? 0 : -1, message, data }), { status })
}

describe('meeting API helpers', () => {
  beforeEach(() => {
    authMocks.authedFetch.mockReset()
  })

  it('lists meetings with backend pagination parameters', async () => {
    authMocks.authedFetch.mockResolvedValue(envelope({ items: [], total: 0 }))

    await listMeetings({ offset: 20, limit: 10 })

    expect(authMocks.authedFetch).toHaveBeenCalledWith('/api/meetings?offset=20&limit=10')
  })

  it('updates title and summary content through the detail endpoint', async () => {
    authMocks.authedFetch.mockResolvedValue(envelope({ id: 3, title: '新标题', transcripts: [] }))

    await updateMeeting(3, { title: '新标题', summary_content: '正文' })

    expect(authMocks.authedFetch).toHaveBeenCalledWith('/api/meetings/3', expect.objectContaining({
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title: '新标题', summary_content: '正文' }),
    }))
  })

  it('regenerates summaries with the selected meeting workflow id', async () => {
    authMocks.authedFetch.mockResolvedValue(envelope({ ok: true }))

    await regenerateMeetingSummary(8, 99)
    expect(authMocks.authedFetch).toHaveBeenCalledWith('/api/meetings/8/summary', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ workflow_id: 99 }),
    }))
  })

  it('surfaces backend delete failures for manual retry', async () => {
    authMocks.authedFetch.mockResolvedValue(envelope(null, 500, '删除失败原因'))

    await expect(deleteMeeting(6)).rejects.toThrow('删除失败原因')
  })
})