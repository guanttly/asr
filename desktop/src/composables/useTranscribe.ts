import { ref } from 'vue'
import { useAppStore } from '@/stores/app'
import { useInjector } from './useInjector'
import { authedFetch, readResponseEnvelope } from '@/utils/auth'
import { debugLog } from '@/utils/debug'

const TARGET_SAMPLE_RATE = 16000
const CHUNK_MS = 300
const DEFAULT_NOISE_FLOOR = 0.004
const MAX_SPEECH_RMS = 0.08
const NOISE_FLOOR_SMOOTHING = 0.08
const PRE_ROLL_CHUNKS = 1
const MAX_SEGMENT_CHUNKS = 40

export function useTranscribe() {
  const appStore = useAppStore()
  const { injectText } = useInjector()

  const listeningLevel = ref(0)
  const noiseFloorLevel = ref(DEFAULT_NOISE_FLOOR)
  const status = ref<'idle' | 'listening' | 'collecting' | 'uploading' | 'processing'>('idle')
  const lastError = ref('')
  const totalSegments = ref(0)
  const pendingCount = ref(0)

  const leadInChunks: ArrayBuffer[] = []
  let activeSegmentChunks: ArrayBuffer[] = []
  let activeSpeechCount = 0
  let peakLevel = 0
  let trailingSilence = 0
  let uploadQueue: { file: File, duration: number }[] = []
  let uploadPromise: Promise<void> | null = null

  function computeRms(chunk: ArrayBuffer): number {
    const samples = new Int16Array(chunk)
    if (samples.length === 0) return 0
    let sum = 0
    for (let i = 0; i < samples.length; i++) {
      const n = samples[i] / 0x8000
      sum += n * n
    }
    return Math.sqrt(sum / samples.length)
  }

  function clamp(v: number, min: number, max: number) {
    return Math.min(max, Math.max(min, v))
  }

  function getThreshold(): number {
    const s = appStore.recognitionSettings
    return clamp(
      Math.max(s.minSpeechThreshold, noiseFloorLevel.value * s.noiseGateMultiplier),
      s.minSpeechThreshold,
      MAX_SPEECH_RMS,
    )
  }

  function updateNoiseFloor(rms: number) {
    const bounded = clamp(rms, 0, MAX_SPEECH_RMS)
    if (noiseFloorLevel.value <= 0)
      noiseFloorLevel.value = bounded
    else
      noiseFloorLevel.value = noiseFloorLevel.value * (1 - NOISE_FLOOR_SMOOTHING) + bounded * NOISE_FLOOR_SMOOTHING
  }

  function isNoiseSegment(): boolean {
    const s = appStore.recognitionSettings
    if (activeSpeechCount === 0) return true
    if (activeSpeechCount >= s.minEffectiveSpeechChunks) return false
    return peakLevel < getThreshold() * s.singleChunkPeakMultiplier
  }

  function sanitizeText(value?: string): string {
    if (!value) return ''
    return value
      .replace(/language\s+[a-z_-]+<asr_text>/gi, '')
      .replace(/<\/?asr_text>/gi, '')
      .replace(/<\|[^>]+\|>/g, '')
      .replace(/\u00A0/g, ' ')
      .trim()
  }

  function normalizeText(value?: string): string {
    const clean = sanitizeText(value)
    if (!clean) return ''
    if (appStore.recognitionSettings.keepPunctuation) return clean
    return clean.replace(/[\p{P}\p{S}]/gu, '').replace(/\s{2,}/g, ' ').trim()
  }

  function hasContent(value?: string): boolean {
    return /[A-Z0-9\u3400-\u9FFF]/i.test(normalizeText(value))
  }

  function writeASCII(view: DataView, offset: number, value: string) {
    for (let i = 0; i < value.length; i++)
      view.setUint8(offset + i, value.charCodeAt(i))
  }

  function createWav(chunks: ArrayBuffer[], fileName: string): File {
    const totalBytes = chunks.reduce((s, c) => s + c.byteLength, 0)
    const buf = new ArrayBuffer(44 + totalBytes)
    const view = new DataView(buf)
    const pcm = new Uint8Array(buf, 44)

    writeASCII(view, 0, 'RIFF')
    view.setUint32(4, 36 + totalBytes, true)
    writeASCII(view, 8, 'WAVE')
    writeASCII(view, 12, 'fmt ')
    view.setUint32(16, 16, true)
    view.setUint16(20, 1, true)
    view.setUint16(22, 1, true)
    view.setUint32(24, TARGET_SAMPLE_RATE, true)
    view.setUint32(28, TARGET_SAMPLE_RATE * 2, true)
    view.setUint16(32, 2, true)
    view.setUint16(34, 16, true)
    writeASCII(view, 36, 'data')
    view.setUint32(40, totalBytes, true)

    let offset = 0
    for (const chunk of chunks) {
      pcm.set(new Uint8Array(chunk), offset)
      offset += chunk.byteLength
    }
    return new File([buf], fileName, { type: 'audio/wav' })
  }

  function resetActiveSegment() {
    activeSegmentChunks = []
    activeSpeechCount = 0
    peakLevel = 0
    trailingSilence = 0
  }

  function finalizeSegment(reason: 'silence' | 'limit' | 'stop') {
    if (activeSegmentChunks.length === 0) return

    let chunks = activeSegmentChunks.slice()
    if (reason === 'silence' && trailingSilence > 0 && trailingSilence < chunks.length)
      chunks = chunks.slice(0, chunks.length - trailingSilence)
    if (chunks.length === 0)
      chunks = activeSegmentChunks.slice()

    if (isNoiseSegment()) {
      resetActiveSegment()
      return
    }

    totalSegments.value += 1
    const bytes = chunks.reduce((s, c) => s + c.byteLength, 0)
    const file = createWav(chunks, `segment-${Date.now()}.wav`)
    uploadQueue.push({ file, duration: bytes / 2 / TARGET_SAMPLE_RATE })
    pendingCount.value = uploadQueue.length
    void debugLog('transcribe.segment', 'queued segment for upload', { reason, chunks: chunks.length, pending: pendingCount.value })
    resetActiveSegment()
    void flushUploadQueue()
  }

  async function flushUploadQueue() {
    if (uploadPromise) return
    uploadPromise = (async () => {
      try {
        while (uploadQueue.length > 0) {
          const item = uploadQueue.shift()!
          pendingCount.value = uploadQueue.length
          status.value = 'uploading'

          const formData = new FormData()
          formData.append('file', item.file)

          try {
            await debugLog('transcribe.upload', 'uploading segment', { pending: uploadQueue.length, duration: item.duration })
            const resp = await authedFetch('/api/asr/realtime-segments', {
              method: 'POST',
              body: formData,
            })
            const json = await readResponseEnvelope<{ text?: string }>(resp)
            if (!resp.ok) {
              lastError.value = json.message || `识别请求失败: ${resp.status}`
              void debugLog('transcribe.error', 'segment upload failed', { status: resp.status, message: lastError.value })
              continue
            }
            const text = normalizeText(json.data?.text)
            if (!text || !hasContent(text)) continue

            appStore.appendHistory(text)
            lastError.value = ''
            await debugLog('transcribe.upload', 'received transcript text', { text })

            // Auto-inject to cursor
            if (appStore.autoInject) {
              const result = await injectText(text)
              if (!result.success) {
                lastError.value = result.message
                void debugLog('inject.error', 'failed to inject transcript', result)
              }
            }
          }
          catch (e) {
            lastError.value = e instanceof Error ? e.message : '识别请求异常'
            void debugLog('transcribe.error', 'segment upload threw', e instanceof Error ? { message: e.message, stack: e.stack } : e)
          }
        }
      }
      finally {
        pendingCount.value = uploadQueue.length
        uploadPromise = null
        if (status.value === 'uploading') status.value = 'listening'
      }
    })()
  }

  function handleChunk(chunk: ArrayBuffer) {
    const copied = chunk.slice(0)
    const rms = computeRms(copied)
    listeningLevel.value = rms
    const threshold = getThreshold()
    const hasSpeech = rms >= threshold

    if (!hasSpeech && activeSegmentChunks.length === 0)
      updateNoiseFloor(rms)

    if (hasSpeech) {
      lastError.value = ''
      if (activeSegmentChunks.length === 0) {
        activeSegmentChunks = [...leadInChunks, copied]
        leadInChunks.splice(0, leadInChunks.length)
      }
      else {
        activeSegmentChunks.push(copied)
      }
      activeSpeechCount += 1
      peakLevel = Math.max(peakLevel, rms)
      trailingSilence = 0
      status.value = 'collecting'
    }
    else if (activeSegmentChunks.length > 0) {
      activeSegmentChunks.push(copied)
      trailingSilence += 1
    }
    else {
      leadInChunks.push(copied)
      if (leadInChunks.length > PRE_ROLL_CHUNKS)
        leadInChunks.splice(0, leadInChunks.length - PRE_ROLL_CHUNKS)
      if (status.value !== 'uploading') status.value = 'listening'
    }

    const s = appStore.recognitionSettings
    if (activeSegmentChunks.length >= MAX_SEGMENT_CHUNKS) {
      finalizeSegment('limit')
      return
    }
    if (activeSegmentChunks.length > 0 && trailingSilence >= s.endSilenceChunks) {
      finalizeSegment('silence')
      return
    }
  }

  function stopAndFlush() {
    finalizeSegment('stop')
    status.value = 'idle'
  }

  function reset() {
    resetActiveSegment()
    leadInChunks.splice(0, leadInChunks.length)
    uploadQueue = []
    uploadPromise = null
    noiseFloorLevel.value = DEFAULT_NOISE_FLOOR
    listeningLevel.value = 0
    status.value = 'idle'
    lastError.value = ''
    totalSegments.value = 0
    pendingCount.value = 0
  }

  return {
    listeningLevel,
    noiseFloorLevel,
    status,
    lastError,
    totalSegments,
    pendingCount,
    handleChunk,
    stopAndFlush,
    reset,
    getThreshold,
  }
}
