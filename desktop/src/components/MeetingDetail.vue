<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { marked } from 'marked'

import { useConfirm } from '@/composables/useConfirm'
import { getMeetingDetail, regenerateMeetingSummary, type MeetingDetailResponse } from '@/utils/meetings'
import { debugLog } from '@/utils/debug'

const props = defineProps<{ meetingId: number }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'deleted'): void }>()

const detail = ref<MeetingDetailResponse | null>(null)
const loading = ref(false)
const errorText = ref('')
const regenerating = ref(false)
const exporting = ref(false)
const activeTab = ref<'summary' | 'transcript'>('summary')
const printableRef = ref<HTMLElement | null>(null)

const { confirm } = useConfirm()
let pollTimer: ReturnType<typeof setInterval> | null = null

marked.setOptions({ gfm: true, breaks: true })

const renderedHtml = computed(() => {
  const content = detail.value?.summary?.content?.trim()
  if (!content)
    return ''
  try {
    return marked.parse(content) as string
  }
  catch {
    return `<pre>${escapeHtml(content)}</pre>`
  }
})

function escapeHtml(text: string) {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

const summaryReady = computed(() => Boolean(detail.value?.summary?.content?.trim()))

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

async function load(opts?: { silent?: boolean }) {
  if (!opts?.silent)
    loading.value = true
  errorText.value = ''
  try {
    detail.value = await getMeetingDetail(props.meetingId)
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

async function handleRegenerate() {
  if (!detail.value)
    return
  const ok = await confirm({
    title: '重新生成纪要',
    message: '确定使用当前绑定的会议工作流重新生成本次纪要吗？',
    confirmText: '重新生成',
  })
  if (!ok)
    return
  regenerating.value = true
  try {
    await regenerateMeetingSummary(detail.value.id, detail.value.workflow_id ?? null)
    await load({ silent: true })
  }
  catch (error) {
    errorText.value = error instanceof Error ? error.message : '重新生成失败'
  }
  finally {
    regenerating.value = false
  }
}

function buildPrintableHtml(title: string, dateText: string) {
  return `
    <div class="pdf-root" style="font-family: -apple-system, BlinkMacSystemFont, 'PingFang SC', 'Microsoft YaHei', 'Hiragino Sans GB', 'Helvetica Neue', Arial, sans-serif; color: #0f172a; padding: 32px 40px; background: #ffffff; width: 760px; box-sizing: border-box;">
      <h1 style="font-size: 22px; margin: 0 0 6px; color: #0f766e; line-height: 1.4;">${escapeHtml(title)}</h1>
      <div style="color: #64748b; font-size: 12px; margin-bottom: 22px; border-bottom: 1px solid rgba(15,118,110,0.18); padding-bottom: 12px;">${escapeHtml(dateText)}</div>
      <div class="pdf-content" style="font-size: 13.5px; line-height: 1.75; color: #1e293b;">${renderedHtml.value}</div>
    </div>
  `
}

async function exportToPdf() {
  if (!summaryReady.value || exporting.value)
    return
  exporting.value = true
  errorText.value = ''
  // 用临时 DOM 渲染 + html2pdf 直接下载，规避 Tauri WebView 屏蔽 window.open 的问题
  const host = document.createElement('div')
  host.style.position = 'fixed'
  host.style.left = '-99999px'
  host.style.top = '0'
  host.style.zIndex = '-1'
  host.style.pointerEvents = 'none'
  const title = detail.value?.title || `会议纪要 #${detail.value?.id}`
  const dateText = detail.value?.summary?.created_at || ''
  host.innerHTML = buildPrintableHtml(title, dateText)
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
    await html2pdf()
      .from(host.firstElementChild as HTMLElement)
      .set(opts)
      .save()
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
    if (isProcessing.value)
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
  void load()
}, { immediate: false })

onMounted(() => {
  void load()
  startPolling()
})

onBeforeUnmount(() => {
  stopPolling()
})
</script>

<template>
  <Transition name="detail-slide">
    <div class="meeting-detail" @click.self="emit('close')">
      <div class="detail-panel">
        <header class="detail-head">
          <button class="detail-back" type="button" @click="emit('close')">
            <span class="back-arrow">‹</span> 返回
          </button>
          <div class="detail-title">{{ detail?.title || '加载中...' }}</div>
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
            <button class="action-btn" :disabled="!summaryReady || exporting" @click="exportToPdf">
              {{ exporting ? '准备中...' : '导出 PDF' }}
            </button>
            <button class="action-btn" :disabled="regenerating || !detail" @click="handleRegenerate">
              {{ regenerating ? '生成中...' : '重新生成' }}
            </button>
          </div>
        </div>

        <p v-if="errorText" class="detail-error">{{ errorText }}</p>

        <div class="detail-body">
          <div v-if="loading && !detail" class="detail-loading">正在加载会议详情...</div>

          <template v-else-if="activeTab === 'summary'">
            <div v-if="isProcessing" class="detail-progress">
              <div class="spinner" />
              <p>会议纪要正在生成，请稍候，页面会自动刷新</p>
            </div>
            <div v-else-if="!summaryReady" class="detail-empty">
              暂无会议纪要内容，可点击右上方“重新生成”手动触发
            </div>
            <article v-else ref="printableRef" class="markdown-body" v-html="renderedHtml" />
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
  width: min(420px, 100%);
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

.action-btn {
  border-radius: 999px;
  padding: 5px 12px;
  font-size: 11px;
  border: 1px solid rgba(148, 163, 184, 0.36);
  background: #ffffff;
  color: #334155;
  cursor: pointer;
  transition: background 0.18s ease;
}

.action-btn:hover:not(:disabled) {
  background: rgba(241, 245, 249, 0.92);
}

.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.detail-error {
  margin: 8px 16px 0;
  padding: 8px 12px;
  font-size: 12px;
  color: #b91c1c;
  background: rgba(254, 242, 242, 0.92);
  border-radius: 10px;
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
  border-radius: 12px;
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
</style>
