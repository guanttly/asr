<script setup lang="ts">
import { NButton, NTag, NTooltip, useMessage } from 'naive-ui'
import { computed, h, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { createTranscriptionTask, deleteTranscriptionTask, getTranscriptionTaskDetail, getTranscriptionTaskExecutions, getTranscriptionTasks, resumeTranscriptionTaskPostProcess, syncTranscriptionTask, uploadTranscriptionFile } from '@/api/asr'
import NodeDetailPanel from '@/components/NodeDetailPanel.vue'
import TextDiffPreview from '@/components/TextDiffPreview.vue'
import WorkflowSelectionPreview from '@/components/WorkflowSelectionPreview.vue'
import { useBusinessSocket } from '@/composables/useBusinessSocket'
import { useDeleteConfirmDialog } from '@/composables/useDeleteConfirmDialog'
import { useWorkflowBindingStatus } from '@/composables/useWorkflowBindingStatus'
import { useWorkflowCatalog } from '@/composables/useWorkflowCatalog'

interface TaskItem {
  id: number
  type: string
  status: string
  external_task_id?: string
  progress_percent?: number
  progress_stage?: string
  progress_message?: string
  segment_total?: number
  segment_completed?: number
  audio_url?: string
  meeting_id?: number
  post_process_status?: string
  post_process_error?: string
  post_processed_at?: string
  sync_fail_count?: number
  last_sync_error?: string
  last_sync_at?: string
  next_sync_at?: string
  result_text?: string
  duration: number
  workflow_id?: number
  created_at?: string
  createdAt?: string
  updated_at?: string
  updatedAt?: string
}

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
  workflow_id: number
  trigger_type: string
  final_text?: string
  status: string
  error_message?: string
  created_at?: string
  node_results?: ExecutionNodeResult[]
}

interface ExecutionSummary {
  status: string
  created_at?: string
  error_message?: string
}

const EXECUTION_SUMMARY_NOT_STARTED = 'not_started'

const message = useMessage()
const confirmDelete = useDeleteConfirmDialog()
const route = useRoute()
const router = useRouter()
const batchWorkflowCatalog = useWorkflowCatalog('batch_transcription', 100)
const {
  configuredWorkflowId,
  configuredWorkflow: selectedWorkflowOption,
  configuredWorkflowLabel,
  configuredWorkflowMissing,
  configuredWorkflowNotice: configuredWorkflowMessage,
} = useWorkflowBindingStatus('batch', batchWorkflowCatalog, {
  emptyLabel: '未配置默认工作流',
  unsetMessage: '当前未配置批量转写默认工作流，提交任务后只会执行 ASR，不会自动触发后处理。',
  missingMessage: workflowId => `应用配置中的批量工作流 #${workflowId} 当前不可用，请前往应用配置页重新选择。`,
  readyMessage: () => '当前上传文件和 URL 提交都会自动带上应用配置中的默认工作流。',
})
const { subscribe: subscribeBusinessTopic } = useBusinessSocket()
const loading = ref(false)
const creating = ref(false)
const uploading = ref(false)
const detailLoading = ref(false)
const syncingAll = ref(false)
const syncingIds = ref<number[]>([])
const deletingTaskId = ref<number | null>(null)
const resumingTaskId = ref<number | null>(null)
const tasks = ref<TaskItem[]>([])
const keyword = ref('')
const fileInput = ref<HTMLInputElement | null>(null)
const selectedFile = ref<File | null>(null)
const detailVisible = ref(false)
const detailTask = ref<TaskItem | null>(null)
const detailExecutions = ref<ExecutionItem[]>([])
const executionSummaryByTask = ref<Record<number, ExecutionSummary>>({})
const contentViewerVisible = ref(false)
const contentViewerTitle = ref('')
const contentViewerText = ref('')
const nowTimestamp = ref(Date.now())
const submitForm = reactive({
  audioUrl: '',
})
let stopBusinessSubscription: (() => void) | null = null
let clockTimer: number | null = null

function formatDateTime(value?: string) {
  if (!value)
    return '-'

  const date = new Date(value)
  if (Number.isNaN(date.getTime()))
    return value

  return date.toLocaleString('zh-CN', {
    hour12: false,
  })
}

function extractErrorMessage(error: unknown, fallback: string) {
  const messageText = (error as { response?: { data?: { message?: string } } })?.response?.data?.message
  if (typeof messageText === 'string' && messageText.trim())
    return messageText
  return fallback
}

function sanitizeTranscriptionText(value?: string) {
  if (!value)
    return ''

  return value
    .replace(/language\s+[a-z_-]+<asr_text>/gi, '')
    .replace(/<\/?asr_text>/gi, '')
    .replace(/<\|[^>]+\|>/g, '')
    .replace(/\u00A0/g, ' ')
    .trim()
}

function formatDuration(value?: number) {
  if (!value || value <= 0)
    return '-'

  const totalSeconds = Math.max(0, Math.round(value))
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60

  if (hours > 0)
    return `${hours}时${minutes}分${seconds}秒`
  if (minutes > 0)
    return `${minutes}分${seconds}秒`
  return `${seconds}秒`
}

function parseDateValue(value?: string) {
  if (!value)
    return 0

  const timestamp = new Date(value).getTime()
  return Number.isNaN(timestamp) ? 0 : timestamp
}

function formatTaskType(value?: string) {
  const map: Record<string, string> = {
    realtime: '实时',
    batch: '批量',
  }
  return map[value || ''] || value || '-'
}

function formatTaskStatus(value?: string) {
  const map: Record<string, string> = {
    pending: '待处理',
    processing: '处理中',
    completed: '已完成',
    failed: '失败',
  }
  return map[value || ''] || value || '-'
}

function workflowLabel(workflowId?: number) {
  if (!workflowId)
    return '-'
  return batchWorkflowCatalog.labelForWorkflow(workflowId)
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

function formatProgressStage(value?: string) {
  const map: Record<string, string> = {
    queued: '已入队',
    submitted: '已提交',
    submitting: '提交中',
    processing: '解析中',
    transcribed: '转写完成',
    postprocessing: '后处理中',
    postprocess_failed: '后处理失败',
    retry_waiting: '等待重试',
    completed: '已完成',
    failed: '失败',
  }
  return map[value || ''] || value || '-'
}

function formatExecutionStatus(value?: string) {
  const map: Record<string, string> = {
    not_started: '未执行',
    pending: '待执行',
    running: '执行中',
    completed: '已完成',
    failed: '失败',
    success: '成功',
    skipped: '跳过',
  }
  return map[value || ''] || value || '-'
}

function executionSummaryState(task: TaskItem | null | undefined) {
  if (!task?.workflow_id) {
    return {
      label: '未绑定',
      type: 'default',
      tooltip: '当前任务未绑定工作流，因此不会产生工作流执行记录。',
      emptyMessage: '当前任务未绑定工作流，因此不会产生执行记录。',
    }
  }

  const summary = executionSummaryByTask.value[task.id]
  if (summary && summary.status !== EXECUTION_SUMMARY_NOT_STARTED) {
    const parts = [`状态：${formatExecutionStatus(summary.status)}`]
    if (summary.created_at)
      parts.push(`时间：${formatDateTime(summary.created_at)}`)
    if (summary.error_message?.trim())
      parts.push(`错误：${summary.error_message.trim()}`)
    else if (summary.status === 'completed' || summary.status === 'success')
      parts.push('最近一次工作流执行已完成。')
    else if (summary.status === 'running' || summary.status === 'pending')
      parts.push('最近一次工作流仍在执行中或等待执行。')

    return {
      label: formatExecutionStatus(summary.status),
      type: summary.status === 'completed' || summary.status === 'success'
        ? 'success'
        : summary.status === 'running' || summary.status === 'pending'
          ? 'info'
          : summary.status === 'failed'
            ? 'error'
            : 'default',
      tooltip: parts.join('\n'),
      emptyMessage: '当前任务已返回工作流执行记录。',
    }
  }

  if (task.status === 'pending' || task.status === 'processing') {
    return {
      label: '等待 ASR',
      type: 'default',
      tooltip: '当前任务已绑定工作流，但需要先等待 ASR 转写完成，之后才会触发工作流后处理。',
      emptyMessage: '当前任务已绑定工作流，待 ASR 转写完成后会自动触发工作流执行。',
    }
  }

  if (task.post_process_status === 'processing') {
    return {
      label: '后处理中',
      type: 'info',
      tooltip: 'ASR 已完成，工作流后处理正在执行中，执行记录可能稍后返回。',
      emptyMessage: '当前任务已进入工作流后处理阶段，执行记录可能稍后显示。',
    }
  }

  if (task.post_process_status === 'pending') {
    return {
      label: '待后处理',
      type: 'default',
      tooltip: 'ASR 已完成，但工作流后处理尚未开始。',
      emptyMessage: '当前任务已完成 ASR，但工作流后处理尚未开始。',
    }
  }

  if (task.post_process_status === 'failed') {
    return {
      label: '后处理失败',
      type: 'error',
      tooltip: task.post_process_error?.trim()
        ? `任务后处理失败：${task.post_process_error.trim()}`
        : '任务后处理失败，当前没有返回对应的工作流执行记录。',
      emptyMessage: task.post_process_error?.trim()
        ? `当前任务后处理失败：${task.post_process_error.trim()}`
        : '当前任务后处理失败，但没有返回对应的工作流执行记录。',
    }
  }

  return {
    label: '未查到记录',
    type: 'warning',
    tooltip: '当前任务已绑定工作流，但没有查询到执行记录。这通常表示历史数据仍是旧链路，或执行记录尚未写回。',
    emptyMessage: '当前任务已绑定工作流，但暂未查到执行记录。这通常是历史数据、legacy 工作流，或执行记录尚未写回。',
  }
}

function executionSummaryLabel(task: TaskItem | null | undefined) {
  return executionSummaryState(task).label
}

function executionSummaryType(task: TaskItem | null | undefined) {
  return executionSummaryState(task).type
}

function executionSummaryTooltip(task: TaskItem | null | undefined) {
  return executionSummaryState(task).tooltip
}

function taskProgressPercent(task: TaskItem | null | undefined) {
  const rawValue = task?.progress_percent ?? 0
  return Math.max(0, Math.min(100, Math.round(rawValue)))
}

function taskProgressMessage(task: TaskItem | null | undefined) {
  if (!task)
    return '任务已入队'
  if (task.progress_message?.trim())
    return task.progress_message
  if (task.status === 'failed')
    return task.last_sync_error || task.post_process_error || '任务失败'
  if (task.status === 'completed')
    return task.post_process_status === 'completed' ? '转写与后处理已完成' : '转写完成'
  if (task.status === 'processing')
    return 'ASR 正在解析音频'
  return '任务已入队，等待提交到 ASR'
}

function getRemarkContent(task: TaskItem | null | undefined) {
  if (!task)
    return ''
  return sanitizeTranscriptionText(task.result_text) || task.last_sync_error || task.progress_message || ''
}

function openContentViewer(task: TaskItem) {
  const content = getRemarkContent(task)
  if (!content) {
    message.info('当前没有可查看的内容')
    return
  }

  contentViewerTitle.value = `任务 #${task.id} 内容查看`
  contentViewerText.value = content
  contentViewerVisible.value = true
}

function patchTaskRow(task: TaskItem | null | undefined) {
  if (!task)
    return

  const index = tasks.value.findIndex(item => item.id === task.id)
  if (index === -1)
    return

  tasks.value[index] = {
    ...tasks.value[index],
    ...task,
  }
}

function applyTaskUpdate(task: TaskItem | null | undefined) {
  if (!task)
    return

  const existingIndex = tasks.value.findIndex(item => item.id === task.id)
  if (existingIndex === -1) {
    tasks.value = [task, ...tasks.value].slice(0, 100)
  }
  else {
    patchTaskRow(task)
  }

  if (detailTask.value?.id === task.id)
    detailTask.value = { ...detailTask.value, ...task }
}

function isTaskActive(task: TaskItem | null | undefined) {
  if (!task || task.type !== 'batch')
    return false
  if (task.status === 'pending' || task.status === 'processing')
    return true
  return task.status === 'completed' && task.post_process_status !== 'completed' && task.post_process_status !== 'failed'
}

function isTaskDeletable(task: TaskItem | null | undefined) {
  if (!task)
    return false
  if (task.status === 'failed')
    return true
  if (task.status !== 'completed')
    return false
  return !isTaskActive(task)
}

function taskElapsedText(task: TaskItem | null | undefined) {
  if (!task)
    return '-'

  const start = parseDateValue(task.createdAt || task.created_at)
  if (!start)
    return '-'

  const end = isTaskActive(task)
    ? nowTimestamp.value
    : parseDateValue(task.updatedAt || task.updated_at)
      || parseDateValue(task.post_processed_at)
      || parseDateValue(task.last_sync_at)
      || nowTimestamp.value
  const totalSeconds = Math.max(0, Math.floor((end - start) / 1000))
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60

  if (hours > 0)
    return `${hours}时${minutes}分${seconds}秒`
  if (minutes > 0)
    return `${minutes}分${seconds}秒`
  return `${seconds}秒`
}

function renderProgressCell(row: TaskItem) {
  const percent = taskProgressPercent(row)
  return h('div', { class: 'min-w-0' }, [
    h('div', { class: 'flex items-center justify-between gap-2 text-xs text-slate' }, [
      h('span', { class: 'min-w-0 flex-1 truncate' }, taskProgressMessage(row)),
      h('span', { class: 'shrink-0 whitespace-nowrap text-right font-700 tabular-nums text-ink' }, `${percent}%`),
    ]),
    h('div', { class: 'mt-1 h-1.5 overflow-hidden rounded-full', style: { backgroundColor: 'rgba(100, 116, 139, 0.16)' } }, [
      h('div', { class: 'h-full rounded-full transition-all duration-300', style: { width: `${percent}%`, backgroundColor: '#0f766e' } }),
    ]),
  ])
}

const columns = [
  { title: 'ID', key: 'id', width: 64 },
  {
    title: '类型',
    key: 'type',
    width: 72,
    render: (row: TaskItem) => formatTaskType(row.type),
  },
  {
    title: '状态',
    key: 'status',
    width: 88,
    render: (row: TaskItem) => {
      const map: Record<string, { type: string, text: string }> = {
        pending: { type: 'default', text: '待处理' },
        processing: { type: 'info', text: '处理中' },
        completed: { type: 'success', text: '已完成' },
        failed: { type: 'error', text: '失败' },
      }
      const item = map[row.status] || { type: 'default', text: formatTaskStatus(row.status) }
      return h(NTag, { type: item.type as any, size: 'small', round: true, bordered: false }, { default: () => item.text })
    },
  },
  {
    title: '工作流',
    key: 'workflow_id',
    width: 160,
    render: (row: TaskItem) => workflowLabel(row.workflow_id),
  },
  {
    title: '执行摘要',
    key: 'execution_status',
    width: 120,
    render: (row: TaskItem) => {
      if (!row.workflow_id)
        return '-'
      return h(NTooltip, { placement: 'top' }, {
        trigger: () => h(NTag, { type: executionSummaryType(row) as any, size: 'small', round: true, bordered: false }, { default: () => executionSummaryLabel(row) }),
        default: () => h('div', { class: 'max-w-72 whitespace-pre-wrap text-xs leading-6' }, executionSummaryTooltip(row)),
      })
    },
  },
  {
    title: '备注',
    key: 'result_text',
    width: 280,
    render: (row: TaskItem) => {
      const content = getRemarkContent(row)
      if (!content)
        return '-'

      return h('div', { class: 'min-w-0' }, [
        h('div', {
          class: 'truncate text-sm text-ink',
          title: content.length <= 80 ? content : undefined,
        }, content),
        h(NButton, {
          text: true,
          type: 'primary',
          size: 'small',
          class: 'mt-1',
          onClick: () => openContentViewer(row),
        }, { default: () => '查看内容' }),
      ])
    },
  },
  {
    title: '后处理',
    key: 'post_process_status',
    width: 88,
    render: (row: TaskItem) => {
      const s = row.post_process_status || '-'
      const map: Record<string, { type: string, text: string }> = {
        pending: { type: 'default', text: '待处理' },
        processing: { type: 'info', text: '处理中' },
        completed: { type: 'success', text: '已完成' },
        failed: { type: 'error', text: '失败' },
      }
      const item = map[s]
      return item
        ? h(NTag, { type: item.type as any, size: 'small', round: true, bordered: false }, { default: () => item.text })
        : s
    },
  },
  {
    title: '进度',
    key: 'progress_percent',
    width: 220,
    render: (row: TaskItem) => renderProgressCell(row),
  },
  { title: '音频时长', key: 'duration', width: 120, render: (row: TaskItem) => formatDuration(row.duration) },
  { title: '处理耗时', key: 'elapsed', width: 120, render: (row: TaskItem) => taskElapsedText(row) },
  {
    title: '创建时间',
    key: 'createdAt',
    width: 160,
    render: (row: TaskItem) => formatDateTime(row.createdAt || row.created_at),
  },
  {
    title: '操作',
    key: 'actions',
    width: 200,
    render: (row: TaskItem) => h('div', { class: 'flex items-center gap-1' }, [
      h(NButton, {
        text: true,
        type: 'primary',
        size: 'small',
        onClick: () => handleShowDetail(row.id),
      }, { default: () => '详情' }),
      row.audio_url
        ? h(NButton, {
            text: true,
            size: 'small',
            onClick: () => handleDownloadAudio(row),
          }, { default: () => '音频' })
        : null,
      row.meeting_id
        ? h(NButton, {
            text: true,
            size: 'small',
            onClick: () => void router.push({ name: 'meeting-detail', params: { id: String(row.meeting_id) } }),
          }, { default: () => '会议' })
        : null,
      h(NButton, {
        text: true,
        size: 'small',
        loading: syncingIds.value.includes(row.id),
        disabled: row.type !== 'batch' || !isTaskActive(row),
        onClick: () => handleSyncSingle(row.id),
      }, { default: () => '同步' }),
      isTaskDeletable(row)
        ? h(NButton, {
            text: true,
            size: 'small',
            type: 'error',
            loading: deletingTaskId.value === row.id,
            onClick: () => handleDeleteTask(row),
          }, { default: () => '删除' })
        : null,
    ]),
  },
]

const canDownloadAudio = computed(() => Boolean(detailTask.value?.audio_url))
const detailWorkflowPendingUpgrade = computed(() => {
  const workflowId = detailTask.value?.workflow_id
  if (!workflowId)
    return false
  return !batchWorkflowCatalog.hasWorkflow(workflowId)
})
const detailWorkflowPendingUpgradeMessage = computed(() => {
  if (!detailWorkflowPendingUpgrade.value || !detailTask.value?.workflow_id)
    return ''
  return `当前任务绑定的工作流 #${detailTask.value.workflow_id} 不在可用批量工作流列表中，通常表示它仍是待升级的 legacy 工作流。请前往工作流管理页补齐 source 节点后再继续复用。`
})
const detailExecutionState = computed(() => executionSummaryState(detailTask.value))
const canResumePostProcessFromFailure = computed(() => {
  const task = detailTask.value
  if (!task)
    return false
  return task.type === 'batch'
    && task.status === 'completed'
    && Boolean(task.workflow_id)
    && task.post_process_status === 'failed'
    && !detailWorkflowPendingUpgrade.value
})

const filteredTasks = computed(() => {
  const value = keyword.value.trim().toLowerCase()
  if (!value)
    return tasks.value
  return tasks.value.filter(item =>
    String(item.id).includes(value)
    || item.type.toLowerCase().includes(value)
    || item.status.toLowerCase().includes(value),
  )
})

const activeTaskIds = computed(() => {
  return tasks.value
    .filter(task => isTaskActive(task))
    .map(task => task.id)
})

watch(() => route.query.taskId, (taskId) => {
  if (typeof taskId === 'string' && taskId.trim())
    keyword.value = taskId.trim()
}, { immediate: true })

async function loadWorkflowOptions() {
  try {
    await batchWorkflowCatalog.loadWorkflows()
  }
  catch {
    message.warning('工作流列表加载失败，可稍后到应用配置页重试')
  }
}

async function loadTasks(options?: { silent?: boolean }) {
  loading.value = true
  try {
    const result = await getTranscriptionTasks({ offset: 0, limit: 100 })
    tasks.value = result.data.items
    await loadExecutionSummaries(result.data.items)
  }
  catch {
    if (!options?.silent)
      message.error('转写历史加载失败')
  }
  finally {
    loading.value = false
  }
}

async function loadExecutionSummaries(items: TaskItem[]) {
  const workflowTasks = items.filter(item => item.workflow_id)
  if (workflowTasks.length === 0) {
    executionSummaryByTask.value = {}
    return
  }

  const next: Record<number, ExecutionSummary> = {}
  const results = await Promise.allSettled(
    workflowTasks.map(async (item) => {
      const result = await getTranscriptionTaskExecutions(item.id)
      const latest = (result.data || [])[0] as ExecutionItem | undefined
      return {
        taskId: item.id,
        summary: latest
          ? {
              status: latest.status,
              created_at: latest.created_at,
              error_message: latest.error_message,
            }
          : {
              status: EXECUTION_SUMMARY_NOT_STARTED,
            },
      }
    }),
  )

  for (const result of results) {
    if (result.status !== 'fulfilled')
      continue
    next[result.value.taskId] = result.value.summary
  }
  executionSummaryByTask.value = next
}

async function refreshExecutionSummary(task: TaskItem | null | undefined) {
  if (!task?.workflow_id)
    return
  try {
    const result = await getTranscriptionTaskExecutions(task.id)
    const latest = (result.data || [])[0] as ExecutionItem | undefined
    executionSummaryByTask.value = {
      ...executionSummaryByTask.value,
      [task.id]: latest
        ? {
            status: latest.status,
            created_at: latest.created_at,
            error_message: latest.error_message,
          }
        : {
            status: EXECUTION_SUMMARY_NOT_STARTED,
          },
    }
  }
  catch {
    // Ignore summary refresh failures to avoid noisy list refreshes.
  }
}

async function handleCreateTask() {
  if (!submitForm.audioUrl.trim()) {
    message.warning('请填写音频 URL')
    return
  }

  creating.value = true
  try {
    await createTranscriptionTask({
      audio_url: submitForm.audioUrl.trim(),
      type: 'batch',
      workflow_id: configuredWorkflowId.value ?? undefined,
    })
    message.success('批量转写任务已提交到 ASR 引擎')
    submitForm.audioUrl = ''
    await loadTasks()
  }
  catch (error) {
    message.error(extractErrorMessage(error, '批量转写提交失败'))
    await loadTasks()
  }
  finally {
    creating.value = false
  }
}

function handleChooseFile() {
  fileInput.value?.click()
}

function handleFileSelected(event: Event) {
  const target = event.target as HTMLInputElement | null
  selectedFile.value = target?.files?.[0] ?? null
}

function clearSelectedFile() {
  selectedFile.value = null
  if (fileInput.value)
    fileInput.value.value = ''
}

function handleDownloadAudio(task: TaskItem) {
  if (!task.audio_url) {
    message.warning('该任务没有原音频地址')
    return
  }
  window.open(task.audio_url, '_blank', 'noopener,noreferrer')
}

async function handleShowDetail(taskId: number) {
  detailLoading.value = true
  detailVisible.value = true
  try {
    const [result, executionsResult] = await Promise.all([
      getTranscriptionTaskDetail(taskId),
      getTranscriptionTaskExecutions(taskId),
    ])
    detailTask.value = result.data
    detailExecutions.value = executionsResult.data || []
    patchTaskRow(result.data)
  }
  catch (error) {
    detailTask.value = null
    detailExecutions.value = []
    detailVisible.value = false
    message.error(extractErrorMessage(error, '任务详情加载失败'))
  }
  finally {
    detailLoading.value = false
  }
}

async function handleResumePostProcess() {
  if (!detailTask.value || !canResumePostProcessFromFailure.value || resumingTaskId.value != null)
    return

  const taskId = detailTask.value.id
  resumingTaskId.value = taskId
  try {
    const result = await resumeTranscriptionTaskPostProcess(taskId)
    detailTask.value = result.data
    patchTaskRow(result.data)
    const executionsResult = await getTranscriptionTaskExecutions(taskId)
    detailExecutions.value = executionsResult.data || []
    await refreshExecutionSummary(result.data)

    if (result.data.post_process_status === 'processing' || result.data.post_process_status === 'completed')
      message.success('已从失败节点继续执行后处理')
    else
      message.warning(result.data.post_process_error || '继续执行失败，请查看后处理错误信息')
  }
  catch (error) {
    message.error(extractErrorMessage(error, '继续执行后处理失败'))
  }
  finally {
    resumingTaskId.value = null
  }
}

async function handleUploadTask() {
  if (!selectedFile.value) {
    message.warning('请先选择音频文件')
    return
  }

  const formData = new FormData()
  formData.append('file', selectedFile.value)
  if (configuredWorkflowId.value != null)
    formData.append('workflow_id', String(configuredWorkflowId.value))

  uploading.value = true
  try {
    await uploadTranscriptionFile(formData)
    message.success('音频已上传并创建批量转写任务')
    clearSelectedFile()
    await loadTasks()
  }
  catch (error) {
    message.error(extractErrorMessage(error, '音频上传失败'))
    await loadTasks()
  }
  finally {
    uploading.value = false
  }
}

async function handleSyncSingle(taskId: number) {
  await syncTaskIds([taskId], { silent: false, successMessage: '任务状态已同步' })
}

async function handleDeleteTask(row: TaskItem) {
  if (!isTaskDeletable(row) || deletingTaskId.value != null)
    return

  const confirmed = await confirmDelete({
    entityType: '任务记录',
    entityName: `#${row.id}`,
    description: row.audio_url
      ? '删除后，这条任务会从列表中移除；如果音频是通过当前系统上传的，本地上传文件也会一并清理。'
      : '删除后，这条任务会从列表中移除，已有会议记录和执行结果不会自动回滚。',
  })
  if (!confirmed)
    return

  deletingTaskId.value = row.id
  try {
    await deleteTranscriptionTask(row.id)
    tasks.value = tasks.value.filter(item => item.id !== row.id)
    const nextSummaries = { ...executionSummaryByTask.value }
    delete nextSummaries[row.id]
    executionSummaryByTask.value = nextSummaries
    if (detailTask.value?.id === row.id) {
      detailVisible.value = false
      detailTask.value = null
      detailExecutions.value = []
    }
    message.success('任务记录已删除')
  }
  catch (error) {
    message.error(extractErrorMessage(error, '任务删除失败'))
  }
  finally {
    deletingTaskId.value = null
  }
}

async function handleSyncProcessing() {
  syncingAll.value = true
  try {
    await syncTaskIds(activeTaskIds.value, {
      silent: false,
      emptyMessage: '当前没有需要同步的批量任务',
      successMessage: `已触发 ${activeTaskIds.value.length} 个任务的后台刷新`,
      errorMessage: '批量同步任务状态失败',
    })
  }
  finally {
    syncingAll.value = false
  }
}

async function syncTaskIds(taskIds: number[], options?: {
  silent?: boolean
  emptyMessage?: string
  successMessage?: string
  errorMessage?: string
}) {
  const uniqueTaskIds = Array.from(new Set(taskIds)).filter(taskId => !syncingIds.value.includes(taskId))

  if (uniqueTaskIds.length === 0) {
    if (!options?.silent && options?.emptyMessage)
      message.info(options.emptyMessage)
    return
  }

  syncingIds.value = [...syncingIds.value, ...uniqueTaskIds]
  try {
    const results = await Promise.allSettled(uniqueTaskIds.map(taskId => syncTranscriptionTask(taskId)))
    const failedCount = results.filter(result => result.status === 'rejected').length

    await loadTasks({ silent: true })

    if (failedCount > 0) {
      if (!options?.silent)
        message.error(options?.errorMessage || '任务状态同步失败')
      return
    }

    if (!options?.silent && options?.successMessage)
      message.success(options.successMessage)
  }
  finally {
    syncingIds.value = syncingIds.value.filter(id => !uniqueTaskIds.includes(id))
  }
}

onMounted(async () => {
  await Promise.all([loadTasks(), loadWorkflowOptions()])
  clockTimer = window.setInterval(() => {
    nowTimestamp.value = Date.now()
  }, 1000)

  stopBusinessSubscription = subscribeBusinessTopic(['asr.task.updated'], (event) => {
    const payload = event.payload as { task?: TaskItem } | undefined
    applyTaskUpdate(payload?.task)
    void refreshExecutionSummary(payload?.task)
  })
})

onBeforeUnmount(() => {
  stopBusinessSubscription?.()
  stopBusinessSubscription = null
  if (clockTimer != null)
    window.clearInterval(clockTimer)
})
</script>

<template>
  <div class="flex h-full min-w-0 min-h-0 flex-col gap-5 overflow-hidden">
    <NCard class="card-main shrink-0">
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span class="text-sm font-600">发起批量转写</span>
          <div class="flex items-center gap-2 text-xs text-slate">
            <span>当前默认工作流：{{ configuredWorkflowLabel }}</span>
            <NButton text size="small" @click="router.push('/workflows/application-settings')">
              应用配置
            </NButton>
          </div>
        </div>
      </template>
      <NTabs type="line" animated>
        <NTabPane name="upload" tab="上传本地音频">
          <div class="flex flex-wrap items-center gap-3 pt-3">
            <NInput :value="selectedFile?.name || ''" readonly placeholder="请选择 wav / mp3 / m4a / flac 等音频文件" class="w-full sm:!w-80" />
            <NButton quaternary @click="handleChooseFile">
              选择文件
            </NButton>
            <NButton quaternary @click="router.push('/workflows/application-settings')">
              应用配置
            </NButton>
            <NButton type="primary" color="#0f766e" :loading="uploading" @click="handleUploadTask">
              上传并转写
            </NButton>
          </div>
          <div class="mt-2 flex flex-wrap items-center gap-3 text-xs text-slate">
            <span>{{ selectedFile ? `文件大小 ${(selectedFile.size / 1024 / 1024).toFixed(2)} MB` : '未选择文件' }}</span>
            <span>{{ configuredWorkflowMessage }}</span>
            <NButton v-if="selectedFile" text size="small" @click="clearSelectedFile">
              清除
            </NButton>
          </div>
        </NTabPane>
        <NTabPane name="url" tab="提交音频 URL">
          <div class="flex flex-wrap items-center gap-3 pt-3">
            <NInput v-model:value="submitForm.audioUrl" placeholder="https://example.com/audio/demo.wav" class="w-full sm:!w-96" />
            <NButton quaternary @click="router.push('/workflows/application-settings')">
              应用配置
            </NButton>
            <NButton type="primary" color="#0f766e" :loading="creating" @click="handleCreateTask">
              提交 URL 任务
            </NButton>
          </div>
          <div class="mt-2 text-xs text-slate">
            {{ configuredWorkflowMessage }}
          </div>
        </NTabPane>
      </NTabs>

      <div class="mt-4">
        <div v-if="configuredWorkflowMissing" class="mb-3 rounded-2 border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-6 text-amber-700">
          {{ configuredWorkflowMessage }}
        </div>
        <WorkflowSelectionPreview
          :workflow="selectedWorkflowOption"
          empty-title="未配置批量默认工作流"
          empty-description="前往应用配置页设置后，这里会展示批量任务统一复用的后处理链路。"
        />
      </div>

      <input
        ref="fileInput"
        type="file"
        accept="audio/*,.wav,.mp3,.m4a,.aac,.flac,.ogg,.opus,.webm"
        class="hidden"
        @change="handleFileSelected"
      >
    </NCard>

    <NCard class="card-main task-list-card min-h-0 flex-1 overflow-hidden" content-style="display: flex; flex-direction: column; min-height: 0; padding: 0 20px 20px;">
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span class="text-sm font-600">任务列表</span>
          <div class="flex flex-wrap items-center gap-2">
            <NInput v-model:value="keyword" clearable placeholder="搜索 ID / 类型 / 状态" size="small" class="w-full sm:!w-48" />
            <NButton quaternary size="small" :loading="syncingAll" @click="handleSyncProcessing">
              同步
            </NButton>
            <NButton quaternary size="small" @click="() => loadTasks()">
              刷新
            </NButton>
          </div>
        </div>
      </template>
      <NDataTable
        flex-height
        class="flex-1 min-h-0"
        :columns="columns"
        :data="filteredTasks"
        :loading="loading"
        :pagination="{ pageSize: 10 }"
        :scroll-x="900"
        size="small"
      />
    </NCard>

    <NModal v-model:show="detailVisible" preset="card" title="转写任务详情" class="modal-card max-w-3xl">
      <div v-if="detailLoading" class="py-10 text-center text-slate">
        正在加载任务详情...
      </div>
      <div v-else-if="detailTask" class="grid gap-4">
        <div class="grid grid-cols-1 gap-3 md:grid-cols-5">
          <div class="rounded-2.5 bg-mist/60 p-3.5">
            <div class="text-xs text-slate/70">
              任务状态
            </div>
            <div class="mt-1.5 text-base font-700 text-ink">
              {{ formatTaskStatus(detailTask.status) }}
            </div>
          </div>
          <div class="rounded-2.5 bg-mist/60 p-3.5">
            <div class="text-xs text-slate/70">
              任务类型
            </div>
            <div class="mt-1.5 text-base font-700 text-ink">
              {{ formatTaskType(detailTask.type) }}
            </div>
          </div>
          <div class="rounded-2.5 bg-mist/60 p-3.5">
            <div class="text-xs text-slate/70">
              音频时长
            </div>
            <div class="mt-1.5 text-base font-700 text-ink">
              {{ formatDuration(detailTask.duration) }}
            </div>
          </div>
          <div class="rounded-2.5 bg-mist/60 p-3.5">
            <div class="text-xs text-slate/70">
              处理耗时
            </div>
            <div class="mt-1.5 text-base font-700 text-ink">
              {{ taskElapsedText(detailTask) }}
            </div>
          </div>
          <div class="rounded-2.5 bg-mist/60 p-3.5">
            <div class="text-xs text-slate/70">
              当前进度
            </div>
            <div class="mt-1.5 text-base font-700 text-ink">
              {{ taskProgressPercent(detailTask) }}%
            </div>
            <div class="mt-2 h-1.5 overflow-hidden rounded-full bg-slate/12">
              <div class="h-full rounded-full bg-teal transition-all duration-300" :style="{ width: `${taskProgressPercent(detailTask)}%` }" />
            </div>
            <div class="mt-2 text-xs text-slate">
              {{ taskProgressMessage(detailTask) }}
            </div>
          </div>
          <div class="rounded-2.5 bg-mist/60 p-3.5">
            <div class="text-xs text-slate/70">
              工作流
            </div>
            <div class="mt-1.5 text-base font-700 text-ink">
              {{ workflowLabel(detailTask.workflow_id) }}
            </div>
          </div>
        </div>

        <div v-if="detailWorkflowPendingUpgradeMessage" class="rounded-2 border border-amber-200 bg-amber-50 px-3 py-2 text-sm leading-6 text-amber-700">
          {{ detailWorkflowPendingUpgradeMessage }}
        </div>

        <div class="rounded-2.5 bg-mist/60 p-3.5">
          <div class="text-xs font-600 text-ink">
            完整转写结果
          </div>
          <div class="mt-2 max-h-64 overflow-auto whitespace-pre-wrap rounded-2 bg-white/80 p-3.5 text-sm leading-7 text-ink">
            {{ sanitizeTranscriptionText(detailTask.result_text) || '当前还没有可展示的结果文本。' }}
          </div>
        </div>

        <div class="rounded-2.5 bg-mist/60 p-3.5">
          <div class="flex items-center justify-between gap-3">
            <div class="text-xs font-600 text-ink">
              工作流执行记录
            </div>
            <div class="text-xs text-slate">
              {{ detailExecutions.length > 0 ? `${detailExecutions.length} 次执行` : detailExecutionState.label }}
            </div>
          </div>
          <div v-if="detailExecutions.length === 0" class="mt-3 rounded-2 bg-white/80 p-3 text-sm text-slate">
            {{ detailExecutionState.emptyMessage }}
          </div>
          <div v-else class="mt-3 grid gap-3">
            <div v-for="execution in detailExecutions" :key="execution.id" class="rounded-2 bg-white/80 p-3.5">
              <div class="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <div class="text-sm font-700 text-ink">
                    执行 #{{ execution.id }} / {{ formatExecutionStatus(execution.status) }}
                  </div>
                  <div class="mt-1 text-xs text-slate">
                    触发类型：{{ execution.trigger_type }} · 创建时间：{{ formatDateTime(execution.created_at) }}
                  </div>
                </div>
                <div class="text-xs text-slate">
                  最终文本长度：{{ execution.final_text?.length || 0 }}
                </div>
              </div>
              <div v-if="execution.error_message" class="mt-3 rounded-2 bg-red-50 px-3 py-2 text-xs text-red-600">
                {{ execution.error_message }}
              </div>
              <div class="mt-3 grid gap-3">
                <div v-for="node in execution.node_results || []" :key="node.id" class="rounded-2 border border-gray-200 bg-[#fbfdff] p-3">
                  <div class="flex flex-wrap items-center justify-between gap-2">
                    <div class="text-sm font-600 text-ink">
                      {{ node.position }}. {{ node.label || node.node_type }}
                    </div>
                    <div class="text-xs text-slate">
                      {{ formatExecutionStatus(node.status) }} · {{ node.duration_ms || 0 }} ms
                    </div>
                  </div>
                  <div class="mt-3 grid gap-3 lg:grid-cols-2">
                    <TextDiffPreview :before-text="sanitizeTranscriptionText(node.input_text)" :after-text="sanitizeTranscriptionText(node.output_text)" />
                  </div>
                  <div class="mt-3">
                    <NodeDetailPanel :detail="node.detail" empty-label="当前节点没有 detail 信息。" />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <div class="rounded-2.5 bg-mist/60 p-3.5">
            <div class="text-xs font-600 text-ink">
              后处理信息
            </div>
            <div class="mt-1.5 text-sm text-slate">
              状态：{{ formatPostProcessStatus(detailTask.post_process_status) }}
            </div>
            <div class="mt-1 text-sm text-slate">
              错误：{{ detailTask.post_process_error || '-' }}
            </div>
            <div class="mt-1 text-sm text-slate">
              完成时间：{{ formatDateTime(detailTask.post_processed_at) }}
            </div>
          </div>
          <div class="rounded-2.5 bg-mist/60 p-3.5">
            <div class="text-xs font-600 text-ink">
              同步信息
            </div>
            <div class="mt-1.5 text-sm text-slate">
              失败次数：{{ detailTask.sync_fail_count || 0 }}
            </div>
            <div class="mt-1 text-sm text-slate">
              进度阶段：{{ formatProgressStage(detailTask.progress_stage) }}
            </div>
            <div class="mt-1 text-sm text-slate">
              分片进度：{{ detailTask.segment_total ? `${detailTask.segment_completed || 0}/${detailTask.segment_total}` : '-' }}
            </div>
            <div class="mt-1 text-sm text-slate">
              上次同步：{{ formatDateTime(detailTask.last_sync_at) }}
            </div>
            <div class="mt-1 text-sm text-slate">
              下次同步：{{ formatDateTime(detailTask.next_sync_at) }}
            </div>
            <div class="mt-1 text-sm text-slate">
              最近错误：{{ detailTask.last_sync_error || '-' }}
            </div>
          </div>
        </div>

        <div class="flex justify-end gap-3">
          <NButton
            v-if="canResumePostProcessFromFailure"
            :loading="resumingTaskId === detailTask.id"
            @click="handleResumePostProcess"
          >
            从失败节点继续
          </NButton>
          <NButton :disabled="!canDownloadAudio" @click="detailTask && handleDownloadAudio(detailTask)">
            下载原音频
          </NButton>
          <NButton type="primary" color="#0f766e" @click="detailVisible = false">
            关闭
          </NButton>
        </div>
      </div>
    </NModal>

    <NModal v-model:show="contentViewerVisible" preset="card" :title="contentViewerTitle" class="modal-card max-w-4xl">
      <div class="rounded-2.5 bg-mist/60 p-3.5">
        <div class="max-h-[70vh] overflow-auto whitespace-pre-wrap rounded-2 bg-white/80 p-4 text-sm leading-7 text-ink">
          {{ contentViewerText || '当前没有可展示的内容。' }}
        </div>
      </div>
      <div class="mt-4 flex justify-end">
        <NButton type="primary" color="#0f766e" @click="contentViewerVisible = false">
          关闭
        </NButton>
      </div>
    </NModal>
  </div>
</template>

<style scoped>
.task-list-card {
  display: flex;
  flex-direction: column;
}
.task-list-card :deep(.n-card__content) {
  flex: 1;
}
</style>
