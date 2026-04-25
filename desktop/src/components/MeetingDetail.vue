<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { invoke } from '@tauri-apps/api/core'
import { marked } from 'marked'

import { useConfirm } from '@/composables/useConfirm'
import { useAppStore } from '@/stores/app'
import { getMeetingDetail, regenerateMeetingSummary, updateMeeting, type MeetingDetailResponse } from '@/utils/meetings'
import { debugLog } from '@/utils/debug'

const props = defineProps<{ meetingId: number }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'deleted'): void; (e: 'saved'): void }>()

type SummaryMode = 'preview' | 'edit' | 'export'
type ExportFontSize = 'compact' | 'normal' | 'large'
type ExportAccent = 'teal' | 'blue' | 'ink'

interface ExportOptions {
  includeTitle: boolean
  includeMeta: boolean
  includeTranscript: boolean
  fontSize: ExportFontSize
  accent: ExportAccent
}

const MEETING_META_FIELDS = [
  { key: 'topic', label: '会议主题', placeholder: '[填写会议主题]' },
  { key: 'time', label: '会议时间', placeholder: '[填写具体日期]' },
  { key: 'location', label: '会议地点', placeholder: '[填写具体会议室/线上平台]' },
  { key: 'host', label: '主持人', placeholder: '[填写姓名]' },
  { key: 'recorder', label: '记录人', placeholder: '[填写记录人]' },
] as const

type MeetingMetaKey = typeof MEETING_META_FIELDS[number]['key']
type MeetingMeta = Record<MeetingMetaKey, string>

const EXPORT_OPTIONS_STORAGE_KEY = 'asr-desktop-meeting-export-options'
const EXPORT_FONT_SIZES: ExportFontSize[] = ['compact', 'normal', 'large']
const EXPORT_ACCENTS: ExportAccent[] = ['teal', 'blue', 'ink']
const DEFAULT_EXPORT_OPTIONS: ExportOptions = {
  includeTitle: true,
  includeMeta: true,
  includeTranscript: false,
  fontSize: 'normal',
  accent: 'teal',
}
const EXPORT_ACCENT_CONFIG: Record<ExportAccent, { label: string, color: string, soft: string }> = {
  teal: { label: '青绿', color: '#0f766e', soft: 'rgba(15, 118, 110, 0.1)' },
  blue: { label: '蓝色', color: '#1d4ed8', soft: 'rgba(29, 78, 216, 0.1)' },
  ink: { label: '墨色', color: '#111827', soft: 'rgba(17, 24, 39, 0.08)' },
}
const EXPORT_FONT_CONFIG: Record<ExportFontSize, { label: string, size: number, lineHeight: number }> = {
  compact: { label: '紧凑', size: 12.5, lineHeight: 1.62 },
  normal: { label: '标准', size: 13.5, lineHeight: 1.75 },
  large: { label: '宽松', size: 15, lineHeight: 1.86 },
}

const appStore = useAppStore()
const detail = ref<MeetingDetailResponse | null>(null)
const loading = ref(false)
const errorText = ref('')
const saveNotice = ref('')
const saving = ref(false)
const regenerating = ref(false)
const exporting = ref(false)
const activeTab = ref<'summary' | 'transcript'>('summary')
const summaryMode = ref<SummaryMode>('preview')
const draftTitle = ref('')
const draftContent = ref('')
const draftMeta = reactive<MeetingMeta>(createEmptyMeta())
const exportOptions = reactive<ExportOptions>(loadExportOptions())

const { confirm } = useConfirm()
let pollTimer: ReturnType<typeof setInterval> | null = null

marked.setOptions({ gfm: true, breaks: true })

const sourceTitle = computed(() => detail.value?.title || '')
const sourceContent = computed(() => detail.value?.summary?.content || '')
const exportAccent = computed(() => EXPORT_ACCENT_CONFIG[exportOptions.accent])
const exportFont = computed(() => EXPORT_FONT_CONFIG[exportOptions.fontSize])

const hasUnsavedChanges = computed(() => {
  if (!detail.value)
    return false
  return draftTitle.value.trim() !== sourceTitle.value.trim() || draftContent.value.trim() !== sourceContent.value.trim()
})

const currentSummaryContent = computed(() => {
  if (summaryMode.value === 'edit' || hasUnsavedChanges.value)
    return draftContent.value
  return sourceContent.value
})

const currentTitle = computed(() => {
  const title = (summaryMode.value === 'edit' || hasUnsavedChanges.value) ? draftTitle.value : sourceTitle.value
  return title.trim() || `会议纪要 #${detail.value?.id || props.meetingId}`
})

const renderedHtml = computed(() => renderMarkdown(currentSummaryContent.value))

const summaryReady = computed(() => Boolean(currentSummaryContent.value.trim()))
const canSave = computed(() => Boolean(detail.value && draftTitle.value.trim() && !saving.value))
const canExport = computed(() => Boolean(summaryReady.value && currentTitle.value.trim() && !exporting.value))

const statusLabel = computed(() => {
  switch (detail.value?.status) {
    case 'completed':
      return '已完成'
    case 'processing':
      return '生成中'
    case 'uploaded':
      return '排队中'
    case 'failed':
      return '失败'
    default:
      return '未知'
  }
})

const isProcessing = computed(() => detail.value?.status === 'uploaded' || detail.value?.status === 'processing')
const showSummaryToolbar = computed(() => activeTab.value === 'summary' && Boolean(detail.value) && !isProcessing.value)
const showSummaryFooter = computed(() => showSummaryToolbar.value && (summaryMode.value === 'edit' || summaryMode.value === 'export'))
const exportPreviewBodyMarkdown = computed(() => {
  const body = resolveExportBodyMarkdown(currentSummaryContent.value, currentTitle.value, exportOptions)
  return body || '_暂无正文内容_'
})
const exportPreviewHtml = computed(() => renderMarkdown(exportPreviewBodyMarkdown.value))
const exportPreviewTranscriptItems = computed(() => detail.value?.transcripts.slice(0, 2) || [])
const exportPreviewStyle = computed(() => ({
  '--preview-accent': exportAccent.value.color,
  '--preview-soft': exportAccent.value.soft,
  '--preview-font-size': `${exportFont.value.size}px`,
  '--preview-line-height': String(exportFont.value.lineHeight),
}))

function createEmptyMeta(): MeetingMeta {
  return {
    topic: '',
    time: '',
    location: '',
    host: '',
    recorder: '',
  }
}

function escapeRegExp(text: string) {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function escapeHtml(text: string) {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function renderMarkdown(content: string) {
  const text = content.trim()
  if (!text)
    return ''
  try {
    return marked.parse(text) as string
  }
  catch {
    return `<pre>${escapeHtml(text)}</pre>`
  }
}

function normalizeExportOptions(raw?: Partial<ExportOptions> | null): ExportOptions {
  return {
    includeTitle: raw?.includeTitle !== false,
    includeMeta: raw?.includeMeta !== false,
    includeTranscript: raw?.includeTranscript === true,
    fontSize: EXPORT_FONT_SIZES.includes(raw?.fontSize as ExportFontSize) ? raw?.fontSize as ExportFontSize : DEFAULT_EXPORT_OPTIONS.fontSize,
    accent: EXPORT_ACCENTS.includes(raw?.accent as ExportAccent) ? raw?.accent as ExportAccent : DEFAULT_EXPORT_OPTIONS.accent,
  }
}

function loadExportOptions(): ExportOptions {
  if (typeof localStorage === 'undefined')
    return { ...DEFAULT_EXPORT_OPTIONS }
  try {
    const raw = localStorage.getItem(EXPORT_OPTIONS_STORAGE_KEY)
    return normalizeExportOptions(raw ? JSON.parse(raw) as Partial<ExportOptions> : null)
  }
  catch {
    return { ...DEFAULT_EXPORT_OPTIONS }
  }
}

function persistExportOptions() {
  if (typeof localStorage === 'undefined')
    return
  localStorage.setItem(EXPORT_OPTIONS_STORAGE_KEY, JSON.stringify(exportOptions))
}

function normalizeMetaLineForMatch(line: string) {
  return line
    .trim()
    .replace(/^[-*]\s*/, '')
    .replace(/[*_]/g, '')
}

function metaLinePattern(field: typeof MEETING_META_FIELDS[number]) {
  return new RegExp(`^${escapeRegExp(field.label)}\\s*[：:]\\s*(.*)$`)
}

function isMetaLine(line: string, field: typeof MEETING_META_FIELDS[number]) {
  return metaLinePattern(field).test(normalizeMetaLineForMatch(line))
}

function stripMetaMarkdown(value: string) {
  return value
    .trim()
    .replace(/^(?:[*_]{1,2})+\s*/, '')
    .replace(/\s*(?:[*_]{1,2})+$/, '')
    .trim()
}

function normalizeComparableText(text: string) {
  return stripMetaMarkdown(text.replace(/^#+\s*/, ''))
    .replace(/\s+/g, ' ')
    .trim()
}

function isTitleLine(line: string, title: string) {
  const normalizedTitle = normalizeComparableText(title)
  if (!normalizedTitle)
    return false
  return normalizeComparableText(line) === normalizedTitle
}

function resolveExportBodyMarkdown(content: string, title: string, options: Pick<ExportOptions, 'includeTitle' | 'includeMeta'>) {
  if (!content.trim())
    return ''
  const lines = content.split(/\r?\n/)
  const removeIndexes = new Set<number>()
  let cursor = 0
  while (cursor < lines.length && lines[cursor].trim() === '') {
    removeIndexes.add(cursor)
    cursor++
  }

  const titleIndex = cursor < lines.length && isTitleLine(lines[cursor], title) ? cursor : -1
  if (titleIndex >= 0 && !options.includeTitle)
    removeIndexes.add(titleIndex)

  let scanIndex = titleIndex >= 0 ? titleIndex + 1 : cursor
  if (!options.includeMeta) {
    while (scanIndex < lines.length) {
      const line = lines[scanIndex]
      if (line.trim() === '') {
        removeIndexes.add(scanIndex)
        scanIndex++
        continue
      }
      if (MEETING_META_FIELDS.some(field => isMetaLine(line, field))) {
        removeIndexes.add(scanIndex)
        scanIndex++
        continue
      }
      break
    }
  }

  return lines
    .filter((_, index) => !removeIndexes.has(index))
    .join('\n')
    .trim()
}

function normalizeMetaValue(value: string, placeholder: string) {
  const trimmed = stripMetaMarkdown(value)
  if (!trimmed || trimmed === placeholder || /^\[.+\]$/.test(trimmed))
    return ''
  return trimmed
}

function buildPrintableMarkdownHtml(markdown: string) {
  return renderMarkdown(markdown)
    .replace(/<h1>/g, `<h1 style="font-size: ${exportFont.value.size + 8}px; margin: 0 0 12px; font-weight: 800; color: ${exportAccent.value.color}; line-height: 1.35;">`)
    .replace(/<h2>/g, `<h2 style="font-size: ${exportFont.value.size + 3}px; margin: 18px 0 8px; font-weight: 700; color: ${exportAccent.value.color}; line-height: 1.45;">`)
    .replace(/<h3>/g, `<h3 style="font-size: ${exportFont.value.size + 1}px; margin: 12px 0 6px; font-weight: 700; color: #0f172a; line-height: 1.5;">`)
}

function extractMeetingMeta(content: string): MeetingMeta {
  const meta = createEmptyMeta()
  const lines = content.split(/\r?\n/)
  for (const line of lines) {
    const normalizedLine = normalizeMetaLineForMatch(line)
    for (const field of MEETING_META_FIELDS) {
      const match = normalizedLine.match(metaLinePattern(field))
      if (match)
        meta[field.key] = normalizeMetaValue(match[1] || '', field.placeholder)
    }
  }
  return meta
}

function resetDraftFromDetail() {
  draftTitle.value = detail.value?.title || ''
  draftContent.value = detail.value?.summary?.content || ''
  Object.assign(draftMeta, extractMeetingMeta(draftContent.value))
}

function findMetaInsertIndex(lines: string[]) {
  let lastMetaIndex = -1
  for (const field of MEETING_META_FIELDS) {
    const index = lines.findIndex(line => isMetaLine(line, field))
    if (index > lastMetaIndex)
      lastMetaIndex = index
  }
  if (lastMetaIndex >= 0)
    return lastMetaIndex + 1

  if (lines.length && /^#\s+/.test(lines[0].trim())) {
    let index = 1
    while (index < lines.length && lines[index].trim() === '')
      index++
    return index
  }
  return 0
}

function setMarkdownField(content: string, field: typeof MEETING_META_FIELDS[number], rawValue: string) {
  const value = rawValue.trim() || field.placeholder
  const lines = content.split(/\r?\n/)
  const existingIndex = lines.findIndex(line => isMetaLine(line, field))
  if (existingIndex >= 0) {
    lines[existingIndex] = `${field.label}： ${value}`
    return lines.join('\n')
  }
  if (!rawValue.trim())
    return content
  const insertIndex = findMetaInsertIndex(lines)
  lines.splice(insertIndex, 0, `${field.label}： ${value}`)
  return lines.join('\n')
}

function syncMetaFieldToDraft(key: MeetingMetaKey) {
  const field = MEETING_META_FIELDS.find(item => item.key === key)
  if (!field)
    return
  draftContent.value = setMarkdownField(draftContent.value, field, draftMeta[key])
}

function syncMetaFieldsToDraft() {
  for (const field of MEETING_META_FIELDS)
    draftContent.value = setMarkdownField(draftContent.value, field, draftMeta[field.key])
}

function applyMeetingInfoToDraft() {
  if (!detail.value)
    return
  if (!draftMeta.topic.trim())
    draftMeta.topic = draftTitle.value.trim() || detail.value.title
  if (!draftMeta.time.trim())
    draftMeta.time = formatDateTime(detail.value.created_at)
  if (!draftMeta.recorder.trim())
    draftMeta.recorder = appStore.displayName || appStore.username || ''
  syncMetaFieldsToDraft()
}

function formatDateTime(value?: string) {
  if (!value)
    return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime()))
    return value
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(date)
}

async function load(opts?: { silent?: boolean, resetDraft?: boolean }) {
  if (!opts?.silent)
    loading.value = true
  errorText.value = ''
  try {
    const shouldResetDraft = opts?.resetDraft || !hasUnsavedChanges.value
    const result = await getMeetingDetail(props.meetingId)
    detail.value = result
    if (shouldResetDraft)
      resetDraftFromDetail()
  }
  catch (error) {
    errorText.value = error instanceof Error ? error.message : '加载会议详情失败'
    void debugLog('meetings.detail', 'failed to load detail', error instanceof Error ? {
      meetingId: props.meetingId,
      message: error.message,
      stack: error.stack,
    } : { meetingId: props.meetingId, error })
  }
  finally {
    loading.value = false
  }
}

function setSummaryMode(mode: SummaryMode) {
  if (!detail.value)
    return
  if (mode === 'edit' && !draftTitle.value && !draftContent.value)
    resetDraftFromDetail()
  summaryMode.value = mode
  saveNotice.value = ''
}

async function requestClose() {
  if (hasUnsavedChanges.value) {
    const ok = await confirm({
      title: '放弃编辑',
      message: '当前会议纪要还有未保存修改，确定关闭吗？',
      confirmText: '关闭',
      tone: 'danger',
    })
    if (!ok)
      return
  }
  emit('close')
}

async function cancelEdit() {
  if (hasUnsavedChanges.value) {
    const ok = await confirm({
      title: '放弃编辑',
      message: '确定恢复到最近一次保存的会议纪要吗？',
      confirmText: '恢复',
      tone: 'danger',
    })
    if (!ok)
      return
  }
  resetDraftFromDetail()
  summaryMode.value = 'preview'
  saveNotice.value = ''
}

async function saveEdits() {
  if (!detail.value || !canSave.value)
    return
  syncMetaFieldsToDraft()
  saving.value = true
  errorText.value = ''
  saveNotice.value = ''
  try {
    const result = await updateMeeting(detail.value.id, {
      title: draftTitle.value.trim(),
      summary_content: draftContent.value,
    })
    detail.value = result
    resetDraftFromDetail()
    summaryMode.value = 'preview'
    saveNotice.value = '已保存'
    emit('saved')
  }
  catch (error) {
    errorText.value = error instanceof Error ? error.message : '保存会议纪要失败'
  }
  finally {
    saving.value = false
  }
}

async function handleRegenerate() {
  if (!detail.value)
    return
  const ok = await confirm({
    title: '重新生成纪要',
    message: hasUnsavedChanges.value ? '重新生成会覆盖当前未保存编辑，确定继续吗？' : '确定使用当前绑定的会议工作流重新生成本次纪要吗？',
    confirmText: '重新生成',
  })
  if (!ok)
    return
  regenerating.value = true
  try {
    await regenerateMeetingSummary(detail.value.id, detail.value.workflow_id ?? null)
    await load({ silent: true, resetDraft: true })
    summaryMode.value = 'preview'
  }
  catch (error) {
    errorText.value = error instanceof Error ? error.message : '重新生成失败'
  }
  finally {
    regenerating.value = false
  }
}

function buildTranscriptHtml() {
  if (!exportOptions.includeTranscript || !detail.value?.transcripts.length)
    return ''
  const items = detail.value.transcripts.map((item) => {
    const speaker = item.speaker_label || '说话人'
    return `
      <div style="break-inside: avoid; margin: 0 0 10px; padding: 8px 10px; border-left: 3px solid ${exportAccent.value.color}; background: rgba(248,250,252,0.92);">
        <div style="font-size: 11px; color: #64748b; margin-bottom: 4px;">${escapeHtml(speaker)} · ${item.start_time.toFixed(1)}s</div>
        <div style="white-space: pre-wrap; color: #1e293b;">${escapeHtml(item.text)}</div>
      </div>
    `
  }).join('')
  return `
    <section style="margin-top: 26px; padding-top: 16px; border-top: 1px solid rgba(148,163,184,0.28);">
      <h2 style="font-size: 16px; color: ${exportAccent.value.color}; margin: 0 0 12px;">逐字稿</h2>
      ${items}
    </section>
  `
}

function buildPrintableHtml(title: string) {
  const bodyMarkdown = resolveExportBodyMarkdown(currentSummaryContent.value, title, exportOptions)
  return `
    <div class="pdf-root" style="font-family: -apple-system, BlinkMacSystemFont, 'PingFang SC', 'Microsoft YaHei', 'Hiragino Sans GB', 'Helvetica Neue', Arial, sans-serif; color: #0f172a; padding: 32px 40px; background: #ffffff; width: 760px; box-sizing: border-box;">
      <div class="pdf-content" style="font-size: ${exportFont.value.size}px; line-height: ${exportFont.value.lineHeight}; color: #1e293b;">${buildPrintableMarkdownHtml(bodyMarkdown)}</div>
      ${buildTranscriptHtml()}
    </div>
  `
}

async function blobToBase64(blob: Blob) {
  const dataUrl = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      if (typeof reader.result === 'string') {
        resolve(reader.result)
        return
      }
      reject(new Error('导出 PDF 失败：无法读取生成结果'))
    }
    reader.onerror = () => reject(reader.error || new Error('导出 PDF 失败：无法读取生成结果'))
    reader.readAsDataURL(blob)
  })
  const encoded = dataUrl.split(',', 2)[1]
  if (!encoded)
    throw new Error('导出 PDF 失败：生成文件内容为空')
  return encoded
}

async function exportToPdf() {
  if (!canExport.value)
    return
  syncMetaFieldsToDraft()
  exporting.value = true
  errorText.value = ''
  const host = document.createElement('div')
  host.style.position = 'fixed'
  host.style.left = '-99999px'
  host.style.top = '0'
  host.style.zIndex = '-1'
  host.style.pointerEvents = 'none'
  const title = currentTitle.value
  host.innerHTML = buildPrintableHtml(title)
  document.body.appendChild(host)
  try {
    const filename = `${title.replace(/[\\/:*?"<>|]+/g, '_')}.pdf`
    const opts: any = {
      margin: [10, 10, 12, 10],
      filename,
      image: { type: 'jpeg', quality: 0.96 },
      html2canvas: {
        scale: 2,
        useCORS: true,
        backgroundColor: '#ffffff',
        windowWidth: 800,
      },
      jsPDF: { unit: 'mm', format: 'a4', orientation: 'portrait' },
      pagebreak: { mode: ['avoid-all', 'css', 'legacy'] },
    }
    const html2pdfMod = await import('html2pdf.js')
    const html2pdf = (html2pdfMod as any).default || (html2pdfMod as any)
    const pdfBlob = await html2pdf()
      .from(host.firstElementChild as HTMLElement)
      .set(opts)
      .outputPdf('blob')
    const pdfBase64 = await blobToBase64(pdfBlob as Blob)
    await invoke<boolean>('save_pdf_file', { suggestedName: filename, pdfBase64 })
  }
  catch (error) {
    errorText.value = error instanceof Error ? error.message : '导出 PDF 失败'
    void debugLog('meetings.detail', 'pdf export failed', error instanceof Error ? { message: error.message, stack: error.stack } : { error })
  }
  finally {
    document.body.removeChild(host)
    exporting.value = false
  }
}

function startPolling() {
  if (pollTimer)
    return
  pollTimer = setInterval(() => {
    if (isProcessing.value && !hasUnsavedChanges.value)
      void load({ silent: true })
  }, 4000)
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

watch(() => props.meetingId, () => {
  detail.value = null
  draftTitle.value = ''
  draftContent.value = ''
  Object.assign(draftMeta, createEmptyMeta())
  summaryMode.value = 'preview'
  void load({ resetDraft: true })
}, { immediate: false })

watch(exportOptions, persistExportOptions, { deep: true })

onMounted(() => {
  void load({ resetDraft: true })
  startPolling()
})

onBeforeUnmount(() => {
  stopPolling()
})
</script>

<template>
  <Transition name="detail-slide">
    <div class="meeting-detail" @click.self="requestClose">
      <div class="detail-panel">
        <header class="detail-head">
          <button class="detail-back" type="button" @click="requestClose">
            <span class="back-arrow">‹</span> 返回
          </button>
          <div class="detail-title">{{ currentTitle || '加载中...' }}</div>
          <span class="status-pill" :class="detail?.status === 'completed' ? 'success' : (detail?.status === 'failed' ? 'danger' : 'pending')">
            <span v-if="isProcessing" class="pulse-dot" />
            {{ statusLabel }}
          </span>
        </header>

        <div class="detail-tabs">
          <button class="detail-tab" :class="{ active: activeTab === 'summary' }" @click="activeTab = 'summary'">
            会议纪要
          </button>
          <button class="detail-tab" :class="{ active: activeTab === 'transcript' }" @click="activeTab = 'transcript'">
            逐字稿
          </button>
          <div class="detail-actions">
            <button class="action-btn" :disabled="!canExport" @click="exportToPdf">
              {{ exporting ? '准备中...' : '导出 PDF' }}
            </button>
            <button class="action-btn" :disabled="regenerating || !detail" @click="handleRegenerate">
              {{ regenerating ? '生成中...' : '重新生成' }}
            </button>
          </div>
        </div>

        <p v-if="errorText" class="detail-error">{{ errorText }}</p>
        <p v-else-if="saveNotice" class="detail-success">{{ saveNotice }}</p>

        <div v-if="showSummaryToolbar" class="summary-toolbar">
          <button class="mode-btn" :class="{ active: summaryMode === 'preview' }" type="button" @click="setSummaryMode('preview')">
            预览
          </button>
          <button class="mode-btn" :class="{ active: summaryMode === 'edit' }" type="button" @click="setSummaryMode('edit')">
            编辑
          </button>
          <button class="mode-btn" :class="{ active: summaryMode === 'export' }" type="button" @click="setSummaryMode('export')">
            导出
          </button>
          <span v-if="hasUnsavedChanges" class="draft-pill">未保存</span>
        </div>

        <div class="detail-body">
          <div v-if="loading && !detail" class="detail-loading">正在加载会议详情...</div>

          <template v-else-if="activeTab === 'summary'">
            <div v-if="isProcessing" class="detail-progress">
              <div class="spinner" />
              <p>会议纪要正在生成，请稍候，页面会自动刷新</p>
            </div>
            <div v-else-if="!summaryReady && summaryMode !== 'edit'" class="detail-empty">
              暂无会议纪要内容，可切换到“编辑”补充纪要
            </div>
            <template v-else>
              <article v-if="summaryMode === 'preview'" class="markdown-body" v-html="renderedHtml" />

              <section v-else-if="summaryMode === 'edit'" class="editor-panel">
                <label class="editor-field wide">
                  <span>标题</span>
                  <input v-model="draftTitle" type="text" maxlength="120" spellcheck="false">
                </label>

                <div class="meta-grid">
                  <label v-for="field in MEETING_META_FIELDS" :key="field.key" class="editor-field">
                    <span>{{ field.label }}</span>
                    <input
                      v-model="draftMeta[field.key]"
                      type="text"
                      spellcheck="false"
                      :placeholder="field.placeholder"
                      @input="syncMetaFieldToDraft(field.key)"
                    >
                  </label>
                </div>

                <div class="meta-actions">
                  <button class="small-btn" type="button" @click="applyMeetingInfoToDraft">使用会议信息</button>
                  <button class="small-btn" type="button" @click="syncMetaFieldsToDraft">更新正文</button>
                </div>

                <label class="editor-field wide">
                  <span>Markdown</span>
                  <textarea v-model="draftContent" spellcheck="false" />
                </label>
              </section>

              <section v-else class="export-panel">
                <div class="export-options">
                  <label class="check-row">
                    <input v-model="exportOptions.includeTitle" type="checkbox">
                    <span>包含标题</span>
                  </label>
                  <label class="check-row">
                    <input v-model="exportOptions.includeMeta" type="checkbox">
                    <span>包含会议信息</span>
                  </label>
                  <label class="check-row">
                    <input v-model="exportOptions.includeTranscript" type="checkbox">
                    <span>附加逐字稿</span>
                  </label>
                </div>

                <div class="export-group">
                  <span class="export-label">字号</span>
                  <div class="segmented-control">
                    <button
                      v-for="item in EXPORT_FONT_SIZES"
                      :key="item"
                      type="button"
                      :class="{ active: exportOptions.fontSize === item }"
                      @click="exportOptions.fontSize = item"
                    >
                      {{ EXPORT_FONT_CONFIG[item].label }}
                    </button>
                  </div>
                </div>

                <div class="export-group">
                  <span class="export-label">强调色</span>
                  <div class="swatch-row">
                    <button
                      v-for="item in EXPORT_ACCENTS"
                      :key="item"
                      class="swatch-btn"
                      type="button"
                      :class="{ active: exportOptions.accent === item }"
                      :aria-label="EXPORT_ACCENT_CONFIG[item].label"
                      @click="exportOptions.accent = item"
                    >
                      <span :style="{ background: EXPORT_ACCENT_CONFIG[item].color }" />
                    </button>
                  </div>
                </div>

                <div class="export-preview" :style="exportPreviewStyle">
                  <article class="preview-body markdown-body" v-html="exportPreviewHtml" />

                  <section v-if="exportOptions.includeTranscript && exportPreviewTranscriptItems.length" class="preview-transcripts">
                    <h4>逐字稿</h4>
                    <div v-for="(item, index) in exportPreviewTranscriptItems" :key="index" class="preview-transcript-item">
                      <div class="preview-transcript-meta">{{ item.speaker_label || '说话人' }} · {{ item.start_time.toFixed(1) }}s</div>
                      <p class="preview-transcript-text">{{ item.text }}</p>
                    </div>
                  </section>
                </div>
              </section>
            </template>
          </template>

          <template v-else>
            <div v-if="!detail || detail.transcripts.length === 0" class="detail-empty">
              暂无逐字稿内容
            </div>
            <ol v-else class="transcript-list">
              <li v-for="(item, index) in detail.transcripts" :key="index" class="transcript-item">
                <div class="transcript-meta">
                  <span class="speaker">{{ item.speaker_label || '说话人' }}</span>
                  <span class="time">{{ item.start_time.toFixed(1) }}s</span>
                </div>
                <p class="transcript-text">{{ item.text }}</p>
              </li>
            </ol>
          </template>
        </div>

        <footer v-if="showSummaryFooter" class="detail-footer">
          <template v-if="summaryMode === 'edit'">
            <button class="primary-btn" type="button" :disabled="!canSave" @click="saveEdits">
              {{ saving ? '保存中...' : '保存' }}
            </button>
            <button class="action-btn" type="button" :disabled="saving" @click="cancelEdit">取消</button>
            <button class="action-btn" type="button" :disabled="saving" @click="setSummaryMode('preview')">预览</button>
          </template>

          <template v-else-if="summaryMode === 'export'">
            <button class="primary-btn" type="button" :disabled="!canExport" @click="exportToPdf">
              {{ exporting ? '准备中...' : '导出 PDF' }}
            </button>
            <button class="action-btn" type="button" @click="setSummaryMode('preview')">预览</button>
          </template>
        </footer>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.meeting-detail {
  position: absolute;
  inset: 0;
  z-index: 5000;
  background: rgba(15, 23, 42, 0.4);
  backdrop-filter: blur(6px);
  display: flex;
  justify-content: flex-end;
}

.detail-panel {
  width: min(560px, 100%);
  height: 100%;
  background: #ffffff;
  display: flex;
  flex-direction: column;
  box-shadow: -16px 0 32px rgba(15, 23, 42, 0.18);
}

.detail-slide-enter-active,
.detail-slide-leave-active {
  transition: opacity 0.2s ease;
}

.detail-slide-enter-active .detail-panel,
.detail-slide-leave-active .detail-panel {
  transition: transform 0.24s cubic-bezier(0.32, 0.72, 0.32, 1);
}

.detail-slide-enter-from,
.detail-slide-leave-to {
  opacity: 0;
}

.detail-slide-enter-from .detail-panel,
.detail-slide-leave-to .detail-panel {
  transform: translateX(40px);
}

.detail-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.7);
}

.detail-back {
  border: 0;
  background: transparent;
  color: #475569;
  font-size: 12px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 4px 8px;
  border-radius: 6px;
  transition: background 0.18s ease;
}

.detail-back:hover {
  background: rgba(241, 245, 249, 0.92);
}

.back-arrow {
  font-size: 16px;
  line-height: 1;
}

.detail-title {
  flex: 1;
  min-width: 0;
  font-size: 14px;
  font-weight: 700;
  color: #0f172a;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-tabs {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.7);
  flex-wrap: wrap;
}

.detail-tab {
  border: 0;
  background: transparent;
  padding: 6px 14px;
  border-radius: 999px;
  color: #475569;
  font-size: 12px;
  cursor: pointer;
  transition: background 0.18s ease, color 0.18s ease;
}

.detail-tab.active {
  background: rgba(15, 118, 110, 0.12);
  color: #0f766e;
  font-weight: 600;
}

.detail-tab:hover:not(.active) {
  background: rgba(241, 245, 249, 0.92);
}

.detail-actions {
  margin-left: auto;
  display: flex;
  gap: 6px;
}

.action-btn,
.small-btn,
.primary-btn {
  border-radius: 999px;
  padding: 5px 12px;
  font-size: 11px;
  border: 1px solid rgba(148, 163, 184, 0.36);
  background: #ffffff;
  color: #334155;
  cursor: pointer;
  transition: background 0.18s ease, border-color 0.18s ease, color 0.18s ease;
}

.action-btn:hover:not(:disabled),
.small-btn:hover:not(:disabled) {
  background: rgba(241, 245, 249, 0.92);
}

.primary-btn {
  border-color: #0f766e;
  background: #0f766e;
  color: #ffffff;
  font-weight: 600;
}

.primary-btn:hover:not(:disabled) {
  background: #115e59;
  border-color: #115e59;
}

.action-btn:disabled,
.small-btn:disabled,
.primary-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.detail-error,
.detail-success {
  margin: 8px 16px 0;
  padding: 8px 12px;
  font-size: 12px;
  border-radius: 8px;
}

.detail-error {
  color: #b91c1c;
  background: rgba(254, 242, 242, 0.92);
}

.detail-success {
  color: #166534;
  background: rgba(220, 252, 231, 0.9);
}

.detail-body {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
}

.detail-loading,
.detail-empty {
  text-align: center;
  font-size: 12px;
  color: #94a3b8;
  padding: 40px 0;
}

.detail-progress {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 32px 0;
  color: #475569;
  font-size: 12px;
}

.spinner {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  border: 3px solid rgba(15, 118, 110, 0.2);
  border-top-color: #0f766e;
  animation: spin 0.9s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.summary-toolbar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 16px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.72);
  background: rgba(255, 255, 255, 0.96);
  flex-wrap: wrap;
}

.mode-btn {
  border: 0;
  border-radius: 999px;
  background: rgba(241, 245, 249, 0.92);
  color: #475569;
  padding: 5px 12px;
  font-size: 12px;
  cursor: pointer;
}

.mode-btn.active {
  background: rgba(15, 118, 110, 0.12);
  color: #0f766e;
  font-weight: 600;
}

.draft-pill {
  margin-left: auto;
  padding: 3px 8px;
  border-radius: 999px;
  background: rgba(254, 249, 195, 0.92);
  color: #854d0e;
  font-size: 11px;
  font-weight: 600;
}

.editor-panel,
.export-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.meta-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.editor-field {
  display: flex;
  flex-direction: column;
  gap: 5px;
  min-width: 0;
}

.editor-field.wide {
  grid-column: 1 / -1;
}

.editor-field span,
.export-label {
  font-size: 11px;
  font-weight: 600;
  color: #64748b;
}

.editor-field input,
.editor-field textarea {
  width: 100%;
  box-sizing: border-box;
  border: 1px solid rgba(148, 163, 184, 0.34);
  border-radius: 8px;
  background: #ffffff;
  color: #0f172a;
  outline: none;
  font-size: 12px;
  transition: border-color 0.18s ease, box-shadow 0.18s ease;
}

.editor-field input {
  height: 32px;
  padding: 0 10px;
}

.editor-field textarea {
  min-height: 220px;
  resize: vertical;
  padding: 10px 12px;
  line-height: 1.65;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.editor-field input:focus,
.editor-field textarea:focus {
  border-color: #0f766e;
  box-shadow: 0 0 0 3px rgba(15, 118, 110, 0.1);
}

.meta-actions {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.export-options {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.check-row {
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  border: 1px solid rgba(226, 232, 240, 0.88);
  border-radius: 8px;
  padding: 8px 10px;
  color: #334155;
  font-size: 12px;
}

.check-row input {
  accent-color: #0f766e;
}

.export-group {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.segmented-control {
  display: inline-flex;
  background: rgba(241, 245, 249, 0.92);
  border-radius: 999px;
  padding: 3px;
  gap: 2px;
}

.segmented-control button {
  border: 0;
  border-radius: 999px;
  background: transparent;
  color: #475569;
  font-size: 11px;
  padding: 5px 10px;
  cursor: pointer;
}

.segmented-control button.active {
  background: #ffffff;
  color: #0f766e;
  font-weight: 600;
  box-shadow: 0 1px 4px rgba(15, 23, 42, 0.08);
}

.swatch-row {
  display: flex;
  gap: 8px;
}

.swatch-btn {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  border: 1px solid rgba(148, 163, 184, 0.36);
  background: #ffffff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}

.swatch-btn span {
  width: 16px;
  height: 16px;
  border-radius: 50%;
}

.swatch-btn.active {
  border-color: #0f766e;
  box-shadow: 0 0 0 3px rgba(15, 118, 110, 0.12);
}

.export-preview {
  --preview-accent: #0f766e;
  --preview-soft: rgba(15, 118, 110, 0.1);
  --preview-font-size: 13.5px;
  --preview-line-height: 1.75;
  border: 1px solid var(--preview-soft);
  border-radius: 8px;
  padding: 14px;
  background: linear-gradient(180deg, #ffffff, rgba(248, 250, 252, 0.9));
  box-shadow: inset 0 0 0 1px var(--preview-soft);
}

.preview-body {
  font-size: var(--preview-font-size);
  line-height: var(--preview-line-height);
  max-height: 220px;
  overflow: hidden;
}

.preview-body.markdown-body :deep(h1) {
  font-size: calc(var(--preview-font-size) + 4px);
  color: var(--preview-accent);
}

.preview-body.markdown-body :deep(h2) {
  font-size: calc(var(--preview-font-size) + 2px);
  color: var(--preview-accent);
}

.preview-transcripts {
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px solid var(--preview-soft);
}

.preview-transcripts h4 {
  margin: 0 0 8px;
  color: var(--preview-accent);
  font-size: 12px;
}

.preview-transcript-item {
  padding: 8px 10px;
  border-left: 3px solid var(--preview-accent);
  background: rgba(248, 250, 252, 0.92);
  border-radius: 0 8px 8px 0;
}

.preview-transcript-item + .preview-transcript-item {
  margin-top: 8px;
}

.preview-transcript-meta {
  font-size: 11px;
  color: #64748b;
  margin-bottom: 4px;
}

.preview-transcript-text {
  margin: 0;
  color: #1e293b;
  font-size: 12px;
  line-height: 1.6;
  display: -webkit-box;
  line-clamp: 2;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.detail-footer {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 12px 16px;
  border-top: 1px solid rgba(226, 232, 240, 0.72);
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.95), #ffffff);
  box-shadow: 0 -8px 18px rgba(15, 23, 42, 0.05);
}

.markdown-body {
  font-size: 13px;
  line-height: 1.7;
  color: #0f172a;
  word-break: break-word;
}

.markdown-body :deep(h1) { font-size: 18px; margin: 12px 0 6px; font-weight: 700; }
.markdown-body :deep(h2) { font-size: 15px; margin: 12px 0 6px; font-weight: 700; color: #0f766e; }
.markdown-body :deep(h3) { font-size: 13px; margin: 10px 0 4px; font-weight: 700; }
.markdown-body :deep(p) { margin: 6px 0; }
.markdown-body :deep(ul),
.markdown-body :deep(ol) { padding-left: 22px; margin: 6px 0; }
.markdown-body :deep(li) { margin: 3px 0; }
.markdown-body :deep(code) {
  background: rgba(15, 23, 42, 0.06);
  padding: 1px 6px;
  border-radius: 4px;
  font-family: ui-monospace, monospace;
  font-size: 12px;
}
.markdown-body :deep(pre) {
  background: rgba(15, 23, 42, 0.06);
  padding: 10px 12px;
  border-radius: 8px;
  overflow-x: auto;
  font-size: 12px;
}
.markdown-body :deep(blockquote) {
  border-left: 3px solid rgba(15, 118, 110, 0.4);
  padding: 4px 10px;
  color: #475569;
  margin: 8px 0;
  background: rgba(15, 118, 110, 0.04);
  border-radius: 0 6px 6px 0;
}
.markdown-body :deep(table) {
  border-collapse: collapse;
  width: 100%;
  margin: 8px 0;
  font-size: 12px;
}
.markdown-body :deep(th),
.markdown-body :deep(td) {
  border: 1px solid rgba(148, 163, 184, 0.36);
  padding: 4px 8px;
}

.transcript-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.transcript-item {
  border-radius: 8px;
  background: rgba(241, 245, 249, 0.6);
  padding: 10px 12px;
}

.transcript-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11px;
  color: #64748b;
  margin-bottom: 4px;
}

.speaker {
  font-weight: 600;
  color: #0f766e;
}

.transcript-text {
  margin: 0;
  font-size: 13px;
  line-height: 1.7;
  color: #0f172a;
  white-space: pre-wrap;
}

.status-pill {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border-radius: 999px;
  padding: 3px 10px;
  font-size: 11px;
  font-weight: 600;
}

.status-pill.success {
  background: rgba(220, 252, 231, 0.9);
  color: #166534;
}

.status-pill.pending {
  background: rgba(254, 249, 195, 0.92);
  color: #854d0e;
}

.status-pill.danger {
  background: rgba(254, 242, 242, 0.92);
  color: #b91c1c;
}

.pulse-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
  animation: pulse 1.4s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 0.4; transform: scale(0.9); }
  50% { opacity: 1; transform: scale(1.1); }
}

@media (max-width: 460px) {
  .detail-actions {
    width: 100%;
    justify-content: flex-end;
  }

  .summary-toolbar {
    justify-content: flex-start;
  }

  .meta-grid,
  .export-options {
    grid-template-columns: 1fr;
  }
}
</style>