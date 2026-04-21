import { ref } from 'vue'
import { useAppStore } from '@/stores/app'
import { useInjector } from './useInjector'
import { useVoiceControl } from './useVoiceControl'
import { authedFetch, ensureRealtimeWorkflowBinding, readResponseEnvelope } from '@/utils/auth'
import { debugLog } from '@/utils/debug'
import { createRealtimeTranscriptionTask, uploadMeetingFromAudio, uploadRealtimeSessionTask } from '@/utils/transcription'

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
  const voiceControl = useVoiceControl()
  void voiceControl.ensureLoaded()

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
  let saveSessionPromise: Promise<void> | null = null
  const sessionAudioChunks: ArrayBuffer[] = []
  const sessionRecognizedTexts: string[] = []

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

  async function applyRealtimeWorkflow(text: string) {
    const workflowId = await ensureRealtimeWorkflowBinding()
    if (workflowId == null)
      return text

    status.value = 'processing'
    try {
      const resp = await authedFetch(`/api/admin/workflows/${workflowId}/execute`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ input_text: text }),
      })
      const json = await readResponseEnvelope<{ final_text?: string, status?: string, error_message?: string }>(resp)
      if (!resp.ok)
        throw new Error(json.message || `工作流执行失败: ${resp.status}`)

      const hasFinalText = Object.prototype.hasOwnProperty.call(json.data || {}, 'final_text')
      const output = normalizeText(hasFinalText ? json.data?.final_text : text)
      if (hasFinalText)
        return output

      if (!output || !hasContent(output))
        return text

      await debugLog('transcribe.workflow', 'applied realtime workflow to segment', {
        workflowId,
        status: json.data?.status || 'completed',
      })
      return output
    }
    catch (error) {
      await debugLog('transcribe.workflow', 'realtime workflow execution failed, fallback to raw text', error instanceof Error ? {
        workflowId,
        message: error.message,
        stack: error.stack,
      } : { workflowId, error })
      return text
    }
    finally {
      if (status.value === 'processing')
        status.value = 'uploading'
    }
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
    if (uploadPromise)
      return uploadPromise

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
            const rawText = normalizeText(json.data?.text)
            if (!rawText || !hasContent(rawText)) continue
            sessionRecognizedTexts.push(rawText)

            // Voice control: detect wake word / classify command. When swallowed,
            // we skip workflow / inject / history append for this segment so the
            // user's command words don't pollute the document.
            const voiceResult = await voiceControl.handleSegmentText(rawText)
            if (voiceResult.swallow) {
              await debugLog('voice.command', 'segment swallowed', { rawText, voiceResult })
              continue
            }

            const text = await applyRealtimeWorkflow(rawText)
            if (!text || !hasContent(text)) {
              await debugLog('transcribe.workflow', 'workflow produced empty segment text', { rawText })
              continue
            }

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
        if (status.value === 'uploading' || status.value === 'processing') status.value = 'listening'
      }
    })()

    return uploadPromise
  }

  async function persistRealtimeSession() {
    const transcript = sessionRecognizedTexts.join('\n').trim()
    if (!transcript)
      return

    if (saveSessionPromise)
      return saveSessionPromise

    const scene = appStore.sceneMode

    saveSessionPromise = (async () => {
      const duration = sessionAudioChunks.reduce((sum, chunk) => sum + chunk.byteLength, 0) / 2 / TARGET_SAMPLE_RATE

      if (scene === 'meeting') {
        await persistMeetingSession(transcript, duration)
        return
      }

      const workflowId = await ensureRealtimeWorkflowBinding().catch(() => appStore.realtimeWorkflowId)

      try {
        if (sessionAudioChunks.length > 0) {
          try {
            const formData = new FormData()
            formData.append('file', createWav(sessionAudioChunks, `realtime-session-${Date.now()}.wav`))
            formData.append('result_text', transcript)
            if (workflowId != null)
              formData.append('workflow_id', String(workflowId))
            await uploadRealtimeSessionTask(formData)
          }
          catch (error) {
            await debugLog('transcribe.session', 'session audio upload failed, fallback to text-only task', error instanceof Error ? {
              message: error.message,
              stack: error.stack,
            } : error)
            await createRealtimeTranscriptionTask({
              result_text: transcript,
              duration,
              workflow_id: workflowId ?? undefined,
            })
          }
        }
        else {
          await createRealtimeTranscriptionTask({
            result_text: transcript,
            duration,
            workflow_id: workflowId ?? undefined,
          })
        }

        await debugLog('transcribe.session', 'saved realtime transcription session', {
          duration,
          workflowId,
          hasAudio: sessionAudioChunks.length > 0,
          segmentCount: sessionRecognizedTexts.length,
        })
      }
      catch (error) {
        lastError.value = error instanceof Error ? error.message : '实时转写历史保存失败'
        await debugLog('transcribe.session', 'failed to save realtime transcription session', error instanceof Error ? {
          message: error.message,
          stack: error.stack,
        } : error)
      }
      finally {
        saveSessionPromise = null
      }
    })()

    return saveSessionPromise
  }

  async function persistMeetingSession(transcript: string, duration: number) {
    try {
      if (sessionAudioChunks.length === 0) {
        // No audio means we cannot create a meeting (会议链路依赖音频). Fall
        // back to a realtime task so the user does not lose the transcript.
        await debugLog('transcribe.session', 'meeting scene without audio, fallback to realtime task')
        await createRealtimeTranscriptionTask({
          result_text: transcript,
          duration,
          workflow_id: appStore.realtimeWorkflowId ?? undefined,
        })
        return
      }
      const formData = new FormData()
      formData.append('file', createWav(sessionAudioChunks, `meeting-session-${Date.now()}.wav`))
      const meetingTitle = `桌面会议 ${new Date().toLocaleString('zh-CN', { hour12: false })}`
      formData.append('title', meetingTitle)
      if (appStore.meetingWorkflowId != null)
        formData.append('workflow_id', String(appStore.meetingWorkflowId))
      const result = await uploadMeetingFromAudio(formData)
      await debugLog('transcribe.session', 'created meeting from desktop session', {
        meetingId: result?.meeting?.id,
        duration,
        segmentCount: sessionRecognizedTexts.length,
      })
    }
    catch (error) {
      lastError.value = error instanceof Error ? error.message : '会议纪要任务创建失败'
      await debugLog('transcribe.session', 'failed to create meeting session', error instanceof Error ? {
        message: error.message,
        stack: error.stack,
      } : error)
    }
    finally {
      saveSessionPromise = null
    }
  }

  function handleChunk(chunk: ArrayBuffer) {
    const copied = chunk.slice(0)
    sessionAudioChunks.push(copied)
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

  async function stopAndFlush() {
    finalizeSegment('stop')
    await flushUploadQueue()
    await persistRealtimeSession()
    status.value = 'idle'
  }

  function reset() {
    voiceControl.reset()
    resetActiveSegment()
    leadInChunks.splice(0, leadInChunks.length)
    sessionAudioChunks.splice(0, sessionAudioChunks.length)
    sessionRecognizedTexts.splice(0, sessionRecognizedTexts.length)
    uploadQueue = []
    uploadPromise = null
    saveSessionPromise = null
    appStore.invalidateWorkflowBindings()
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
