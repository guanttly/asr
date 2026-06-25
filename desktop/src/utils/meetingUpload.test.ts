import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  initMeetingLiveUpload: vi.fn(),
  appendMeetingLiveChunk: vi.fn(),
  heartbeatMeetingUpload: vi.fn(),
  completeMeetingLiveUpload: vi.fn(),
  abortMeetingUpload: vi.fn(),
  getMeetingUploadStatus: vi.fn(),
}))

class FakeMeetingUploadError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.name = 'MeetingUploadError'
    this.status = status
  }
}

vi.mock('./transcription', () => ({
  initMeetingLiveUpload: apiMocks.initMeetingLiveUpload,
  appendMeetingLiveChunk: apiMocks.appendMeetingLiveChunk,
  heartbeatMeetingUpload: apiMocks.heartbeatMeetingUpload,
  completeMeetingLiveUpload: apiMocks.completeMeetingLiveUpload,
  abortMeetingUpload: apiMocks.abortMeetingUpload,
  getMeetingUploadStatus: apiMocks.getMeetingUploadStatus,
  MeetingUploadError: FakeMeetingUploadError,
}))

const { MeetingLiveUpload, recoverPendingMeetingUploads } = await import('./meetingUpload')

const BYTES_PER_SECOND = 16000 * 2

function pcm(seconds: number): ArrayBuffer {
  return new ArrayBuffer(Math.round(seconds * BYTES_PER_SECOND))
}

beforeEach(() => {
  apiMocks.initMeetingLiveUpload.mockReset().mockResolvedValue({ uploadId: 'up-1', nextIndex: 0, maxChunkSize: 0 })
  apiMocks.appendMeetingLiveChunk.mockReset().mockResolvedValue({ received: 0, nextIndex: 1, duration: 0, status: 'recording', meetingId: null, duplicate: false })
  apiMocks.heartbeatMeetingUpload.mockReset().mockResolvedValue(null)
  apiMocks.completeMeetingLiveUpload.mockReset().mockResolvedValue({ meetingId: 7, status: 'uploaded', duration: 0 })
  apiMocks.abortMeetingUpload.mockReset().mockResolvedValue(undefined)
  apiMocks.getMeetingUploadStatus.mockReset().mockResolvedValue(null)
})

describe('meetingLiveUpload', () => {
  it('flushes ~5s chunks in order and completes a long recording', async () => {
    const uploader = new MeetingLiveUpload({ resolveInitFields: () => ({ title: 't' }) })
    uploader.start()
    // 12s of audio in 1s chunks → flush at the 5s and 10s boundaries, plus a 2s
    // tail on finish.
    for (let i = 0; i < 12; i++)
      uploader.pushPcm(pcm(1))
    const result = await uploader.finish()

    expect(apiMocks.initMeetingLiveUpload).toHaveBeenCalledTimes(1)
    expect(apiMocks.appendMeetingLiveChunk).toHaveBeenCalledTimes(3)
    const indices = apiMocks.appendMeetingLiveChunk.mock.calls.map(call => call[1])
    expect(indices).toEqual([0, 1, 2])
    expect(apiMocks.completeMeetingLiveUpload).toHaveBeenCalledWith('up-1')
    expect(result.meetingId).toBe(7)
    expect(result.discarded).toBe(false)
  })

  it('discards a recording shorter than the minimum without completing', async () => {
    const uploader = new MeetingLiveUpload()
    uploader.start()
    uploader.pushPcm(pcm(3))
    const result = await uploader.finish()

    expect(apiMocks.completeMeetingLiveUpload).not.toHaveBeenCalled()
    expect(apiMocks.abortMeetingUpload).toHaveBeenCalledWith('up-1')
    expect(result.discarded).toBe(true)
    expect(result.status).toBe('aborted')
  })

  it('treats a 409 duplicate chunk as success so resends never break the stream', async () => {
    apiMocks.appendMeetingLiveChunk.mockRejectedValueOnce(new FakeMeetingUploadError('duplicate', 409))
    const uploader = new MeetingLiveUpload()
    uploader.start()
    for (let i = 0; i < 6; i++)
      uploader.pushPcm(pcm(1))
    const result = await uploader.finish()

    // The 409 on the first chunk is swallowed; the recording still completes.
    expect(apiMocks.completeMeetingLiveUpload).toHaveBeenCalledTimes(1)
    expect(result.discarded).toBe(false)
  })

  it('keeps uploaded chunks on detach without aborting the server session', async () => {
    const uploader = new MeetingLiveUpload()
    uploader.start()
    for (let i = 0; i < 6; i++)
      uploader.pushPcm(pcm(1))
    uploader.detach()
    // let the background queue settle
    await Promise.resolve()
    await Promise.resolve()

    expect(apiMocks.abortMeetingUpload).not.toHaveBeenCalled()
    expect(apiMocks.completeMeetingLiveUpload).not.toHaveBeenCalled()
  })

  it('aborts and clears the session on explicit cancel', async () => {
    const uploader = new MeetingLiveUpload()
    uploader.start()
    for (let i = 0; i < 6; i++)
      uploader.pushPcm(pcm(1))
    await uploader.cancel()

    expect(apiMocks.abortMeetingUpload).toHaveBeenCalledWith('up-1')
    expect(apiMocks.completeMeetingLiveUpload).not.toHaveBeenCalled()
  })
})

describe('recoverPendingMeetingUploads', () => {
  it('does nothing when there are no pending markers', async () => {
    await recoverPendingMeetingUploads()
    expect(apiMocks.getMeetingUploadStatus).not.toHaveBeenCalled()
    expect(apiMocks.completeMeetingLiveUpload).not.toHaveBeenCalled()
  })
})
