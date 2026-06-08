import { beforeEach, describe, expect, it, vi } from 'vitest'

const authMocks = vi.hoisted(() => ({
  authedFetch: vi.fn(),
  createTimeoutSignal: vi.fn(() => {
    const controller = new AbortController()
    return {
      signal: controller.signal,
      cleanup: vi.fn(),
    }
  }),
  ensureMeetingWorkflowBinding: vi.fn(),
  ensureProductFeatures: vi.fn(),
  ensureRealtimeWorkflowBinding: vi.fn(),
}))

const debugMocks = vi.hoisted(() => ({
  debugLog: vi.fn(),
}))

const transcriptionMocks = vi.hoisted(() => ({
  createRealtimeTranscriptionTask: vi.fn(),
  uploadMeetingFromAudio: vi.fn(),
  uploadMeetingFromAudioChunked: vi.fn(),
  uploadRealtimeSessionTask: vi.fn(),
}))

const injectorMocks = vi.hoisted(() => ({
  injectText: vi.fn(),
}))

const voiceControlMocks = vi.hoisted(() => ({
  ensureLoaded: vi.fn(),
  handleSegmentText: vi.fn(),
  reset: vi.fn(),
}))

const appStoreMocks = vi.hoisted(() => {
  const state = {
    autoInject: false,
    appendHistory: vi.fn(),
    hasCapability: vi.fn((key: string) => key === 'meeting' && state.meetingCapability),
    invalidateWorkflowBindings: vi.fn(),
    meetingCapability: false,
    meetingWorkflowId: null as number | null,
    realtimeWorkflowId: null as number | null,
    recognitionSettings: {
      keepPunctuation: false,
      minSpeechThreshold: 0.018,
      noiseGateMultiplier: 2.8,
      endSilenceChunks: 4,
      minEffectiveSpeechChunks: 2,
      singleChunkPeakMultiplier: 1.45,
    },
    sceneMode: 'report',
  }
  return { state }
})

vi.mock('@/utils/auth', () => ({
  authedFetch: authMocks.authedFetch,
  createTimeoutSignal: authMocks.createTimeoutSignal,
  ensureMeetingWorkflowBinding: authMocks.ensureMeetingWorkflowBinding,
  ensureProductFeatures: authMocks.ensureProductFeatures,
  ensureRealtimeWorkflowBinding: authMocks.ensureRealtimeWorkflowBinding,
  readResponseEnvelope: async (response: Response) => response.json(),
}))

vi.mock('@/utils/debug', () => ({
  debugLog: debugMocks.debugLog,
}))

vi.mock('@/utils/transcription', () => ({
  createRealtimeTranscriptionTask: transcriptionMocks.createRealtimeTranscriptionTask,
  uploadMeetingFromAudio: transcriptionMocks.uploadMeetingFromAudio,
  uploadMeetingFromAudioChunked: transcriptionMocks.uploadMeetingFromAudioChunked,
  uploadRealtimeSessionTask: transcriptionMocks.uploadRealtimeSessionTask,
  MEETING_DIRECT_UPLOAD_LIMIT: 150 * 1024 * 1024,
}))

vi.mock('./useInjector', () => ({
  useInjector: () => ({
    injectText: injectorMocks.injectText,
  }),
}))

vi.mock('./useVoiceControl', () => ({
  useVoiceControl: () => ({
    ensureLoaded: voiceControlMocks.ensureLoaded,
    handleSegmentText: voiceControlMocks.handleSegmentText,
    reset: voiceControlMocks.reset,
  }),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStoreMocks.state,
}))

import { useTranscribe } from './useTranscribe'

function envelope(data: unknown, status = 200, message = 'ok') {
  return new Response(JSON.stringify({ code: status < 400 ? 0 : -1, message, data }), { status })
}

const CHUNK_SAMPLE_COUNT = 3200

function createChunk(amplitude = 0) {
  const samples = new Int16Array(CHUNK_SAMPLE_COUNT)
  samples.fill(amplitude)
  return samples.buffer
}

function emitSegment(transcribe: ReturnType<typeof useTranscribe>, amplitude = 1400) {
  transcribe.handleChunk(createChunk(amplitude))
  transcribe.handleChunk(createChunk(amplitude))
  for (let i = 0; i < 4; i++)
    transcribe.handleChunk(createChunk(0))
}

// Repeatedly drain the microtask queue until `predicate` holds. Used to await
// the parallel transcription pipeline reaching a known point without relying on
// real timers.
async function waitFor(predicate: () => boolean, max = 100) {
  for (let i = 0; i < max; i++) {
    if (predicate())
      return
    await Promise.resolve()
  }
  throw new Error('waitFor timed out')
}

// Drain a fixed number of microtask turns so any (incorrect) out-of-order commit
// would have a chance to happen before we assert it did not.
async function flushMicrotasks(times = 12) {
  for (let i = 0; i < times; i++)
    await Promise.resolve()
}

describe('useTranscribe realtime persistence', () => {
  beforeEach(() => {
    authMocks.authedFetch.mockReset()
    authMocks.createTimeoutSignal.mockClear()
    authMocks.ensureMeetingWorkflowBinding.mockReset().mockResolvedValue(null)
    authMocks.ensureProductFeatures.mockReset().mockResolvedValue(undefined)
    authMocks.ensureRealtimeWorkflowBinding.mockReset().mockResolvedValue(null)

    debugMocks.debugLog.mockReset().mockResolvedValue(undefined)

    transcriptionMocks.createRealtimeTranscriptionTask.mockReset().mockResolvedValue({ id: 1 })
    transcriptionMocks.uploadMeetingFromAudio.mockReset().mockResolvedValue({ meeting: { id: 1 } })
    transcriptionMocks.uploadRealtimeSessionTask.mockReset().mockResolvedValue({ task: { id: 1 } })

    injectorMocks.injectText.mockReset().mockResolvedValue({ success: true })

    voiceControlMocks.ensureLoaded.mockReset().mockResolvedValue(undefined)
    voiceControlMocks.handleSegmentText.mockReset().mockResolvedValue({ swallow: false })
    voiceControlMocks.reset.mockReset()

    appStoreMocks.state.autoInject = false
    appStoreMocks.state.meetingCapability = false
    appStoreMocks.state.meetingWorkflowId = null
    appStoreMocks.state.realtimeWorkflowId = null
    appStoreMocks.state.sceneMode = 'report'
    appStoreMocks.state.appendHistory.mockReset()
    appStoreMocks.state.hasCapability.mockClear()
    appStoreMocks.state.invalidateWorkflowBindings.mockReset()
  })

  it('creates one realtime task per finalized segment instead of merging a session transcript', async () => {
    const recognizedTexts = ['可以', '失败']
    authMocks.authedFetch.mockImplementation(async (url: string) => {
      if (url === '/api/asr/realtime-segments')
        return envelope({ text: recognizedTexts.shift() || '' })
      throw new Error(`unexpected request: ${url}`)
    })

    const transcribe = useTranscribe()
    emitSegment(transcribe)
    emitSegment(transcribe)
    await transcribe.stopAndFlush()

    expect(transcriptionMocks.uploadRealtimeSessionTask).toHaveBeenCalledTimes(2)
    const savedTexts = transcriptionMocks.uploadRealtimeSessionTask.mock.calls.map(([payload]) => payload.get('result_text'))
    expect(savedTexts).toEqual(['可以', '失败'])
    expect(transcriptionMocks.createRealtimeTranscriptionTask).not.toHaveBeenCalled()
    expect(appStoreMocks.state.appendHistory.mock.calls.map(([text]) => text)).toEqual(['可以', '失败'])
  })

  it('keeps the short-recording guard for accidental realtime taps', async () => {
    authMocks.authedFetch.mockResolvedValue(envelope({ text: '短句' }))

    const transcribe = useTranscribe()
    transcribe.handleChunk(createChunk(1400))
    transcribe.handleChunk(createChunk(1400))
    await transcribe.stopAndFlush()

    expect(transcriptionMocks.uploadRealtimeSessionTask).not.toHaveBeenCalled()
    expect(transcriptionMocks.createRealtimeTranscriptionTask).not.toHaveBeenCalled()
  })

  it('keeps meeting mode on whole-session upload', async () => {
    authMocks.authedFetch.mockResolvedValue(envelope({ text: '会议内容' }))
    appStoreMocks.state.sceneMode = 'meeting'
    appStoreMocks.state.meetingCapability = true

    const transcribe = useTranscribe()
    emitSegment(transcribe)
    for (let i = 0; i < 20; i++)
      transcribe.handleChunk(createChunk(0))
    await transcribe.stopAndFlush()

    expect(transcriptionMocks.uploadMeetingFromAudio).toHaveBeenCalledTimes(1)
    expect(transcriptionMocks.uploadRealtimeSessionTask).not.toHaveBeenCalled()
    expect(transcriptionMocks.createRealtimeTranscriptionTask).not.toHaveBeenCalled()
  })

  it('does not inject or append realtime history while recording in meeting mode', async () => {
    authMocks.authedFetch.mockResolvedValue(envelope({ text: '会议内容' }))
    appStoreMocks.state.sceneMode = 'meeting'
    appStoreMocks.state.meetingCapability = true
    appStoreMocks.state.autoInject = true

    const transcribe = useTranscribe()
    emitSegment(transcribe)
    for (let i = 0; i < 20; i++)
      transcribe.handleChunk(createChunk(0))
    await transcribe.stopAndFlush()

    expect(transcriptionMocks.uploadMeetingFromAudio).toHaveBeenCalledTimes(1)
    expect(injectorMocks.injectText).not.toHaveBeenCalled()
    expect(appStoreMocks.state.appendHistory).not.toHaveBeenCalled()
    expect(transcriptionMocks.uploadRealtimeSessionTask).not.toHaveBeenCalled()
    expect(transcriptionMocks.createRealtimeTranscriptionTask).not.toHaveBeenCalled()
  })

  it('sends the configured meeting workflow id when uploading desktop meeting recordings', async () => {
    authMocks.authedFetch.mockResolvedValue(envelope({ text: '会议内容' }))
    authMocks.ensureMeetingWorkflowBinding.mockResolvedValue(33)
    appStoreMocks.state.sceneMode = 'meeting'
    appStoreMocks.state.meetingCapability = true

    const transcribe = useTranscribe()
    emitSegment(transcribe)
    for (let i = 0; i < 20; i++)
      transcribe.handleChunk(createChunk(0))
    await transcribe.stopAndFlush()

    expect(authMocks.ensureMeetingWorkflowBinding).toHaveBeenCalled()
    const [formData] = transcriptionMocks.uploadMeetingFromAudio.mock.calls[0]
    expect(formData.get('workflow_id')).toBe('33')
  })

  it('preserves segment order even when a later segment is recognized first', async () => {
    // Each ASR call parks on a gate so the test controls completion order.
    const gates: Array<(text: string) => void> = []
    authMocks.authedFetch.mockImplementation((url: string) => {
      if (url !== '/api/asr/realtime-segments')
        throw new Error(`unexpected request: ${url}`)
      return new Promise<Response>((resolve) => {
        gates.push((text: string) => resolve(envelope({ text })))
      })
    })

    const transcribe = useTranscribe()
    emitSegment(transcribe)
    emitSegment(transcribe)
    await waitFor(() => gates.length === 2)

    // Finish the SECOND segment first; the ordered consumer must still hold its
    // output back until the first segment is committed.
    gates[1]('第二段')
    await flushMicrotasks()
    expect(appStoreMocks.state.appendHistory).not.toHaveBeenCalled()

    gates[0]('第一段')
    await transcribe.stopAndFlush()

    expect(appStoreMocks.state.appendHistory.mock.calls.map(([t]) => t)).toEqual(['第一段', '第二段'])
    const savedTexts = transcriptionMocks.uploadRealtimeSessionTask.mock.calls.map(([payload]) => payload.get('result_text'))
    expect(savedTexts).toEqual(['第一段', '第二段'])
  })

  it('does not paste to the cursor after the user stops', async () => {
    appStoreMocks.state.autoInject = true
    let releaseAsr: (() => void) | null = null
    authMocks.authedFetch.mockImplementation((url: string) => {
      if (url !== '/api/asr/realtime-segments')
        throw new Error(`unexpected request: ${url}`)
      return new Promise<Response>((resolve) => {
        releaseAsr = () => resolve(envelope({ text: '停止后内容' }))
      })
    })

    const transcribe = useTranscribe()
    emitSegment(transcribe)
    await waitFor(() => releaseAsr !== null)

    // Stop first (severs the inject link), then let the in-flight ASR finish.
    const stopPromise = transcribe.stopAndFlush()
    releaseAsr!()
    await stopPromise

    expect(injectorMocks.injectText).not.toHaveBeenCalled()
    // The mid-transcription segment is still recorded to history, just not pasted.
    expect(appStoreMocks.state.appendHistory).toHaveBeenCalledWith('停止后内容')
  })

  it('drops not-yet-started segments from the queue when the user stops', async () => {
    let asrCallCount = 0
    const releasers: Array<() => void> = []
    authMocks.authedFetch.mockImplementation((url: string) => {
      if (url !== '/api/asr/realtime-segments')
        throw new Error(`unexpected request: ${url}`)
      asrCallCount += 1
      return new Promise<Response>((resolve) => {
        releasers.push(() => resolve(envelope({ text: '内容' })))
      })
    })

    const transcribe = useTranscribe()
    // Emit more segments than the parallel limit so the tail stays not-started.
    for (let i = 0; i < 5; i++)
      emitSegment(transcribe)

    // Only the concurrency-limited segments begin ASR; the rest wait in queue.
    await waitFor(() => releasers.length === 3)
    expect(asrCallCount).toBe(3)

    const stopPromise = transcribe.stopAndFlush()
    releasers.forEach(release => release())
    await stopPromise

    // The queued-but-not-started segments never reach ASR and are not recorded.
    expect(asrCallCount).toBe(3)
    expect(appStoreMocks.state.appendHistory).toHaveBeenCalledTimes(3)
  })
})
