<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import TextDiffPreview from './TextDiffPreview.vue'

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

const PAGE_SIZE = 10

interface HistoryRecord extends TranscriptionTaskItem {
  expanded: boolean
  deleting: boolean
  loadingExecutions: boolean
  executionsLoaded: boolean
  executions: WorkflowExecutionItem[]
}

const listRef = ref<HTMLElement | null>(null)
const items = ref<HistoryRecord[]>([])
const total = ref(0)
const initialLoading = ref(false)
const loadingMore = ref(false)
const clearing = ref(false)
const feedbackText = ref('')
const feedbackType = ref<'error' | 'info' | 'success'>('info')

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
    expanded: false,
    deleting: false,
    loadingExecutions: false,
    executionsLoaded: false,
    executions: [],
  }
}

function mergeTaskPage(nextItems: TranscriptionTaskItem[], reset: boolean) {
  if (reset) {
    items.value = nextItems.map(createHistoryRecord)
    return
  }

  const seen = new Set(items.value.map(item => item.id))
  const appended = nextItems
    .filter(item => !seen.has(item.id))
    .map(createHistoryRecord)
  items.value = items.value.concat(appended)
}

function latestExecution(item: HistoryRecord) {
  return item.executions[0] || null
}

function finalOutputText(item: HistoryRecord) {
  return normalizeText(latestExecution(item)?.final_text) || normalizeText(item.result_text)
}

function workflowStatusMeta(item: HistoryRecord) {
  const execution = latestExecution(item)
  if (!item.workflow_id)
    return { label: '原始输出', tone: 'plain' }
  if (execution?.status === 'completed')
    return { label: '工作流已完成', tone: 'success' }
  if (execution?.status === 'failed' || item.post_process_status === 'failed')
    return { label: '工作流失败', tone: 'danger' }
  if (item.post_process_status === 'processing')
    return { label: '工作流处理中', tone: 'pending' }
  return { label: '等待工作流', tone: 'pending' }
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
      type: 'realtime',
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
    await debugLog('history.list', 'failed to load history list', error instanceof Error ? {
      message: error.message,
      stack: error.stack,
    } : error)
  }
  finally {
    initialLoading.value = false
    loadingMore.value = false
  }
}

async function loadExecutions(item: HistoryRecord) {
  if (item.loadingExecutions || item.executionsLoaded)
    return

  item.loadingExecutions = true
  try {
    item.executions = await getTranscriptionTaskExecutions(item.id)
    item.executionsLoaded = true
  }
  catch (error) {
    setFeedback('error', error instanceof Error ? error.message : '加载处理链路失败')
    await debugLog('history.execution', 'failed to load task executions', error instanceof Error ? {
      taskId: item.id,
      message: error.message,
      stack: error.stack,
    } : { taskId: item.id, error })
  }
  finally {
    item.loadingExecutions = false
  }
}

async function toggleExpanded(item: HistoryRecord) {
  item.expanded = !item.expanded
  if (item.expanded)
    await loadExecutions(item)
}

async function injectRecord(item: HistoryRecord) {
  const text = finalOutputText(item)
  if (!text) {
    setFeedback('info', '当前记录没有可注入的输出文本')
    return
  }

  const result = await injectText(text)
  if (!result.success) {
    setFeedback('error', result.message)
    return
  }
  setFeedback('success', '已将最终输出注入到当前光标位置')
}

async function removeRecord(item: HistoryRecord) {
  if (item.deleting)
    return
  if (!window.confirm('确认删除这条转写记录吗？'))
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
  if (!window.confirm('确认清空当前账号下的实时转写记录吗？'))
    return

  clearing.value = true
  try {
    const result = await clearTranscriptionTasks('realtime')
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
        <button class="ghost-btn" @click="loadTasks(true)">刷新</button>
        <button class="clear-btn" :disabled="clearing || initialLoading" @click="clearAll">
          {{ clearing ? '清空中...' : '清空' }}
        </button>
      </div>
    </div>

    <p v-if="feedbackText" class="feedback" :class="feedbackType">
      {{ feedbackText }}
    </p>

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
        <div class="history-card__summary" @click="toggleExpanded(item)">
          <div class="summary-top">
            <div class="summary-meta">
              <span>{{ formatDate(item.created_at) }}</span>
              <span class="summary-sep">·</span>
              <span>{{ formatDuration(item.duration) }}</span>
            </div>
            <span class="status-pill" :class="workflowStatusMeta(item).tone">
              {{ workflowStatusMeta(item).label }}
            </span>
          </div>

          <div class="summary-block">
            <div class="summary-label">原始识别</div>
            <div class="summary-text">{{ normalizeText(item.result_text) || '暂无原始文本' }}</div>
          </div>

          <div class="summary-block summary-block--output">
            <div class="summary-label">最终输出</div>
            <div class="summary-text">{{ finalOutputText(item) || '暂无最终输出' }}</div>
          </div>

          <div class="summary-footer">
            <span>任务 #{{ item.id }}</span>
            <span>{{ item.expanded ? '收起详情' : '展开详情' }}</span>
          </div>
        </div>

        <div v-if="item.expanded" class="history-card__detail">
          <div class="detail-actions">
            <button class="ghost-btn" @click="injectRecord(item)">注入输出</button>
            <button class="danger-btn" :disabled="item.deleting" @click="removeRecord(item)">
              {{ item.deleting ? '删除中...' : '删除记录' }}
            </button>
          </div>

          <TextDiffPreview
            before-label="原始识别"
            after-label="最终输出"
            :before-text="normalizeText(item.result_text)"
            :after-text="finalOutputText(item)"
          />

          <div v-if="item.loadingExecutions" class="execution-empty">
            正在加载处理链路...
          </div>
          <template v-else-if="latestExecution(item)">
            <div class="execution-head">
              <div>
                <div class="execution-title">整段复核链路</div>
                <div class="execution-meta">
                  {{ formatDate(latestExecution(item)?.created_at) }}
                  <span v-if="latestExecution(item)?.error_message">
                    · {{ latestExecution(item)?.error_message }}
                  </span>
                </div>
              </div>
              <span class="status-pill" :class="latestExecution(item)?.status === 'completed' ? 'success' : 'danger'">
                {{ latestExecution(item)?.status === 'completed' ? '执行完成' : '执行异常' }}
              </span>
            </div>

            <div v-if="latestExecution(item)?.node_results?.length" class="node-list">
              <section v-for="node in latestExecution(item)?.node_results" :key="node.id" class="node-card">
                <div class="node-head">
                  <div>
                    <div class="node-title">{{ node.label }}</div>
                    <div class="node-meta">节点 {{ node.position }} · {{ node.duration_ms }} ms</div>
                  </div>
                  <span class="status-pill" :class="node.status === 'success' ? 'success' : (node.status === 'failed' ? 'danger' : 'plain')">
                    {{ node.status }}
                  </span>
                </div>
                <TextDiffPreview
                  before-label="节点输入"
                  after-label="节点输出"
                  :before-text="normalizeText(node.input_text)"
                  :after-text="normalizeText(node.output_text)"
                />
              </section>
            </div>
          </template>
          <div v-else class="execution-empty">
            {{ item.workflow_id ? (item.post_process_error || '整段工作流尚未返回执行详情') : '当前记录未绑定整段工作流，最终输出即为原始识别文本。' }}
          </div>
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
  font-size: 15px;
  font-weight: 700;
  color: #16202c;
}

.list-count {
  font-size: 12px;
  color: #94a3b8;
}

.list-actions {
  display: flex;
  gap: 8px;
}

.clear-btn,
.ghost-btn,
.danger-btn,
.load-more {
  border-radius: 999px;
  padding: 8px 12px;
  font-size: 12px;
  cursor: pointer;
}

.clear-btn {
  color: #fff;
  background: linear-gradient(135deg, #ef4444, #dc2626);
  border: 0;
}

.ghost-btn,
.danger-btn {
  border: 1px solid rgba(148, 163, 184, 0.24);
  background: rgba(255, 255, 255, 0.82);
  color: #475569;
}

.danger-btn {
  color: #b91c1c;
  border-color: rgba(248, 113, 113, 0.25);
  background: rgba(254, 242, 242, 0.92);
}

.clear-btn:disabled,
.ghost-btn:disabled,
.danger-btn:disabled,
.load-more:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.feedback {
  margin: 0;
  border-radius: 12px;
  padding: 10px 12px;
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

.history-scroller {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding-right: 2px;
}

.history-card {
  border-radius: 18px;
  border: 1px solid rgba(226, 232, 240, 0.84);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(248, 250, 252, 0.96)),
    radial-gradient(circle at top right, rgba(15, 118, 110, 0.08), transparent 34%);
  box-shadow: 0 14px 30px rgba(148, 163, 184, 0.12);
  overflow: hidden;
}

.history-card + .history-card {
  margin-top: 12px;
}

.history-card__summary {
  display: grid;
  gap: 12px;
  padding: 14px;
  cursor: pointer;
}

.history-card__detail {
  display: grid;
  gap: 14px;
  padding: 0 14px 14px;
}

.summary-top,
.summary-footer,
.detail-actions,
.execution-head,
.node-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.summary-meta,
.execution-meta,
.node-meta,
.summary-footer {
  color: #64748b;
  font-size: 12px;
}

.summary-sep {
  margin: 0 4px;
}

.summary-block {
  border-radius: 14px;
  background: rgba(248, 250, 252, 0.86);
  border: 1px solid rgba(226, 232, 240, 0.82);
  padding: 12px;
}

.summary-block--output {
  background: rgba(240, 253, 250, 0.84);
}

.summary-label,
.execution-title,
.node-title {
  margin-bottom: 6px;
  font-size: 12px;
  font-weight: 600;
  color: #0f766e;
}

.summary-text {
  color: #16202c;
  font-size: 13px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.status-pill {
  flex-shrink: 0;
  border-radius: 999px;
  padding: 6px 10px;
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

.status-pill.plain {
  background: rgba(241, 245, 249, 0.94);
  color: #475569;
}

.execution-empty,
.load-state {
  color: #64748b;
  font-size: 12px;
  text-align: center;
  padding: 10px 0;
}

.node-list {
  display: grid;
  gap: 12px;
}

.node-card {
  display: grid;
  gap: 12px;
  border-radius: 16px;
  background: rgba(248, 250, 252, 0.78);
  border: 1px solid rgba(226, 232, 240, 0.84);
  padding: 12px;
}

.load-more {
  width: 100%;
  margin-top: 12px;
  border: 1px dashed rgba(15, 118, 110, 0.28);
  background: rgba(240, 253, 250, 0.74);
  color: #0f766e;
}
</style>