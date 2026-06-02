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
  uploadRealtimeSessionTask: transcriptionMocks.uploadRealtimeSessionTask,
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
})
