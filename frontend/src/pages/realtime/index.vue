<script setup lang="ts">
import { NTag, useMessage } from 'naive-ui'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'

import { createTranscriptionTask, getTranscriptionTaskExecutions, transcribeRealtimeSegment } from '@/api/asr'
import { executeWorkflow } from '@/api/workflow'
import NodeDetailPanel from '@/components/NodeDetailPanel.vue'
import TextDiffPreview from '@/components/TextDiffPreview.vue'
import WorkflowSelectionPreview from '@/components/WorkflowSelectionPreview.vue'
import { useAudioRecorder } from '@/composables/useAudioRecorder'
import { useWorkflowBindingStatus } from '@/composables/useWorkflowBindingStatus'
import { useWorkflowCatalog } from '@/composables/useWorkflowCatalog'
import { useTranscriptionStore } from '@/stores/transcription'

interface ExecutionNodeResult {
  id: number
  node_type: string
  label: string
  position: number
  input_text?: string
  output_text?: string
  status: string
  detail?: Record<string, unknown> | string | null
  duration_ms?: number
}

interface ExecutionItem {
  id: number
  final_text?: string
  status: string
  error_message?: string
  created_at?: string
  node_results?: ExecutionNodeResult[]
}

interface SavedTaskSummary {
  id: number
  workflow_id?: number
  meeting_id?: number
  result_text?: string
  post_process_status?: string
  post_process_error?: string
}

interface SegmentUploadItem {
  file: File
  duration: number
}

interface RealtimeRecognitionSettings {
  keepPunctuation: boolean
  minSpeechThreshold: number | null
  noiseGateMultiplier: number | null
  endSilenceChunks: number | null
  minEffectiveSpeechChunks: number | null
  singleChunkPeakMultiplier: number | null
}

interface NormalizedRealtimeRecognitionSettings {
  keepPunctuation: boolean
  minSpeechThreshold: number
  noiseGateMultiplier: number
  endSilenceChunks: number
  minEffectiveSpeechChunks: number
  singleChunkPeakMultiplier: number
}

const TARGET_SAMPLE_RATE = 16000
const CHUNK_MS = 300
const DEFAULT_NOISE_FLOOR_LEVEL = 0.004
const DEFAULT_MIN_SPEECH_RMS_THRESHOLD = 0.018
const MAX_SPEECH_RMS_THRESHOLD = 0.08
const NOISE_FLOOR_SMOOTHING = 0.08
const DEFAULT_NOISE_GATE_MULTIPLIER = 2.8
const PRE_ROLL_CHUNKS = 1
const DEFAULT_END_SILENCE_CHUNKS = 4
const MAX_SEGMENT_CHUNKS = 40
const DEFAULT_MIN_EFFECTIVE_SPEECH_CHUNKS = 2
const DEFAULT_SINGLE_CHUNK_PEAK_MULTIPLIER = 1.45
const REALTIME_SETTINGS_STORAGE_KEY = 'asr.realtime.recognition.settings'
const REALTIME_CONFIG_PANEL_STORAGE_KEY = 'asr.realtime.config.panel.expanded'

const DEFAULT_REALTIME_SETTINGS: RealtimeRecognitionSettings = {
  keepPunctuation: false,
  minSpeechThreshold: DEFAULT_MIN_SPEECH_RMS_THRESHOLD,
  noiseGateMultiplier: DEFAULT_NOISE_GATE_MULTIPLIER,
  endSilenceChunks: DEFAULT_END_SILENCE_CHUNKS,
  minEffectiveSpeechChunks: DEFAULT_MIN_EFFECTIVE_SPEECH_CHUNKS,
  singleChunkPeakMultiplier: DEFAULT_SINGLE_CHUNK_PEAK_MULTIPLIER,
}

const message = useMessage()
const router = useRouter()
const store = useTranscriptionStore()
const realtimeWorkflowCatalog = useWorkflowCatalog('realtime_transcription', 100)
const {
  configuredWorkflowId,
  configuredWorkflow: selectedWorkflowOption,
  configuredWorkflowMissing,
  configuredWorkflowNotice: configuredWorkflowMessage,
} = useWorkflowBindingStatus('realtime', realtimeWorkflowCatalog, {
  emptyLabel: '未配置默认工作流',
  unsetMessage: '当前未配置实时应用默认工作流，停止后会保存转写结果，但不会自动触发后处理。',
  missingMessage: workflowId => `应用配置中的实时工作流 #${workflowId} 当前不可用，请前往应用配置页重新选择。`,
  readyMessage: () => '录音结束后会自动套用应用配置页中绑定的实时工作流。',
})
const { start, stop, pause, resume, isRecording, isPaused } = useAudioRecorder()
const savingSession = ref(false)
const stoppingSession = ref(false)
const recordingStartedAt = ref<number | null>(null)
const sessionWorkflowId = ref<number | null>(null)
const latestTask = ref<SavedTaskSummary | null>(null)
const latestExecutions = ref<ExecutionItem[]>([])
const executionLoading = ref(false)
const instantProcessing = ref(false)
const instantQueueSize = ref(0)
const instantProcessingError = ref('')
const instantNodeResults = ref<ExecutionNodeResult[]>([])
const segmentUploadError = ref('')
const uploadQueueSize = ref(0)
const uploadState = ref<'idle' | 'uploading'>('idle')
const listeningLevel = ref(0)
const noiseFloorLevel = ref(DEFAULT_NOISE_FLOOR_LEVEL)
const realtimeSettings = ref<RealtimeRecognitionSettings>(loadRealtimeRecognitionSettings())
const realtimeConfigPanelExpanded = ref(loadRealtimeConfigPanelExpanded())
const totalSegmentCount = ref(0)
const activeSegmentChunkCount = ref(0)
const activeSpeechChunkCount = ref(0)
const activeSegmentPeakLevel = ref(0)
const trailingSilenceChunkCount = ref(0)
const activeSegmentDurationMs = ref(0)

const pendingInstantChunks: string[] = []
let instantProcessingPromise: Promise<void> | null = null
const pendingSegmentUploads: SegmentUploadItem[] = []
const leadInChunks: ArrayBuffer[] = []
let activeSegmentChunks: ArrayBuffer[] = []
let segmentUploadPromise: Promise<void> | null = null
let activeSegmentStartedAt: number | null = null

const effectiveRealtimeSettings = computed<NormalizedRealtimeRecognitionSettings>(() => normalizeRealtimeRecognitionSettings(realtimeSettings.value))
const speechThresholdLevel = computed(() => clamp(
  Math.max(
    effectiveRealtimeSettings.value.minSpeechThreshold,
    noiseFloorLevel.value * effectiveRealtimeSettings.value.noiseGateMultiplier,
  ),
  effectiveRealtimeSettings.value.minSpeechThreshold,
  MAX_SPEECH_RMS_THRESHOLD,
))

const chunkCount = computed(() => totalSegmentCount.value)
const finalCount = computed(() => store.totalSentenceCount)
const latestExecution = computed(() => latestExecutions.value[0] || null)
const hasSessionWorkflow = computed(() => sessionWorkflowId.value != null)
const sessionWorkflowLabel = computed(() => workflowLabel(sessionWorkflowId.value))
const effectiveImmediateWorkflowId = computed(() => isRecording.value
  ? sessionWorkflowId.value
  : (configuredWorkflowMissing.value ? null : (configuredWorkflowId.value ?? null)))
const hasImmediateWorkflow = computed(() => effectiveImmediateWorkflowId.value != null)
const effectiveWorkflowLabel = computed(() => workflowLabel(effectiveImmediateWorkflowId.value))
const effectiveOutputText = computed(() => store.processedTranscriptText.trim() || store.transcriptText.trim())
const realtimeConfigSummary = computed(() => {
  const workflowSummary = hasImmediateWorkflow.value ? effectiveWorkflowLabel.value : '未绑定即时工作流'
  const punctuationSummary = effectiveRealtimeSettings.value.keepPunctuation ? '保留标点' : '过滤标点'
  const thresholdSummary = `阈值 ${speechThresholdLevel.value.toFixed(3)} / 静音 ${effectiveRealtimeSettings.value.endSilenceChunks} 块`
  return `${workflowSummary} · ${punctuationSummary} · ${thresholdSummary}`
})
const immediateOutputTitle = computed(() => hasImmediateWorkflow.value ? '即时输出（已处理）' : '即时输出（原始识别直出）')
const immediateOutputDescription = computed(() => {
  if (hasImmediateWorkflow.value)
    return '每个本地停顿切出的句子都会立即套用本次会话绑定的实时工作流，输出可直接复制到剪贴板。'
  return '当前会话未绑定可用实时工作流，系统会按停顿逐句输出原始识别结果。'
})
const instantProcessingNotice = computed(() => {
  if (isRecording.value && hasSessionWorkflow.value)
    return '当前场景按“说完一句再识别”工作。前端会在本地检测停顿并提交整句音频，每句识别完成后立即执行一次实时工作流；停止录音后系统还会再对整段文本做一次完整复核并保存历史任务。'
  if (hasImmediateWorkflow.value)
    return '当前已配置实时应用默认工作流。开始录音后，前端会在本地按停顿分句，每句识别完成后都会即时进入这条工作流；停止录音时系统还会再做一次整段复核。'
  if (configuredWorkflowMissing.value)
    return configuredWorkflowMessage.value
  return '当前未配置实时应用默认工作流。系统仍会按停顿逐句识别，但即时输出不会做术语纠错或文本后处理。'
})
const transportStateLabel = computed(() => {
  if (stoppingSession.value || savingSession.value)
    return '正在收尾'
  if (activeSegmentChunkCount.value > 0)
    return trailingSilenceChunkCount.value > 0 ? '等待句尾' : '正在收句'
  if (isRecording.value)
    return uploadState.value === 'uploading' || uploadQueueSize.value > 0 ? '识别处理中' : '监听中'
  if (uploadState.value === 'uploading' || uploadQueueSize.value > 0 || instantProcessing.value)
    return '处理中'
  return '本地分句待命'
})
const transportStateDescription = computed(() => {
  if (isRecording.value)
    return `本地停顿检测 + HTTP 整句上传，当前能量 ${listeningLevel.value.toFixed(3)} / 触发阈值 ${speechThresholdLevel.value.toFixed(3)} / 底噪 ${noiseFloorLevel.value.toFixed(3)}`
  return '浏览器本地切句后再调用短句识别接口，不再维持长连接流式会话。'
})

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value))
}

function clampInteger(value: number, min: number, max: number) {
  return Math.round(clamp(value, min, max))
}

function normalizeRealtimeRecognitionSettings(value?: Partial<RealtimeRecognitionSettings> | null): NormalizedRealtimeRecognitionSettings {
  const raw = value || {}
  return {
    keepPunctuation: Boolean(raw.keepPunctuation),
    minSpeechThreshold: clamp(Number(raw.minSpeechThreshold) || DEFAULT_MIN_SPEECH_RMS_THRESHOLD, 0.005, MAX_SPEECH_RMS_THRESHOLD),
    noiseGateMultiplier: clamp(Number(raw.noiseGateMultiplier) || DEFAULT_NOISE_GATE_MULTIPLIER, 1.2, 6),
    endSilenceChunks: clampInteger(Number(raw.endSilenceChunks) || DEFAULT_END_SILENCE_CHUNKS, 2, 10),
    minEffectiveSpeechChunks: clampInteger(Number(raw.minEffectiveSpeechChunks) || DEFAULT_MIN_EFFECTIVE_SPEECH_CHUNKS, 1, 6),
    singleChunkPeakMultiplier: clamp(Number(raw.singleChunkPeakMultiplier) || DEFAULT_SINGLE_CHUNK_PEAK_MULTIPLIER, 1, 3),
  }
}

function loadRealtimeRecognitionSettings() {
  if (typeof window === 'undefined')
    return { ...DEFAULT_REALTIME_SETTINGS }

  try {
    const stored = window.localStorage.getItem(REALTIME_SETTINGS_STORAGE_KEY)
    if (!stored)
      return { ...DEFAULT_REALTIME_SETTINGS }
    return normalizeRealtimeRecognitionSettings(JSON.parse(stored) as Partial<RealtimeRecognitionSettings>)
  }
  catch {
    return { ...DEFAULT_REALTIME_SETTINGS }
  }
}

function loadRealtimeConfigPanelExpanded() {
  if (typeof window === 'undefined')
    return false

  try {
    return window.localStorage.getItem(REALTIME_CONFIG_PANEL_STORAGE_KEY) === '1'
  }
  catch {
    return false
  }
}

function saveRealtimeConfigPanelExpanded(expanded: boolean) {
  if (typeof window === 'undefined')
    return
  window.localStorage.setItem(REALTIME_CONFIG_PANEL_STORAGE_KEY, expanded ? '1' : '0')
}

function saveRealtimeRecognitionSettings(value: RealtimeRecognitionSettings) {
  if (typeof window === 'undefined')
    return
  window.localStorage.setItem(REALTIME_SETTINGS_STORAGE_KEY, JSON.stringify(normalizeRealtimeRecognitionSettings(value)))
}

function resetRealtimeRecognitionSettings() {
  realtimeSettings.value = { ...DEFAULT_REALTIME_SETTINGS }
  noiseFloorLevel.value = DEFAULT_NOISE_FLOOR_LEVEL
}

function recalibrateNoiseFloor() {
  noiseFloorLevel.value = DEFAULT_NOISE_FLOOR_LEVEL
}

function toggleRealtimeConfigPanel() {
  realtimeConfigPanelExpanded.value = !realtimeConfigPanelExpanded.value
}

const latestTaskWorkflowPendingUpgrade = computed(() => {
  const workflowId = latestTask.value?.workflow_id
  if (!workflowId)
    return false
  return !realtimeWorkflowCatalog.hasWorkflow(workflowId)
})
const latestTaskWorkflowPendingUpgradeMessage = computed(() => {
  if (!latestTaskWorkflowPendingUpgrade.value || !latestTask.value?.workflow_id)
    return ''
  return `当前任务绑定的工作流 #${latestTask.value.workflow_id} 不在可用实时工作流列表中，通常表示它仍是待升级的 legacy 工作流。请升级该工作流后再继续绑定到新的实时任务。`
})

const latestExecutionSummary = computed(() => {
  if (!latestTask.value)
    return { label: '未保存', type: 'default' as const, detail: '当前会话还没有保存为实时任务。' }
  if (!latestTask.value.workflow_id)
    return { label: '未绑定工作流', type: 'default' as const, detail: '本次实时转写保存时未绑定工作流，因此不会产生后处理执行记录。' }
  if (executionLoading.value)
    return { label: '加载中', type: 'info' as const, detail: '正在拉取当前任务的工作流执行记录。' }
  if (!latestExecution.value)
    return { label: '未返回记录', type: 'warning' as const, detail: '当前任务已绑定工作流，但尚未查询到执行记录。' }

  const status = latestExecution.value.status
  const type = status === 'failed'
    ? 'error'
    : status === 'completed' || status === 'success'
      ? 'success'
      : 'info'
  const detail = latestExecution.value.error_message?.trim()
    || (latestExecution.value.created_at ? `最近执行时间：${formatDateTime(latestExecution.value.created_at)}` : '工作流已返回执行记录。')

  return {
    label: formatExecutionStatus(status),
    type,
    detail,
  }
})

const latestSaveSummary = computed(() => {
  if (!latestTask.value)
    return { label: '未保存', type: 'default' as const, detail: '停止录音后，实时转写会保存为历史任务。' }

  const status = latestTask.value.post_process_status
  if (!status)
    return { label: '已保存', type: 'success' as const, detail: `任务 #${latestTask.value.id} 已保存。` }

  const type = status === 'failed'
    ? 'error'
    : status === 'completed'
      ? 'success'
      : status === 'processing'
        ? 'info'
        : 'default'

  return {
    label: formatPostProcessStatus(status),
    type,
    detail: latestTask.value.post_process_error?.trim() || `任务 #${latestTask.value.id} 的后处理状态已更新。`,
  }
})

function sanitizeText(value?: string) {
  if (!value)
    return ''
  return value
    .replace(/language\s+[a-z_-]+<asr_text>/gi, '')
    .replace(/<\/?asr_text>/gi, '')
    .replace(/<\|[^>]+\|>/g, '')
    .replace(/\u00A0/g, ' ')
    .trim()
}

function stripPunctuation(value: string) {
  return value
    .replace(/[\p{P}\p{S}]/gu, '')
    .replace(/\s{2,}/g, ' ')
    .trim()
}

function normalizeRecognizedText(value?: string) {
  const normalized = sanitizeText(value)
  if (!normalized)
    return ''
  if (effectiveRealtimeSettings.value.keepPunctuation)
    return normalized
  return stripPunctuation(normalized)
}

function hasRecognizedContent(value?: string) {
  return /[A-Z0-9\u3400-\u9FFF]/i.test(normalizeRecognizedText(value))
}

function formatDateTime(value?: string) {
  if (!value)
    return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime()))
    return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

function formatExecutionStatus(value?: string) {
  const map: Record<string, string> = {
    pending: '待执行',
    running: '执行中',
    completed: '已完成',
    failed: '失败',
    success: '成功',
    skipped: '跳过',
  }
  return map[value || ''] || value || '-'
}

function formatPostProcessStatus(value?: string) {
  const map: Record<string, string> = {
    pending: '待处理',
    processing: '处理中',
    completed: '已完成',
    failed: '失败',
  }
  return map[value || ''] || value || '-'
}

function workflowLabel(workflowId?: number | null) {
  if (!workflowId)
    return '未选择工作流'
  return realtimeWorkflowCatalog.labelForWorkflow(workflowId, '未选择工作流')
}

function writeASCII(view: DataView, offset: number, value: string) {
  for (let i = 0; i < value.length; i++)
    view.setUint8(offset + i, value.charCodeAt(i))
}

function cloneChunk(chunk: ArrayBuffer) {
  return chunk.slice(0)
}

function computePCM16Rms(chunk: ArrayBuffer) {
  const samples = new Int16Array(chunk)
  if (samples.length === 0)
    return 0

  let sumSquares = 0
  for (let i = 0; i < samples.length; i++) {
    const normalized = samples[i] / 0x8000
    sumSquares += normalized * normalized
  }

  return Math.sqrt(sumSquares / samples.length)
}

function pcm16BytesToDurationSeconds(bytes: number) {
  return bytes / 2 / TARGET_SAMPLE_RATE
}

function createWavFileFromChunks(chunks: ArrayBuffer[], fileName: string) {
  const totalBytes = chunks.reduce((sum, chunk) => sum + chunk.byteLength, 0)
  const wavBuffer = new ArrayBuffer(44 + totalBytes)
  const view = new DataView(wavBuffer)
  const pcmPayload = new Uint8Array(wavBuffer, 44)

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
    pcmPayload.set(new Uint8Array(chunk), offset)
    offset += chunk.byteLength
  }

  return new File([wavBuffer], fileName, { type: 'audio/wav' })
}

function extractErrorMessage(error: unknown, fallback: string) {
  if (typeof error === 'object' && error !== null) {
    const response = (error as { response?: { data?: { message?: string } } }).response
    const messageText = response?.data?.message
    if (typeof messageText === 'string' && messageText.trim())
      return messageText.trim()
  }
  if (error instanceof Error && error.message.trim())
    return error.message.trim()
  return fallback
}

function queueImmediateChunk(text: string) {
  const normalized = normalizeRecognizedText(text)
  if (!normalized)
    return

  pendingInstantChunks.push(normalized)
  instantQueueSize.value = pendingInstantChunks.length
  void flushImmediateProcessingQueue()
}

function flushImmediateProcessingQueue() {
  if (instantProcessingPromise)
    return instantProcessingPromise

  instantProcessingPromise = (async () => {
    instantProcessing.value = true
    try {
      while (pendingInstantChunks.length > 0) {
        const chunk = pendingInstantChunks.shift()
        instantQueueSize.value = pendingInstantChunks.length
        if (!chunk)
          continue

        if (!sessionWorkflowId.value) {
          store.appendProcessedSentence(chunk)
          continue
        }

        try {
          const result = await executeWorkflow(sessionWorkflowId.value, { input_text: chunk })
          const output = normalizeRecognizedText(result.data?.final_text || chunk) || chunk
          store.appendProcessedSentence(output)
          if (Array.isArray(result.data?.node_results) && result.data.node_results.length > 0)
            instantNodeResults.value = result.data.node_results
          instantProcessingError.value = result.data?.status === 'failed'
            ? (result.data?.error_message || '即时工作流执行存在失败节点，当前片段已按返回结果输出。')
            : ''
        }
        catch {
          store.appendProcessedSentence(chunk)
          instantProcessingError.value = '即时工作流执行失败，当前片段已回退为原始识别输出。'
          message.warning('即时处理失败，当前片段已按原始识别输出')
        }
      }
    }
    finally {
      instantQueueSize.value = pendingInstantChunks.length
      instantProcessing.value = false
      instantProcessingPromise = null
    }
  })()

  return instantProcessingPromise
}

function updateDraftText() {
  if (!isRecording.value) {
    store.setDraftText('')
    return
  }
  if (isPaused.value) {
    store.setDraftText('录音已暂停')
    return
  }
  if (activeSegmentChunkCount.value > 0) {
    const durationSeconds = (activeSegmentDurationMs.value / 1000).toFixed(1)
    store.setDraftText(trailingSilenceChunkCount.value > 0
      ? `检测到停顿，等待当前句子结束... ${durationSeconds}s`
      : `正在录入当前句子... ${durationSeconds}s`)
    return
  }
  if (uploadState.value === 'uploading' || uploadQueueSize.value > 0) {
    store.setDraftText(`当前句子已提交识别，等待返回... 队列 ${uploadQueueSize.value}`)
    return
  }
  store.setDraftText('正在监听，等待你说完一句话')
}

function enqueueLeadInChunk(chunk: ArrayBuffer) {
  leadInChunks.push(chunk)
  if (leadInChunks.length > PRE_ROLL_CHUNKS)
    leadInChunks.splice(0, leadInChunks.length - PRE_ROLL_CHUNKS)
}

function updateNoiseFloor(rms: number) {
  const boundedRms = clamp(rms, 0, MAX_SPEECH_RMS_THRESHOLD)
  if (noiseFloorLevel.value <= 0)
    noiseFloorLevel.value = boundedRms
  else
    noiseFloorLevel.value = noiseFloorLevel.value * (1 - NOISE_FLOOR_SMOOTHING) + boundedRms * NOISE_FLOOR_SMOOTHING
}

function isLikelyNoiseSegment() {
  if (activeSpeechChunkCount.value === 0)
    return true
  if (activeSpeechChunkCount.value >= effectiveRealtimeSettings.value.minEffectiveSpeechChunks)
    return false
  return activeSegmentPeakLevel.value < speechThresholdLevel.value * effectiveRealtimeSettings.value.singleChunkPeakMultiplier
}

function resetActiveSegment() {
  activeSegmentChunks = []
  activeSegmentStartedAt = null
  activeSegmentChunkCount.value = 0
  activeSpeechChunkCount.value = 0
  activeSegmentPeakLevel.value = 0
  trailingSilenceChunkCount.value = 0
  activeSegmentDurationMs.value = 0
}

function queueSegmentUpload(chunks: ArrayBuffer[]) {
  if (chunks.length === 0)
    return

  const bytes = chunks.reduce((sum, chunk) => sum + chunk.byteLength, 0)
  const file = createWavFileFromChunks(chunks, `realtime-segment-${Date.now()}.wav`)
  pendingSegmentUploads.push({ file, duration: pcm16BytesToDurationSeconds(bytes) })
  uploadQueueSize.value = pendingSegmentUploads.length
  totalSegmentCount.value += 1
  void flushSegmentUploadQueue()
}

function finalizeActiveSegment(reason: 'silence' | 'limit' | 'stop') {
  if (activeSegmentChunks.length === 0)
    return

  let finalizedChunks = activeSegmentChunks.slice()
  if (reason === 'silence' && trailingSilenceChunkCount.value > 0 && trailingSilenceChunkCount.value < finalizedChunks.length)
    finalizedChunks = finalizedChunks.slice(0, finalizedChunks.length - trailingSilenceChunkCount.value)
  if (finalizedChunks.length === 0)
    finalizedChunks = activeSegmentChunks.slice()

  if (isLikelyNoiseSegment()) {
    resetActiveSegment()
    updateDraftText()
    return
  }

  queueSegmentUpload(finalizedChunks)
  resetActiveSegment()
  updateDraftText()
}

function handleRecorderChunk(chunk: ArrayBuffer) {
  const copiedChunk = cloneChunk(chunk)
  const rms = computePCM16Rms(copiedChunk)
  listeningLevel.value = rms
  const hasSpeech = rms >= speechThresholdLevel.value

  if (!hasSpeech && activeSegmentChunks.length === 0)
    updateNoiseFloor(rms)

  if (hasSpeech) {
    segmentUploadError.value = ''
    if (activeSegmentChunks.length === 0) {
      activeSegmentChunks = [...leadInChunks, copiedChunk]
      leadInChunks.splice(0, leadInChunks.length)
      activeSegmentStartedAt = Date.now() - (activeSegmentChunks.length - 1) * CHUNK_MS
    }
    else {
      activeSegmentChunks.push(copiedChunk)
    }
    activeSegmentChunkCount.value = activeSegmentChunks.length
    activeSpeechChunkCount.value += 1
    activeSegmentPeakLevel.value = Math.max(activeSegmentPeakLevel.value, rms)
    trailingSilenceChunkCount.value = 0
  }
  else if (activeSegmentChunks.length > 0) {
    activeSegmentChunks.push(copiedChunk)
    activeSegmentChunkCount.value = activeSegmentChunks.length
    trailingSilenceChunkCount.value += 1
  }
  else {
    enqueueLeadInChunk(copiedChunk)
  }

  if (activeSegmentStartedAt != null)
    activeSegmentDurationMs.value = Math.max(0, Date.now() - activeSegmentStartedAt)

  if (activeSegmentChunkCount.value >= MAX_SEGMENT_CHUNKS) {
    finalizeActiveSegment('limit')
    return
  }
  if (activeSegmentChunkCount.value > 0 && trailingSilenceChunkCount.value >= effectiveRealtimeSettings.value.endSilenceChunks) {
    finalizeActiveSegment('silence')
    return
  }

  updateDraftText()
}

function flushSegmentUploadQueue() {
  if (segmentUploadPromise)
    return segmentUploadPromise

  segmentUploadPromise = (async () => {
    try {
      while (pendingSegmentUploads.length > 0) {
        const item = pendingSegmentUploads.shift()
        uploadQueueSize.value = pendingSegmentUploads.length
        if (!item)
          continue

        uploadState.value = 'uploading'
        updateDraftText()

        const formData = new FormData()
        formData.append('file', item.file)

        try {
          const result = await transcribeRealtimeSegment(formData)
          const text = normalizeRecognizedText(result.data?.text)
          if (!text) {
            segmentUploadError.value = '某个句子没有返回识别文本，已跳过该片段。'
            continue
          }
          if (!hasRecognizedContent(text)) {
            segmentUploadError.value = '已忽略只包含标点或空白的识别结果。'
            continue
          }

          store.appendSentence(text)
          queueImmediateChunk(text)
          segmentUploadError.value = ''
        }
        catch (error) {
          segmentUploadError.value = extractErrorMessage(error, '当前句子识别失败，已跳过该片段。')
          message.warning(segmentUploadError.value)
        }
        finally {
          uploadState.value = 'idle'
          updateDraftText()
        }
      }
    }
    finally {
      uploadQueueSize.value = pendingSegmentUploads.length
      uploadState.value = 'idle'
      segmentUploadPromise = null
      updateDraftText()
    }
  })()

  return segmentUploadPromise
}

async function loadTaskExecutions(taskId: number) {
  executionLoading.value = true
  try {
    const result = await getTranscriptionTaskExecutions(taskId)
    latestExecutions.value = result.data || []
  }
  catch {
    latestExecutions.value = []
    message.warning('任务已保存，但工作流执行记录拉取失败')
  }
  finally {
    executionLoading.value = false
  }
}

async function loadWorkflowOptions() {
  try {
    await realtimeWorkflowCatalog.loadWorkflows()
  }
  catch {
    message.warning('工作流列表加载失败，可稍后到应用配置页重试')
  }
}

function exportTranscript() {
  if (!effectiveOutputText.value) {
    message.warning('当前没有可导出的输出结果')
    return
  }

  const blob = new Blob([effectiveOutputText.value], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = `realtime-transcription-${new Date().toISOString().replace(/[:.]/g, '-')}.txt`
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  URL.revokeObjectURL(url)
}

async function copyTranscript() {
  if (!effectiveOutputText.value) {
    message.warning('当前没有可复制的输出结果')
    return
  }

  try {
    await navigator.clipboard.writeText(effectiveOutputText.value)
    message.success(hasImmediateWorkflow.value ? '即时处理后的输出已复制' : '原始识别输出已复制')
  }
  catch {
    message.error('复制失败，请检查浏览器剪贴板权限')
  }
}

function currentDurationSeconds() {
  if (!recordingStartedAt.value)
    return 0
  return Math.max(0, Math.round((Date.now() - recordingStartedAt.value) / 1000))
}

async function persistRealtimeSession() {
  const transcript = store.transcriptText.trim()
  if (!transcript)
    return

  savingSession.value = true
  try {
    const result = await createTranscriptionTask({
      type: 'realtime',
      result_text: transcript,
      duration: currentDurationSeconds(),
      workflow_id: sessionWorkflowId.value ?? undefined,
    })
    latestTask.value = result?.data || null
    const taskId = result?.data?.id
    latestExecutions.value = []
    if (taskId && sessionWorkflowId.value)
      await loadTaskExecutions(taskId)
    if (taskId)
      message.success(sessionWorkflowId.value ? `实时转写已保存，并已对整段文本执行复核工作流，任务 #${taskId}` : `实时转写已保存到历史任务 #${taskId}`)
    else
      message.success('实时转写已保存')
  }
  catch {
    latestTask.value = null
    latestExecutions.value = []
    message.warning('实时转写已停止，但保存历史任务失败')
  }
  finally {
    savingSession.value = false
  }
}

async function handleStart() {
  try {
    store.reset()
    latestTask.value = null
    latestExecutions.value = []
    resetActiveSegment()
    leadInChunks.splice(0, leadInChunks.length)
    pendingSegmentUploads.splice(0, pendingSegmentUploads.length)
    pendingInstantChunks.splice(0, pendingInstantChunks.length)
    uploadQueueSize.value = 0
    uploadState.value = 'idle'
    totalSegmentCount.value = 0
    listeningLevel.value = 0
    noiseFloorLevel.value = DEFAULT_NOISE_FLOOR_LEVEL
    instantQueueSize.value = 0
    instantProcessingError.value = ''
    instantNodeResults.value = []
    segmentUploadError.value = ''
    sessionWorkflowId.value = configuredWorkflowMissing.value ? null : (configuredWorkflowId.value ?? null)
    await start(handleRecorderChunk)
    store.isRecording = true
    recordingStartedAt.value = Date.now()
    updateDraftText()
    if (configuredWorkflowMissing.value)
      message.warning(`${configuredWorkflowMessage.value}，本次仅输出原始识别文本。`)
    else if (!configuredWorkflowId.value)
      message.success('录音已启动，系统会按停顿逐句识别并输出原始文本')
    else
      message.success('录音已启动，系统会按停顿逐句识别并即时处理')
  }
  catch (error) {
    stop()
    store.isRecording = false
    recordingStartedAt.value = null
    const detail = error instanceof Error ? error.message : '请检查登录态、浏览器麦克风权限和 asr-api 服务'
    message.error(detail)
  }
}

async function handleStop() {
  if (stoppingSession.value)
    return

  stoppingSession.value = true
  stop()
  try {
    finalizeActiveSegment('stop')
    await flushSegmentUploadQueue()
    await flushImmediateProcessingQueue()
  }
  finally {
    store.isRecording = false
    updateDraftText()
    await persistRealtimeSession()
    recordingStartedAt.value = null
    stoppingSession.value = false
  }
}

function handlePause() {
  pause()
  updateDraftText()
}

function handleResume() {
  resume()
  updateDraftText()
}

function pushMockSentence() {
  const sentence = normalizeRecognizedText('当前是转写演示文本。')
  store.appendSentence(sentence)
  queueImmediateChunk(sentence)
}

onMounted(loadWorkflowOptions)

watch(realtimeSettings, (value) => {
  saveRealtimeRecognitionSettings(value)
}, { deep: true })

watch(realtimeConfigPanelExpanded, (value) => {
  saveRealtimeConfigPanelExpanded(value)
})

onBeforeUnmount(() => {
  if (isRecording.value) {
    stop()
    finalizeActiveSegment('stop')
  }
  store.isRecording = false
  recordingStartedAt.value = null
})
</script>

<template>
  <div class="flex-1 flex flex-col gap-5">
    <section class="card-main p-4 sm:p-5 shrink-0 sticky top-0 z-10 !bg-white/95">
      <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div class="grid grid-cols-2 gap-3 lg:grid-cols-5 xl:flex-1">
          <div class="subtle-panel">
            <div class="text-xs text-slate/70">
              识别通道
            </div>
            <div class="mt-1.5 text-sm font-700 text-ink">
              {{ transportStateLabel }}
            </div>
            <div class="mt-2 text-xs leading-6 text-slate/80">
              {{ transportStateDescription }}
            </div>
          </div>
          <div class="subtle-panel">
            <div class="text-xs text-slate/70">
              录音状态
            </div>
            <div class="mt-1.5 text-sm font-700 text-ink">
              {{ isRecording ? (isPaused ? '暂停中' : '录音中') : '空闲' }}
            </div>
          </div>
          <div class="subtle-panel lg:col-span-2">
            <div class="text-xs text-slate/70">
              本次会话即时工作流
            </div>
            <div class="mt-1.5 text-sm font-700 text-ink">
              {{ isRecording ? sessionWorkflowLabel : effectiveWorkflowLabel }}
            </div>
            <div class="mt-2 text-xs leading-6" :class="hasImmediateWorkflow ? 'text-slate/80' : 'text-amber-700'">
              {{ instantProcessingNotice }}
            </div>
          </div>
          <div class="subtle-panel">
            <div class="text-xs text-slate/70">
              累计片段 / 句子
            </div>
            <div class="mt-1.5 text-sm font-700 text-ink">
              {{ chunkCount }} / {{ finalCount }}
            </div>
          </div>
        </div>

        <div class="flex flex-wrap gap-2 xl:max-w-[520px] xl:justify-end">
          <NButton size="small" quaternary @click="router.push('/workflows/application-settings')">
            应用配置
          </NButton>
          <NButton size="small" type="primary" color="#0f766e" :disabled="isRecording" @click="handleStart">
            开始录音
          </NButton>
          <NButton size="small" :disabled="!isRecording || isPaused" @click="handlePause">
            暂停
          </NButton>
          <NButton size="small" :disabled="!isRecording || !isPaused" @click="handleResume">
            继续
          </NButton>
          <NButton size="small" tertiary :disabled="!isRecording || stoppingSession" :loading="stoppingSession" @click="handleStop">
            停止
          </NButton>
          <NButton size="small" quaternary @click="pushMockSentence">
            模拟片段
          </NButton>
          <NButton size="small" quaternary :disabled="!effectiveOutputText || savingSession" :loading="savingSession" @click="copyTranscript">
            复制输出
          </NButton>
          <NButton size="small" quaternary :disabled="!effectiveOutputText" @click="exportTranscript">
            导出输出
          </NButton>
          <NButton size="small" quaternary @click="router.push('/transcription')">
            去批量转写
          </NButton>
        </div>
      </div>

      <div class="mt-4 rounded-3 border border-white/70 bg-white/50 p-3 shadow-sm backdrop-blur-xl sm:p-4">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0 flex-1">
            <div class="text-sm font-700 text-ink">
              实时整理与识别配置
            </div>
            <div class="mt-1 text-xs leading-6 text-slate/80">
              {{ realtimeConfigSummary }}
            </div>
          </div>
          <NButton size="small" quaternary @click="toggleRealtimeConfigPanel">
            {{ realtimeConfigPanelExpanded ? '收起配置' : '展开配置' }}
          </NButton>
        </div>

        <div v-if="realtimeConfigPanelExpanded" class="mt-4">
          <div v-if="!hasImmediateWorkflow && (isRecording || configuredWorkflowMissing || !configuredWorkflowId)" class="mb-3 rounded-2 border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-6 text-amber-700">
            {{ instantProcessingNotice }}
          </div>
          <WorkflowSelectionPreview
            :workflow="selectedWorkflowOption"
            :loading="realtimeWorkflowCatalog.loading.value"
            empty-title="未配置实时应用工作流"
            empty-description="前往应用配置页设置后，前端按停顿切出的每个句子都会即时执行这里展示的默认节点链路；停止录音时系统还会再做一次整段复核。"
          />

          <div class="mt-4 grid gap-3 xl:grid-cols-[1.4fr_0.8fr]">
            <div class="subtle-panel m-0">
              <div class="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <div class="text-sm font-600 text-ink">
                    实时分句参数
                  </div>
                  <div class="mt-1 text-xs leading-6 text-slate/80">
                    1 个块约等于 300ms。这里的参数会保存在当前浏览器，只影响本页实时转写。
                  </div>
                </div>
                <div class="flex flex-wrap gap-2">
                  <NButton size="tiny" quaternary @click="recalibrateNoiseFloor">
                    重置底噪
                  </NButton>
                  <NButton size="tiny" quaternary @click="resetRealtimeRecognitionSettings">
                    恢复默认
                  </NButton>
                </div>
              </div>

              <div class="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                <div>
                  <div class="text-xs text-slate/70">
                    最小触发阈值
                  </div>
                  <NInputNumber v-model:value="realtimeSettings.minSpeechThreshold" size="small" class="mt-2 w-full" :min="0.005" :max="0.08" :step="0.001" />
                </div>
                <div>
                  <div class="text-xs text-slate/70">
                    底噪倍率
                  </div>
                  <NInputNumber v-model:value="realtimeSettings.noiseGateMultiplier" size="small" class="mt-2 w-full" :min="1.2" :max="6" :step="0.1" />
                </div>
                <div>
                  <div class="text-xs text-slate/70">
                    句尾静音块数
                  </div>
                  <NInputNumber v-model:value="realtimeSettings.endSilenceChunks" size="small" class="mt-2 w-full" :min="2" :max="10" :step="1" />
                </div>
                <div>
                  <div class="text-xs text-slate/70">
                    最少有效语音块数
                  </div>
                  <NInputNumber v-model:value="realtimeSettings.minEffectiveSpeechChunks" size="small" class="mt-2 w-full" :min="1" :max="6" :step="1" />
                </div>
              </div>

              <div class="mt-3 grid gap-3 md:grid-cols-2">
                <div>
                  <div class="text-xs text-slate/70">
                    单块峰值倍率
                  </div>
                  <NInputNumber v-model:value="realtimeSettings.singleChunkPeakMultiplier" size="small" class="mt-2 w-full" :min="1" :max="3" :step="0.05" />
                </div>
                <div class="rounded-2 bg-white/70 px-3 py-2 text-xs leading-6 text-slate/80">
                  当前生效阈值 {{ speechThresholdLevel.toFixed(3) }}，当前底噪 {{ noiseFloorLevel.toFixed(3) }}。如果环境噪音大，优先提高“最小触发阈值”或“底噪倍率”；如果句子收尾太慢，再降低“句尾静音块数”。
                </div>
              </div>
            </div>

            <div class="subtle-panel m-0">
              <div class="flex items-center justify-between gap-3">
                <div>
                  <div class="text-sm font-600 text-ink">
                    标点保留
                  </div>
                  <div class="mt-1 text-xs leading-6 text-slate/80">
                    默认关闭。关闭后，实时识别结果、即时工作流输出和最终保存文本中的标点都会被过滤。
                  </div>
                </div>
                <NSwitch v-model:value="realtimeSettings.keepPunctuation" />
              </div>
              <div class="mt-3 rounded-2 bg-white/70 px-3 py-2 text-xs leading-6" :class="realtimeSettings.keepPunctuation ? 'text-emerald-700' : 'text-slate/80'">
                当前模式：{{ realtimeSettings.keepPunctuation ? '保留标点' : '过滤全部标点' }}
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <section class="flex-1 min-h-[500px] grid grid-cols-1 gap-5 xl:grid-cols-[1.2fr_0.8fr]">
      <NCard class="card-main flex flex-col min-h-0" content-style="display: flex; flex-direction: column; min-height: 0; padding: 0 20px 20px;">
        <template #header>
          <span class="text-sm font-600">{{ immediateOutputTitle }}</span>
        </template>
        <div class="flex-1 min-h-0 overflow-y-auto rounded-2.5 bg-[#fbfdff] p-4">
          <div class="mb-3 text-xs leading-6 text-slate/80">
            {{ immediateOutputDescription }}
          </div>
          <div v-if="store.processedSentences.length === 0" class="text-slate">
            开始录音后，这里会在每句说完并识别成功后，逐句显示可直接用于写报告的输出结果。
          </div>
          <div v-for="(line, index) in store.processedSentences" :key="`${index}-${line}`" class="mb-2.5 rounded-2.5 bg-mist/60 px-4 py-3 text-sm leading-6 text-ink last:mb-0">
            {{ line }}
          </div>
        </div>
      </NCard>

      <NCard class="card-main flex flex-col min-h-0" content-style="display: flex; flex-direction: column; min-height: 0; padding: 0 20px 20px;">
        <template #header>
          <span class="text-sm font-600">实时处理状态</span>
        </template>
        <div class="flex-1 min-h-0 flex flex-col overflow-hidden gap-4">
          <div class="rounded-2 px-3 py-2 text-xs leading-6" :class="hasImmediateWorkflow ? 'border border-emerald-200 bg-emerald-50 text-emerald-700' : 'border border-amber-200 bg-amber-50 text-amber-700'">
            {{ instantProcessingNotice }}
          </div>
          <div class="subtle-panel m-0">
            <div class="text-sm font-600 text-ink">
              本次会话工作流
            </div>
            <div class="mt-1 text-sm text-slate">
              {{ isRecording ? sessionWorkflowLabel : effectiveWorkflowLabel }}
            </div>
            <div class="mt-1 text-xs text-slate/80">
              {{ hasImmediateWorkflow ? '当前会话每个停顿切出的句子都会立即走这条链路。' : '当前会话没有可用实时工作流，只能先输出原始识别结果。' }}
            </div>
          </div>
          <div class="subtle-panel m-0">
            <div class="text-sm font-600 text-ink">
              识别与处理队列
            </div>
            <div class="mt-1 text-sm text-slate">
              {{ instantProcessing ? '处理中' : '空闲' }}
            </div>
            <div class="mt-1 text-xs text-slate/80">
              待识别句子：{{ uploadQueueSize }} · 待处理文本：{{ instantQueueSize }}
            </div>
            <div v-if="segmentUploadError" class="mt-2 text-xs leading-6 text-amber-700">
              {{ segmentUploadError }}
            </div>
            <div v-if="instantProcessingError" class="mt-2 text-xs leading-6 text-amber-700">
              {{ instantProcessingError }}
            </div>
          </div>
          <div class="subtle-panel m-0">
            <div class="text-sm font-600 text-ink">
              当前收句状态
            </div>
            <div class="mt-1 text-sm text-slate whitespace-pre-wrap">
              {{ store.draftText || '正在等待一句完整结束' }}
            </div>
          </div>
          <div v-if="instantNodeResults.length > 0" class="subtle-panel m-0">
            <div class="text-sm font-600 text-ink">
              最近一次即时处理节点明细
            </div>
            <div class="mt-3 grid gap-3">
              <div v-for="node in instantNodeResults" :key="node.id" class="rounded-2 bg-white/80 p-3">
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <div class="text-sm font-700 text-ink">
                    {{ node.position }}. {{ node.label || node.node_type }}
                  </div>
                  <div class="text-xs text-slate">
                    {{ formatExecutionStatus(node.status) }} · {{ node.duration_ms || 0 }} ms
                  </div>
                </div>
                <div class="mt-3 grid gap-3 lg:grid-cols-2">
                  <TextDiffPreview :before-text="sanitizeText(node.input_text)" :after-text="sanitizeText(node.output_text)" />
                </div>
                <div class="mt-3">
                  <NodeDetailPanel :detail="node.detail" empty-label="当前节点没有 detail 信息。" />
                </div>
              </div>
            </div>
          </div>
          <div class="subtle-panel flex-1 flex flex-col min-h-[110px] m-0">
            <div class="text-sm font-600 text-ink shrink-0">
              即时输出全文
            </div>
            <div class="mt-2 flex-1 min-h-0 overflow-y-auto whitespace-pre-wrap text-sm leading-6 text-slate">
              {{ effectiveOutputText || '处理后的即时输出会在这里持续累积，可直接复制到剪贴板，后续由专门客户端粘贴到 PACS 当前光标处。' }}
            </div>
          </div>
          <div class="subtle-panel flex-1 flex flex-col min-h-0 m-0">
            <div class="text-sm font-600 text-ink shrink-0">
              原始识别全文
            </div>
            <div class="mt-2 flex-1 min-h-0 overflow-y-auto whitespace-pre-wrap text-sm leading-6 text-slate">
              {{ store.transcriptText || '原始识别文本会在这里按句累积，停止录音后系统会用它做整段复核并保存历史任务。' }}
            </div>
          </div>
        </div>
      </NCard>
    </section>

    <section v-if="latestTask" class="card-main p-4 sm:p-5">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div class="text-sm font-700 text-ink">
            停止后整段复核结果
          </div>
          <div class="mt-1 text-xs text-slate">
            {{ latestTask.id ? `任务 #${latestTask.id}` : '' }}
            <template v-if="latestExecution?.created_at">
              · {{ formatDateTime(latestExecution.created_at) }}
            </template>
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <NButton v-if="latestTask.meeting_id" size="small" quaternary @click="router.push(`/meetings/${latestTask.meeting_id}`)">
            查看会议纪要
          </NButton>
          <NButton v-if="latestTask.id" size="small" quaternary @click="router.push({ path: '/transcription', query: { taskId: String(latestTask.id) } })">
            去历史页查看
          </NButton>
        </div>
      </div>

      <div class="mt-4 grid gap-4 xl:grid-cols-[0.9fr_1.1fr]">
        <div class="grid gap-4">
          <div v-if="latestTaskWorkflowPendingUpgradeMessage" class="rounded-2 border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-6 text-amber-700">
            {{ latestTaskWorkflowPendingUpgradeMessage }}
          </div>
          <div class="grid gap-4 sm:grid-cols-3">
            <div class="subtle-panel m-0">
              <div class="text-xs text-slate/70">
                保存状态
              </div>
              <div class="mt-2 flex items-center gap-2">
                <NTag size="small" round :bordered="false" :type="latestSaveSummary.type as any">
                  {{ latestSaveSummary.label }}
                </NTag>
              </div>
              <div class="mt-2 text-xs leading-6 text-slate">
                {{ latestSaveSummary.detail }}
              </div>
            </div>
            <div class="subtle-panel m-0">
              <div class="text-xs text-slate/70">
                整段复核执行
              </div>
              <div class="mt-2 flex items-center gap-2">
                <NTag size="small" round :bordered="false" :type="latestExecutionSummary.type as any">
                  {{ latestExecutionSummary.label }}
                </NTag>
              </div>
              <div class="mt-2 text-xs leading-6 text-slate">
                {{ latestExecutionSummary.detail }}
              </div>
            </div>
            <div class="subtle-panel m-0">
              <div class="text-xs text-slate/70">
                复核工作流
              </div>
              <div class="mt-2 text-sm font-700 text-ink">
                {{ workflowLabel(latestTask?.workflow_id) }}
              </div>
              <div class="mt-2 text-xs leading-6 text-slate">
                {{ latestTask?.meeting_id ? `已生成会议数据 #${latestTask.meeting_id}` : '只有包含会议纪要节点的工作流才会继续生成会议数据。' }}
              </div>
            </div>
          </div>

          <div class="subtle-panel m-0">
            <div class="text-xs text-slate/70">
              整段复核最终文本
            </div>
            <div class="mt-2 max-h-64 overflow-auto whitespace-pre-wrap text-sm leading-6 text-ink">
              {{ sanitizeText(latestExecution?.final_text || latestTask.result_text) || '停止录音并完成整段工作流复核后，最终文本会显示在这里。' }}
            </div>
          </div>
          <div v-if="latestTask.post_process_error" class="subtle-panel m-0">
            <div class="text-xs text-red-600/80">
              后处理错误
            </div>
            <div class="mt-2 whitespace-pre-wrap text-sm leading-6 text-red-600">
              {{ latestTask.post_process_error }}
            </div>
          </div>
        </div>

        <div class="subtle-panel m-0 min-h-[240px]">
          <div class="flex items-center justify-between gap-2">
            <div class="text-sm font-600 text-ink">
              整段复核节点明细
            </div>
            <div class="text-xs text-slate">
              {{ executionLoading ? '加载中' : (latestExecution?.node_results?.length || 0) }} 步
            </div>
          </div>
          <div v-if="executionLoading" class="mt-4 text-sm text-slate">
            正在加载执行记录...
          </div>
          <div v-else-if="!latestExecution" class="mt-4 text-sm text-slate">
            当前还没有工作流执行记录；如果未绑定工作流，则不会生成执行记录。
          </div>
          <div v-else class="mt-4 grid gap-3">
            <div v-for="node in latestExecution.node_results || []" :key="node.id" class="rounded-2 bg-white/80 p-3">
              <div class="flex flex-wrap items-center justify-between gap-2">
                <div class="text-sm font-700 text-ink">
                  {{ node.position }}. {{ node.label || node.node_type }}
                </div>
                <div class="text-xs text-slate">
                  {{ formatExecutionStatus(node.status) }} · {{ node.duration_ms || 0 }} ms
                </div>
              </div>
              <div class="mt-3 grid gap-3 lg:grid-cols-2">
                <TextDiffPreview :before-text="sanitizeText(node.input_text)" :after-text="sanitizeText(node.output_text)" />
              </div>
              <div class="mt-3">
                <NodeDetailPanel :detail="node.detail" empty-label="当前节点没有 detail 信息。" />
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
