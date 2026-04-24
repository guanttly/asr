<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import DictPickerDialog from './DictPickerDialog.vue'

import { TRANSCRIPTION_TASK_TYPES } from '@/constants/transcription'
import { useConfirm } from '@/composables/useConfirm'
import { useInjector } from '@/composables/useInjector'
import { debugLog } from '@/utils/debug'
import {
  clearTranscriptionTasks,
  deleteTranscriptionTask,
  getTranscriptionTaskExecutions,
  getTranscriptionTasks,
  type TranscriptionTaskItem,
  type WorkflowExecutionItem,
} from '@/utils/transcription'

const { injectText } = useInjector()
const { confirm } = useConfirm()

const PAGE_SIZE = 10

interface HistoryRecord extends TranscriptionTaskItem {
  finalText: string
  loadingFinalText: boolean
  finalTextLoaded: boolean
  deleting: boolean
}

const listRef = ref<HTMLElement | null>(null)
const items = ref<HistoryRecord[]>([])
const total = ref(0)
const initialLoading = ref(false)
const loadingMore = ref(false)
const clearing = ref(false)
const feedbackText = ref('')
const feedbackType = ref<'error' | 'info' | 'success'>('info')

const dialogVisible = ref(false)
const dialogKind = ref<'term' | 'sensitive'>('term')
const dialogText = ref('')

const hasMore = computed(() => items.value.length < total.value)

function setFeedback(type: 'error' | 'info' | 'success', text: string) {
  feedbackType.value = type
  feedbackText.value = text
}

function normalizeText(value?: string) {
  return value?.trim() || ''
}

function createHistoryRecord(task: TranscriptionTaskItem): HistoryRecord {
  return {
    ...task,
    finalText: normalizeText(task.result_text),
    loadingFinalText: Boolean(task.workflow_id),
    finalTextLoaded: !task.workflow_id,
    deleting: false,
  }
}

async function loadFinalText(item: HistoryRecord) {
  if (item.finalTextLoaded || !item.workflow_id)
    return
  try {
    const executions: WorkflowExecutionItem[] = await getTranscriptionTaskExecutions(item.id)
    const latest = executions[0]
    const finalCandidate = normalizeText(latest?.final_text)
    if (finalCandidate)
      item.finalText = finalCandidate
    item.finalTextLoaded = true
  }
  catch (error) {
    item.finalTextLoaded = true
    void debugLog('history.execution', 'failed to load final text', error instanceof Error ? {
      taskId: item.id, message: error.message, stack: error.stack,
    } : { taskId: item.id, error })
  }
  finally {
    item.loadingFinalText = false
  }
}

function mergeTaskPage(nextItems: TranscriptionTaskItem[], reset: boolean) {
  if (reset) {
    items.value = nextItems.map(createHistoryRecord)
    items.value.forEach(item => void loadFinalText(item))
    return
  }

  const seen = new Set(items.value.map(item => item.id))
  const appended = nextItems
    .filter(item => !seen.has(item.id))
    .map(createHistoryRecord)
  items.value = items.value.concat(appended)
  appended.forEach(item => void loadFinalText(item))
}

function formatDate(value?: string) {
  if (!value)
    return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime()))
    return value
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

function formatDuration(value?: number) {
  if (!value || value <= 0)
    return '时长未知'
  if (value < 60)
    return `${Math.round(value)} 秒`
  const minutes = Math.floor(value / 60)
  const seconds = Math.round(value % 60)
  return `${minutes} 分 ${seconds} 秒`
}

async function loadTasks(reset = false) {
  if (initialLoading.value || loadingMore.value)
    return

  if (reset)
    initialLoading.value = true
  else
    loadingMore.value = true

  try {
    const result = await getTranscriptionTasks({
      type: TRANSCRIPTION_TASK_TYPES.REALTIME,
      offset: reset ? 0 : items.value.length,
      limit: PAGE_SIZE,
    })
    total.value = result.total || 0
    mergeTaskPage(result.items || [], reset)
    if (!items.value.length)
      setFeedback('info', '暂无转写记录')
    else if (reset)
      setFeedback('success', `已加载 ${items.value.length} 条记录`)
  }
  catch (error) {
    setFeedback('error', error instanceof Error ? error.message : '加载转写记录失败')
    void debugLog('history.list', 'failed to load history list', error instanceof Error ? {
      message: error.message,
      stack: error.stack,
    } : error)
  }
  finally {
    initialLoading.value = false
    loadingMore.value = false
  }
}

async function injectRecord(item: HistoryRecord) {
  if (!item.finalText) {
    setFeedback('info', '当前记录没有可注入的文本')
    return
  }

  const result = await injectText(item.finalText)
  if (!result.success) {
    setFeedback('error', result.message)
    return
  }
  setFeedback('success', '已将转写文本注入到当前光标位置')
}

async function copyRecord(item: HistoryRecord) {
  if (!item.finalText)
    return
  try {
    await navigator.clipboard.writeText(item.finalText)
    setFeedback('success', '已复制到剪贴板')
  }
  catch (error) {
    setFeedback('error', error instanceof Error ? error.message : '复制失败')
  }
}

function openTermDialog(item: HistoryRecord) {
  dialogKind.value = 'term'
  dialogText.value = item.finalText
  dialogVisible.value = true
}

function openSensitiveDialog(item: HistoryRecord) {
  dialogKind.value = 'sensitive'
  dialogText.value = item.finalText
  dialogVisible.value = true
}

function handleDictSuccess(payload: { kind: 'term' | 'sensitive', dictName: string, value: string }) {
  setFeedback(
    'success',
    payload.kind === 'term'
      ? `已添加术语「${payload.value}」到「${payload.dictName}」`
      : `已添加敏感词「${payload.value}」到「${payload.dictName}」`,
  )
}

async function removeRecord(item: HistoryRecord) {
  if (item.deleting)
    return
  const ok = await confirm({
    title: '删除转写记录',
    message: '确认删除这条转写记录吗？',
    confirmText: '删除',
    tone: 'danger',
  })
  if (!ok)
    return

  item.deleting = true
  try {
    await deleteTranscriptionTask(item.id)
    items.value = items.value.filter(candidate => candidate.id !== item.id)
    total.value = Math.max(0, total.value - 1)
    setFeedback('success', '转写记录已删除')
  }
  catch (error) {
    setFeedback('error', error instanceof Error ? error.message : '删除转写记录失败')
  }
  finally {
    item.deleting = false
  }
}

async function clearAll() {
  if (clearing.value)
    return
  const ok = await confirm({
    title: '清空记录',
    message: '确认清空当前账号下的实时转写记录吗？',
    confirmText: '清空',
    tone: 'danger',
  })
  if (!ok)
    return

  clearing.value = true
  try {
    const result = await clearTranscriptionTasks(TRANSCRIPTION_TASK_TYPES.REALTIME)
    await loadTasks(true)
    setFeedback('success', `已删除 ${result.deleted_count} 条记录${result.skipped_count > 0 ? `，跳过 ${result.skipped_count} 条进行中的任务` : ''}`)
  }
  catch (error) {
    setFeedback('error', error instanceof Error ? error.message : '清空转写记录失败')
  }
  finally {
    clearing.value = false
  }
}

function handleScroll() {
  const container = listRef.value
  if (!container || loadingMore.value || initialLoading.value || !hasMore.value)
    return
  if (container.scrollTop + container.clientHeight >= container.scrollHeight - 120)
    void loadTasks(false)
}

onMounted(() => {
  void loadTasks(true)
})
</script>

<template>
  <div class="history-list">
    <div class="list-header">
      <div>
        <div class="list-title">实时转写记录</div>
        <span class="list-count">已加载 {{ items.length }} / {{ total }} 条</span>
      </div>
      <div class="list-actions">
        <button class="ghost-btn" :disabled="initialLoading" @click="loadTasks(true)">刷新</button>
        <button class="clear-btn" :disabled="clearing || initialLoading || items.length === 0" @click="clearAll">
          {{ clearing ? '清空中...' : '清空' }}
        </button>
      </div>
    </div>

    <Transition name="feedback">
      <p v-if="feedbackText" :key="feedbackText" class="feedback" :class="feedbackType">
        {{ feedbackText }}
      </p>
    </Transition>

    <div ref="listRef" class="history-scroller" @scroll="handleScroll">
      <div v-if="initialLoading" class="empty-state">
        正在加载转写记录...
      </div>
      <div v-else-if="items.length === 0" class="empty-state">
        暂无转写记录
      </div>

      <article
        v-for="item in items"
        :key="item.id"
        class="history-card"
      >
        <div class="card-meta">
          <span>{{ formatDate(item.created_at) }}</span>
          <span class="meta-sep">·</span>
          <span>{{ formatDuration(item.duration) }}</span>
        </div>

        <p v-if="item.loadingFinalText && !item.finalText" class="card-text placeholder">
          正在生成最终输出...
        </p>
        <p v-else class="card-text">
          {{ item.finalText || '（空）' }}
        </p>

        <div class="card-actions">
          <button class="card-btn" :disabled="!item.finalText" @click="injectRecord(item)">
            <span class="btn-icon">↳</span> 注入
          </button>
          <button class="card-btn" :disabled="!item.finalText" @click="copyRecord(item)">
            <span class="btn-icon">⧉</span> 复制
          </button>
          <button class="card-btn" :disabled="!item.finalText" @click="openTermDialog(item)">
            <span class="btn-icon">+</span> 收录术语
          </button>
          <button class="card-btn" :disabled="!item.finalText" @click="openSensitiveDialog(item)">
            <span class="btn-icon">!</span> 加敏感词
          </button>
          <button class="card-btn danger" :disabled="item.deleting" @click="removeRecord(item)">
            <span class="btn-icon">×</span> 删除
          </button>
        </div>
      </article>

      <div v-if="loadingMore" class="load-state">
        正在加载更多记录...
      </div>
      <button v-else-if="hasMore" class="load-more" @click="loadTasks(false)">
        加载更多
      </button>
      <div v-else-if="items.length > 0" class="load-state">
        已加载全部记录
      </div>
    </div>

    <DictPickerDialog
      :visible="dialogVisible"
      :kind="dialogKind"
      :default-text="dialogText"
      @close="dialogVisible = false"
      @success="handleDictSuccess"
    />
  </div>
</template>

<style scoped>
.history-list {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 12px;
  gap: 12px;
}

.empty-state {
  padding: 36px 0;
  text-align: center;
  font-size: 13px;
  color: #94a3b8;
}

.list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}

.list-title {
  font-size: 14px;
  font-weight: 700;
  color: #16202c;
}

.list-count {
  font-size: 11px;
  color: #94a3b8;
}

.list-actions {
  display: flex;
  gap: 8px;
}

.clear-btn,
.ghost-btn,
.load-more {
  border-radius: 999px;
  padding: 6px 14px;
  font-size: 12px;
  cursor: pointer;
}

.clear-btn {
  color: #fff;
  background: linear-gradient(135deg, #ef4444, #dc2626);
  border: 0;
}

.ghost-btn {
  border: 1px solid rgba(148, 163, 184, 0.36);
  background: rgba(255, 255, 255, 0.92);
  color: #475569;
}

.clear-btn:disabled,
.ghost-btn:disabled,
.load-more:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.feedback {
  margin: 0;
  border-radius: 12px;
  padding: 8px 12px;
  font-size: 12px;
}

.feedback.info {
  color: #475569;
  background: rgba(241, 245, 249, 0.92);
}

.feedback.success {
  color: #166534;
  background: rgba(220, 252, 231, 0.92);
}

.feedback.error {
  color: #b91c1c;
  background: rgba(254, 242, 242, 0.92);
}

.feedback-enter-active,
.feedback-leave-active {
  transition: opacity 0.18s ease, transform 0.18s ease;
}

.feedback-enter-from,
.feedback-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

.history-scroller {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding-right: 2px;
}

.history-card {
  border-radius: 16px;
  border: 1px solid rgba(226, 232, 240, 0.84);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(248, 250, 252, 0.96));
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  box-shadow: 0 8px 18px rgba(148, 163, 184, 0.10);
  transition: transform 0.18s ease, box-shadow 0.18s ease;
}

.history-card:hover {
  transform: translateY(-1px);
  box-shadow: 0 12px 22px rgba(148, 163, 184, 0.18);
}

.card-meta {
  font-size: 11px;
  color: #94a3b8;
  display: flex;
  align-items: center;
}

.meta-sep {
  margin: 0 6px;
}

.card-text {
  margin: 0;
  font-size: 13px;
  line-height: 1.7;
  color: #0f172a;
  white-space: pre-wrap;
  word-break: break-word;
}

.card-text.placeholder {
  color: #94a3b8;
  font-style: italic;
}

.card-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.card-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border-radius: 999px;
  padding: 4px 12px;
  font-size: 11px;
  border: 1px solid rgba(148, 163, 184, 0.32);
  background: rgba(248, 250, 252, 0.96);
  color: #334155;
  cursor: pointer;
  transition: background 0.18s ease, color 0.18s ease, border-color 0.18s ease;
}

.card-btn:hover:not(:disabled) {
  background: #ffffff;
  border-color: #0f766e;
  color: #0f766e;
}

.card-btn.danger:hover:not(:disabled) {
  border-color: #dc2626;
  color: #dc2626;
}

.card-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.btn-icon {
  font-size: 12px;
  line-height: 1;
}

.load-state {
  color: #64748b;
  font-size: 12px;
  text-align: center;
  padding: 8px 0;
}

.load-more {
  width: 100%;
  border: 1px dashed rgba(15, 118, 110, 0.32);
  background: rgba(240, 253, 250, 0.74);
  color: #0f766e;
}
</style>
