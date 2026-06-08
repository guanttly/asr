import { ref } from 'vue'
import { PRODUCT_CAPABILITY_KEYS, SCENE_MODES } from '@/constants/product'
import { useAppStore } from '@/stores/app'
import { useInjector } from './useInjector'
import { useVoiceControl } from './useVoiceControl'
import { authedFetch, createTimeoutSignal, ensureMeetingWorkflowBinding, ensureProductFeatures, ensureRealtimeWorkflowBinding, readResponseEnvelope } from '@/utils/auth'
import { debugLog } from '@/utils/debug'
import { createRealtimeTranscriptionTask, MEETING_DIRECT_UPLOAD_LIMIT, uploadMeetingFromAudio, uploadMeetingFromAudioChunked, uploadRealtimeSessionTask } from '@/utils/transcription'

const TARGET_SAMPLE_RATE = 16000
const CHUNK_MS = 200
const DEFAULT_NOISE_FLOOR = 0.004
const MAX_SPEECH_RMS = 0.08
const NOISE_FLOOR_SMOOTHING = 0.08
const PRE_ROLL_CHUNKS = 1
const MAX_SEGMENT_CHUNKS = 60
const REALTIME_WORKFLOW_TIMEOUT_MS = 3 * 1000
const AUTO_INJECT_TIMEOUT_MS = 2 * 1000
// 并行处理的断句上限。ASR 上游支持并发，提高此值可在语速较快时更快地
// 消化堆积的断句；但提交（写历史 / 注入光标）仍严格按断句顺序进行。
const MAX_PARALLEL_SEGMENTS = 3
// 防止用户在会议/报告模式间误切换或者误录产生无意义任务：
// 1) 录音时长低于 MIN_MEETING_DURATION_SECONDS 时直接丢弃；
// 2) 没有任何识别文本时直接丢弃。
const MIN_MEETING_DURATION_SECONDS = 5
const MIN_REALTIME_DURATION_SECONDS = 1

interface RealtimeSessionSegment {
  file: File
  rawText: string
  duration: number
  workflowId?: number
}

// 流水线中的单个断句。`seq` 是全局递增序号，用于在并行识别之后仍能严格
// 按时间顺序提交，避免输出“颠三倒四”。
interface PipelineSegment {
  seq: number
  file: File
  duration: number
  workflowId?: number
  /** ASR 识别结果（已归一化）。 */
  rawText: string
  /** rawText 是否含有效内容。 */
  hasText: boolean
  /** 推测性预跑的实时工作流结果；未预跑时为 undefined，由消费端补算。 */
  finalText?: string
  /** 用户停止后被丢弃的“未开始”断句，消费端直接跳过。 */
  dropped: boolean
  /** 并行阶段（ASR + 推测性工作流）完成后兑现。 */
  ready: Promise<void>
  markReady: () => void
}

export function useTranscribe() {
  const appStore = useAppStore()
  const { injectText } = useInjector()
  const voiceControl = useVoiceControl()
  void ensureProductFeatures().catch(() => undefined)
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
  // 并行转写流水线状态。每个断句分配递增的 seq；ASR + 实时工作流并行执行，
  // 但提交（写历史 / 注入光标）由单一消费协程严格按 seq 顺序进行。
  let nextSeq = 0
  let commitSeq = 0
  let activeWorkers = 0
  let workflowActiveCount = 0
  // 用户停止后置为 true：阻断后续光标注入，并丢弃尚未开始的断句队列。
  let pipelineAborted = false
  let consumerPromise: Promise<void> | null = null
  const pendingSegments: PipelineSegment[] = []
  const segmentsBySeq = new Map<number, PipelineSegment>()
  let saveSessionPromise: Promise<void> | null = null
  const sessionAudioChunks: ArrayBuffer[] = []
  const sessionRecognizedTexts: string[] = []
  const sessionRealtimeSegments: RealtimeSessionSegment[] = []

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

  async function withTimeout<T>(promise: Promise<T>, timeoutMs: number, timeoutFactory: () => T): Promise<T> {
    let timer: ReturnType<typeof setTimeout> | null = null
    try {
      return await Promise.race([
        promise,
        new Promise<T>((resolve) => {
          timer = setTimeout(() => resolve(timeoutFactory()), timeoutMs)
        }),
      ])
    }
    finally {
      if (timer)
        clearTimeout(timer)
    }
  }

  async function applyRealtimeWorkflow(text: string) {
    const workflowId = await ensureRealtimeWorkflowBinding()
    if (workflowId == null)
      return text

    const { signal, cleanup } = createTimeoutSignal(REALTIME_WORKFLOW_TIMEOUT_MS)
    try {
      const resp = await authedFetch(`/api/admin/workflows/${workflowId}/execute`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ input_text: text }),
        signal,
      })
      const json = await readResponseEnvelope<{ final_text?: string, status?: string, error_message?: string }>(resp)
      if (!resp.ok)
        throw new Error(json.message || `工作流执行失败: ${resp.status}`)
      if (json.data?.status === 'failed')
        throw new Error(json.data.error_message || json.message || '实时工作流执行失败')

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
      if (error instanceof Error && error.name === 'AbortError') {
        await debugLog('transcribe.workflow', 'realtime workflow request timed out, fallback to raw text', {
          workflowId,
          timeoutMs: REALTIME_WORKFLOW_TIMEOUT_MS,
        })
        return text
      }

      await debugLog('transcribe.workflow', 'realtime workflow execution failed, fallback to raw text', error instanceof Error ? {
        workflowId,
        message: error.message,
        stack: error.stack,
      } : { workflowId, error })
      return text
    }
    finally {
      cleanup()
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
    enqueueSegment(file, bytes / 2 / TARGET_SAMPLE_RATE, reason)
    resetActiveSegment()
  }

  function uncommittedCount(): number {
    return Math.max(0, nextSeq - commitSeq)
  }

  // 集中计算录音过程中的状态展示，避免多个并行任务相互覆盖状态。
  function refreshStatus() {
    if (activeSegmentChunks.length > 0) {
      status.value = 'collecting'
      return
    }
    if (workflowActiveCount > 0) {
      status.value = 'processing'
      return
    }
    status.value = uncommittedCount() > 0 ? 'uploading' : 'listening'
  }

  // 入队一个断句：分配 seq，立即在并发上限内启动 ASR（并行阶段），
  // 同时确保有序消费协程在运行。
  function enqueueSegment(file: File, duration: number, reason: 'silence' | 'limit' | 'stop') {
    const seq = nextSeq++
    let markReady: () => void = () => {}
    const ready = new Promise<void>((resolve) => {
      markReady = resolve
    })
    const segment: PipelineSegment = {
      seq,
      file,
      duration,
      rawText: '',
      hasText: false,
      dropped: false,
      ready,
      markReady,
    }
    segmentsBySeq.set(seq, segment)
    pendingSegments.push(segment)
    pendingCount.value = uncommittedCount()
    void debugLog('transcribe.segment', 'queued segment for transcription', { reason, seq, pending: pendingCount.value })
    refreshStatus()
    pumpWorkers()
    ensureConsumer()
  }

  // 并行阶段调度器：在 MAX_PARALLEL_SEGMENTS 内尽量多地启动 ASR worker。
  function pumpWorkers() {
    while (activeWorkers < MAX_PARALLEL_SEGMENTS && pendingSegments.length > 0) {
      const segment = pendingSegments.shift()!
      activeWorkers += 1
      void processSegment(segment).finally(() => {
        activeWorkers -= 1
        pumpWorkers()
      })
    }
  }

  async function transcribeSegment(segment: PipelineSegment): Promise<string> {
    const formData = new FormData()
    formData.append('file', segment.file)
    // Realtime workflow drives both LLM correction and (via term-correction
    // node config) the ASR hotword/dictionary. Sending workflow_id lets the
    // backend resolve dict_id → push hotwords to the upstream qwen3-asr call.
    const realtimeWorkflowId = await ensureRealtimeWorkflowBinding().catch(() => appStore.realtimeWorkflowId)
    if (realtimeWorkflowId != null)
      formData.append('workflow_id', String(realtimeWorkflowId))
    segment.workflowId = realtimeWorkflowId ?? undefined

    await debugLog('transcribe.upload', 'uploading segment', { seq: segment.seq, duration: segment.duration, workflowId: realtimeWorkflowId })
    const resp = await authedFetch('/api/asr/realtime-segments', {
      method: 'POST',
      body: formData,
    })
    const json = await readResponseEnvelope<{ text?: string }>(resp)
    if (!resp.ok) {
      lastError.value = json.message || `识别请求失败: ${resp.status}`
      void debugLog('transcribe.error', 'segment upload failed', { seq: segment.seq, status: resp.status, message: lastError.value })
      return ''
    }
    return normalizeText(json.data?.text)
  }

  async function runWorkflow(rawText: string): Promise<string> {
    workflowActiveCount += 1
    refreshStatus()
    try {
      return await applyRealtimeWorkflow(rawText)
    }
    finally {
      workflowActiveCount -= 1
      refreshStatus()
    }
  }

  // 并行阶段：执行 ASR，并对（普通报告场景的）口述断句推测性地预跑实时
  // 工作流，使有序消费端基本无需再等待 LLM。此处不触碰 voice control /
  // 历史 / 注入等顺序敏感状态，结果只写入该断句自身。
  async function processSegment(segment: PipelineSegment) {
    try {
      const rawText = await transcribeSegment(segment)
      segment.rawText = rawText
      segment.hasText = !!rawText && hasContent(rawText)
      if (segment.hasText && !pipelineAborted
        && appStore.sceneMode === SCENE_MODES.REPORT && !appStore.voiceCommandActive) {
        segment.finalText = await runWorkflow(rawText)
      }
    }
    catch (e) {
      lastError.value = e instanceof Error ? e.message : '识别请求异常'
      void debugLog('transcribe.error', 'segment processing threw', e instanceof Error ? { message: e.message, stack: e.stack } : e)
    }
    finally {
      segment.markReady()
      refreshStatus()
    }
  }

  // 有序消费协程：严格按 seq 顺序提交结果，保证输出不“颠三倒四”。
  function ensureConsumer() {
    if (consumerPromise)
      return
    consumerPromise = (async () => {
      try {
        while (commitSeq < nextSeq) {
          const segment = segmentsBySeq.get(commitSeq)
          if (!segment) {
            commitSeq += 1
            continue
          }
          await segment.ready
          segmentsBySeq.delete(segment.seq)
          commitSeq += 1
          pendingCount.value = uncommittedCount()
          await commitSegment(segment)
          refreshStatus()
        }
      }
      finally {
        consumerPromise = null
        if (commitSeq < nextSeq)
          ensureConsumer()
        else
          refreshStatus()
      }
    })()
  }

  // 有序阶段：voice control 是有状态状态机，必须按序执行；写历史与注入光标
  // 同样按序进行。注入受 pipelineAborted 门控——用户停止后不再粘贴。
  async function commitSegment(segment: PipelineSegment) {
    if (segment.dropped)
      return
    const rawText = segment.rawText
    if (!rawText || !segment.hasText)
      return

    // Voice control: detect wake word / classify command. When swallowed, we
    // skip workflow / inject / history append for this segment so the user's
    // command words don't pollute the document.
    const voiceResult = await voiceControl.handleSegmentText(rawText)
    if (voiceResult.swallow) {
      await debugLog('voice.command', 'segment swallowed', { rawText, voiceResult })
      return
    }

    sessionRecognizedTexts.push(rawText)

    if (appStore.sceneMode !== SCENE_MODES.REPORT) {
      lastError.value = ''
      await debugLog('transcribe.upload', 'skip report write path outside report scene', {
        scene: appStore.sceneMode,
        text: rawText,
      })
      return
    }

    // 若并行阶段因场景切换等原因未预跑工作流，则在此按序补算。
    let text = segment.finalText
    if (text === undefined)
      text = await runWorkflow(rawText)
    if (!text || !hasContent(text)) {
      await debugLog('transcribe.workflow', 'workflow produced empty segment text', { rawText })
      return
    }

    sessionRealtimeSegments.push({
      file: segment.file,
      rawText,
      duration: segment.duration,
      workflowId: segment.workflowId,
    })

    appStore.appendHistory(text)
    lastError.value = ''
    await debugLog('transcribe.upload', 'received transcript text', { text })

    // Auto-inject to cursor. Once the user has stopped (pipelineAborted), the
    // link to the cursor is severed so nothing is pasted after they release the
    // mic, even if this segment was already mid-transcription.
    if (appStore.autoInject && !pipelineAborted) {
      const result = await withTimeout(injectText(text), AUTO_INJECT_TIMEOUT_MS, () => ({
        success: false,
        message: '文本注入超时，已跳过本次自动粘贴',
      }))
      if (!result.success) {
        lastError.value = result.message
        void debugLog('inject.error', 'failed to inject transcript', result)
      }
    }
    else if (appStore.autoInject && pipelineAborted) {
      await debugLog('inject.skip', 'injection blocked after stop', { seq: segment.seq })
    }
  }

  // 用户停止后立即丢弃尚未开始识别的断句队列。
  function clearPendingQueue() {
    if (pendingSegments.length === 0)
      return
    const dropped = pendingSegments.splice(0, pendingSegments.length)
    for (const segment of dropped) {
      segment.dropped = true
      segment.markReady()
    }
    void debugLog('transcribe.segment', 'cleared not-started transcription queue on stop', { dropped: dropped.length })
    pendingCount.value = uncommittedCount()
  }

  // 等待已在识别中的断句与有序消费协程全部收尾。
  async function drainPipeline() {
    while (consumerPromise)
      await consumerPromise
  }

  async function persistRealtimeSegment(segment: RealtimeSessionSegment) {
    try {
      const formData = new FormData()
      formData.append('file', segment.file)
      formData.append('result_text', segment.rawText)
      if (segment.workflowId != null)
        formData.append('workflow_id', String(segment.workflowId))
      await uploadRealtimeSessionTask(formData)
    }
    catch (error) {
      await debugLog('transcribe.session', 'segment audio upload failed, fallback to text-only task', error instanceof Error ? {
        message: error.message,
        stack: error.stack,
      } : error)
      await createRealtimeTranscriptionTask({
        result_text: segment.rawText,
        duration: segment.duration,
        workflow_id: segment.workflowId ?? undefined,
      })
    }
  }

  async function persistRealtimeSession() {
    const transcript = sessionRecognizedTexts.join('\n').trim()
    if (!transcript) {
      await debugLog('transcribe.session', 'skip persistence: empty transcript', {
        scene: appStore.sceneMode,
      })
      return
    }

    if (saveSessionPromise)
      return saveSessionPromise

    const scene = appStore.sceneMode

    saveSessionPromise = (async () => {
      const duration = sessionAudioChunks.reduce((sum, chunk) => sum + chunk.byteLength, 0) / 2 / TARGET_SAMPLE_RATE

      if (scene === SCENE_MODES.MEETING && appStore.hasCapability(PRODUCT_CAPABILITY_KEYS.MEETING)) {
        if (duration < MIN_MEETING_DURATION_SECONDS) {
          await debugLog('transcribe.session', 'skip meeting persistence: duration too short', {
            duration,
            minimum: MIN_MEETING_DURATION_SECONDS,
          })
          saveSessionPromise = null
          return
        }
        await persistMeetingSession(transcript, duration)
        return
      }

      if (duration < MIN_REALTIME_DURATION_SECONDS) {
        await debugLog('transcribe.session', 'skip realtime persistence: duration too short', {
          duration,
          minimum: MIN_REALTIME_DURATION_SECONDS,
        })
        saveSessionPromise = null
        return
      }

      if (sessionRealtimeSegments.length === 0) {
        await debugLog('transcribe.session', 'skip realtime persistence: no finalized segments', {
          duration,
          transcriptLength: transcript.length,
        })
        saveSessionPromise = null
        return
      }

      try {
        let savedCount = 0
        let failedCount = 0

        for (const [index, segment] of sessionRealtimeSegments.entries()) {
          try {
            await persistRealtimeSegment(segment)
            savedCount += 1
          }
          catch (error) {
            failedCount += 1
            lastError.value = error instanceof Error ? error.message : '实时转写历史保存失败'
            await debugLog('transcribe.session', 'failed to save realtime segment history', error instanceof Error ? {
              index,
              rawText: segment.rawText,
              duration: segment.duration,
              message: error.message,
              stack: error.stack,
            } : {
              index,
              rawText: segment.rawText,
              duration: segment.duration,
              error,
            })
          }
        }

        if (failedCount > 0 && savedCount > 0)
          lastError.value = `已有 ${savedCount} 条断句保存，${failedCount} 条失败`

        await debugLog('transcribe.session', 'saved realtime transcription segments', {
          duration,
          savedCount,
          failedCount,
          segmentCount: sessionRealtimeSegments.length,
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
      const wavFile = createWav(sessionAudioChunks, `meeting-session-${Date.now()}.wav`)
      const meetingTitle = `桌面会议 ${new Date().toLocaleString('zh-CN', { hour12: false })}`
      const meetingWorkflowId = await ensureMeetingWorkflowBinding()
        .catch(() => appStore.meetingWorkflowId)

      let result: Awaited<ReturnType<typeof uploadMeetingFromAudio>>
      if (wavFile.size > MEETING_DIRECT_UPLOAD_LIMIT) {
        // Long recordings exceed the single-request size limits of nginx and the
        // backend, so upload them in sequential chunks.
        await debugLog('transcribe.session', 'meeting recording large, using chunked upload', {
          bytes: wavFile.size,
          duration,
        })
        result = await uploadMeetingFromAudioChunked(wavFile, {
          title: meetingTitle,
          workflow_id: meetingWorkflowId ?? undefined,
        })
      }
      else {
        const formData = new FormData()
        formData.append('file', wavFile)
        formData.append('title', meetingTitle)
        if (meetingWorkflowId != null)
          formData.append('workflow_id', String(meetingWorkflowId))
        result = await uploadMeetingFromAudio(formData)
      }
      await debugLog('transcribe.session', 'created meeting from desktop session', {
        meetingId: result?.meeting?.id,
        duration,
        bytes: wavFile.size,
        segmentCount: sessionRecognizedTexts.length,
        workflowId: meetingWorkflowId,
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
      refreshStatus()
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
    // 立刻阻断光标注入：用户一旦停止，正在识别中的内容也不再粘贴到光标。
    pipelineAborted = true
    // 先丢弃尚未开始识别的历史堆积队列，避免停止后继续处理积压内容。
    clearPendingQueue()
    // 再入队用户停止前的最后一段口述，确保末尾内容仍会被识别并落库（但不再注入）。
    finalizeSegment('stop')
    // 已在识别中的断句仍会写入历史以便落库，但不会再注入光标。
    await drainPipeline()
    await persistRealtimeSession()
    status.value = 'idle'
  }

  function reset() {
    voiceControl.reset()
    resetActiveSegment()
    leadInChunks.splice(0, leadInChunks.length)
    sessionAudioChunks.splice(0, sessionAudioChunks.length)
    sessionRecognizedTexts.splice(0, sessionRecognizedTexts.length)
    sessionRealtimeSegments.splice(0, sessionRealtimeSegments.length)
    pendingSegments.splice(0, pendingSegments.length)
    segmentsBySeq.clear()
    nextSeq = 0
    commitSeq = 0
    activeWorkers = 0
    workflowActiveCount = 0
    pipelineAborted = false
    consumerPromise = null
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
